# Implementing a Driver

## Overview

A driver is the bridge between the `core` framework and a real database server. It is responsible for:

- Implementing the `Server` interface so the framework can start and stop it.
- Holding a reference to `DriverAPI` and calling its methods as database events occur.
- Never speaking to `core` about anything that happens purely at the wire/protocol level (packet framing, auth handshake details, TLS negotiation, etc.) — only about logical database events.

The driver owns everything below the logical boundary: network I/O, protocol parsing, authentication, and wire-format serialisation. The framework owns everything above it: handler dispatch, delta overlays, plugin coordination, and the event bus.

---

## `Server` Interface

```/dev/null/server.go#L1-4
type Server interface {
    Run() error
    Stop() error
}
```

**`Run() error`**
Called by the framework in a dedicated goroutine during startup (see `vdb.server.start` → `launch` point). `Run` must bind the network port and block until either `Stop` is called or a fatal, unrecoverable error occurs. Any error returned from `Run` is captured by the framework and propagated as the return value of `App.Run()`, which will also trigger a graceful shutdown.

**`Stop() error`**
Called by the framework during graceful shutdown (see `vdb.server.stop` → `halt` point). `Stop` must signal `Run` to return and release all held resources — listener socket, open connections, background goroutines, etc. `Stop` should return only after cleanup is complete or a timeout is reached internally.

---

## Wiring Order — Important

`app.DriverAPI()` must be called **before** `app.UseDriver()`. The `DriverAPI` handle is passed into the driver constructor so the driver has a reference to the bridge before the framework starts. Reversing this order will cause a nil-reference panic when the first connection arrives.

```/dev/null/main.go#L1-10
app := core.New(core.Config{PluginDir: "plugins"})

api := app.DriverAPI()
srv := mydriver.New(cfg, api)   // api passed here

app.UseDriver(srv)              // server registered here
log.Fatal(app.Run())
```

---

## Connection Lifecycle

### Opening a connection

When a client connects and the driver has authenticated or otherwise accepted the network session, report the connection to the framework before doing anything else:

```/dev/null/conn.go#L1-4
err := d.api.ConnectionOpened(connID, user, remoteAddr)
if err != nil {
    // reject the connection — send an error packet and close the socket
}
```

- `connID` must be a unique `uint32` for this connection's lifetime. The value must not be reused while a prior connection with the same ID is still open. A global atomic counter is a sufficient strategy.
- The framework uses `connID` to track all per-connection state (delta overlays, in-flight transactions, etc.).
- If `ConnectionOpened` returns a non-nil error, the connection must be rejected: send an appropriate wire-protocol error to the client, then close the socket.

### Closing a connection

When the client disconnects for any reason — clean `QUIT`, network error, or driver-side close — always call:

```/dev/null/conn.go#L6-7
d.api.ConnectionClosed(connID, user, remoteAddr)
```

`ConnectionClosed` has no return value; connection teardown always proceeds regardless of internal state. Call it even if `ConnectionOpened` failed — the framework is safe to call with an unknown `connID`.

---

## Query Lifecycle

Before forwarding a query to the source database, pass it through the framework:

```/dev/null/query.go#L1-6
rewritten, err := d.api.QueryReceived(connID, originalSQL, currentDatabase)
if err != nil {
    // surface as a query error to the client; do not forward to source DB
    return
}
// use rewritten, not originalSQL, when executing against the source database
```

- `rewritten` is the query as modified by any registered handlers. Always use `rewritten` for the upstream execution.
- `currentDatabase` is the logical database name active on this connection at the moment the query arrives (equivalent to the `USE` state tracked by the driver).
- If `QueryReceived` returns an error, report the error to the client and skip the upstream round-trip entirely.

After the source database returns a result (success or failure):

```/dev/null/query.go#L8-9
d.api.QueryCompleted(connID, rewritten, rowsAffected, queryErr)
```

`QueryCompleted` is fire-and-forget — it has no return value and the driver does not wait on any result.

---

## Record Reads — Delta Overlay

After receiving rows from the source database for a `SELECT`, pass them through the framework before returning them to the client. This allows the delta store and any registered handlers to overlay in-flight virtual writes on top of the committed data.

```/dev/null/read.go#L1-7
overlaid, err := d.api.RecordsSource(connID, tableName, sourceRows)
if err != nil {
    // surface as a query error to the client
    return
}
// send overlaid (not sourceRows) to the client
```

Each row is a `map[string]any` with column names as keys. The slice returned by `RecordsSource` may be a different length than `sourceRows` if handlers have injected or suppressed rows.

If you need a post-overlay hook — for example, to apply a second pass of client-side filtering — call:

```/dev/null/read.go#L9-12
final, err := d.api.RecordsMerged(connID, tableName, overlaid)
if err != nil {
    // err is logged internally; overlaid (pre-error) records were already returned
}
```

`RecordsMerged` errors are logged internally by the framework before returning. The pre-error record slice is still returned, so `final` is always usable even when `err != nil`.

---

## Transaction Lifecycle

Report every transaction boundary to the framework so the delta store can manage isolation correctly.

```/dev/null/txn.go#L1-15
// Client sends BEGIN (or the driver detects an implicit transaction start)
err := d.api.TransactionBegun(connID, isReadOnly)
if err != nil {
    // surface as a transaction error to the client
}

// Client sends COMMIT
err = d.api.TransactionCommitted(connID)
if err != nil {
    // surface as a commit error to the client
}

// Client sends ROLLBACK
// savepointName is empty string ("") when rolling back the full transaction
d.api.TransactionRolledBack(connID, savepointName)
```

`TransactionRolledBack` has no return value — the rollback always proceeds regardless of internal framework state.

---

## Write Operations

All writes intercepted by the driver must be reported to the framework so the delta store stays consistent. The framework may modify the record (via handlers) before returning it — use the returned record as the canonical view of what was written.

### INSERT

```/dev/null/write.go#L1-8
resultRecord, err := d.api.RecordInserted(connID, tableName, map[string]any{
    "id":    42,
    "email": "user@example.com",
})
if err != nil {
    // surface as a write error to the client
    return
}
```

### UPDATE

Both the old row (as read from the source database) and the new row (as the driver intends to write) are required. The framework needs the old row to correctly invalidate delta entries.

```/dev/null/write.go#L10-16
newRecord, err := d.api.RecordUpdated(connID, tableName, oldRow, newRow)
if err != nil {
    // surface as a write error to the client
    return
}
```

### DELETE

```/dev/null/write.go#L18-23
err := d.api.RecordDeleted(connID, tableName, deletedRow)
if err != nil {
    // surface as a write error to the client
    return
}
```

---

## Schema Reporting

Report table schema whenever the driver observes column structure — from a `DESCRIBE`, `SHOW COLUMNS`, or `information_schema` query result. The framework uses this information to resolve stable record keys (primary keys) when managing the delta store.

```/dev/null/schema.go#L1-2
d.api.SchemaLoaded(tableName, []string{"id", "email", "name"}, "id")
```

Arguments: table name, ordered column name slice, primary key column name.

`SchemaLoaded` has no return value.

When DDL changes make the cached schema stale (after `ALTER TABLE`, `DROP TABLE`, etc.):

```/dev/null/schema.go#L4-5
d.api.SchemaInvalidated(tableName)
```

`SchemaInvalidated` has no return value.

---

## Full Driver Skeleton

The following skeleton ties all of the above together. It is illustrative — it omits actual protocol parsing and error-packet formatting, which are wire-protocol-specific details outside the scope of `core`.

```/dev/null/driver.go#L1-130
package mydriver

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/example/virtualdb/core"
)

// Driver implements core.Server. It binds a TCP port, speaks the wire
// protocol, and calls into core.DriverAPI for all logical database events.
type Driver struct {
	cfg      Config
	api      core.DriverAPI
	listener net.Listener

	mu      sync.Mutex
	conns   map[uint32]net.Conn
	nextID  atomic.Uint32

	stopOnce sync.Once
	stopCh   chan struct{}
}

// New constructs a Driver. api must be obtained from app.DriverAPI() before
// app.UseDriver() is called.
func New(cfg Config, api core.DriverAPI) *Driver {
	return &Driver{
		cfg:    cfg,
		api:    api,
		conns:  make(map[uint32]net.Conn),
		stopCh: make(chan struct{}),
	}
}

// Run binds the listen address and accepts connections until Stop is called.
// Implements core.Server.
func (d *Driver) Run() error {
	ln, err := net.Listen("tcp", d.cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("mydriver: listen: %w", err)
	}
	d.listener = ln

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-d.stopCh:
				return nil // normal shutdown
			default:
				return fmt.Errorf("mydriver: accept: %w", err)
			}
		}
		go d.handleConn(conn)
	}
}

// Stop signals Run to return and closes all open connections.
// Implements core.Server.
func (d *Driver) Stop() error {
	d.stopOnce.Do(func() {
		close(d.stopCh)
		if d.listener != nil {
			d.listener.Close()
		}
		d.mu.Lock()
		for _, c := range d.conns {
			c.Close()
		}
		d.mu.Unlock()
	})
	return nil
}

// handleConn manages the full lifecycle of a single client connection.
func (d *Driver) handleConn(conn net.Conn) {
	connID := d.nextID.Add(1)
	user, remoteAddr := parseHandshake(conn) // wire-protocol detail, driver-owned

	// 1. Report connection opened.
	if err := d.api.ConnectionOpened(connID, user, remoteAddr); err != nil {
		sendErrorPacket(conn, err) // wire-protocol detail
		conn.Close()
		return
	}

	d.mu.Lock()
	d.conns[connID] = conn
	d.mu.Unlock()

	// 2. Ensure ConnectionClosed is always called.
	defer func() {
		d.api.ConnectionClosed(connID, user, remoteAddr)
		d.mu.Lock()
		delete(d.conns, connID)
		d.mu.Unlock()
		conn.Close()
	}()

	// 3. Command loop.
	for {
		cmd, err := readCommand(conn) // wire-protocol detail
		if err != nil {
			return // EOF or network error — defer will call ConnectionClosed
		}

		switch cmd.Type {
		case CmdQuery:
			d.handleQuery(conn, connID, cmd)

		case CmdBegin:
			if err := d.api.TransactionBegun(connID, cmd.IsReadOnly); err != nil {
				sendErrorPacket(conn, err)
			}

		case CmdCommit:
			if err := d.api.TransactionCommitted(connID); err != nil {
				sendErrorPacket(conn, err)
			}

		case CmdRollback:
			d.api.TransactionRolledBack(connID, cmd.Savepoint)

		case CmdQuit:
			return
		}
	}
}

// handleQuery executes the full query lifecycle for a single statement.
func (d *Driver) handleQuery(conn net.Conn, connID uint32, cmd Command) {
	// 1. Let the framework rewrite the query.
	rewritten, err := d.api.QueryReceived(connID, cmd.SQL, cmd.Database)
	if err != nil {
		sendErrorPacket(conn, err)
		return
	}

	// 2. Execute against the source database.
	sourceRows, rowsAffected, queryErr := d.cfg.Backend.Execute(context.Background(), rewritten)

	// 3. Overlay virtual writes on top of source rows (SELECT only).
	if queryErr == nil && len(sourceRows) > 0 {
		overlaid, err := d.api.RecordsSource(connID, cmd.Table, sourceRows)
		if err != nil {
			sendErrorPacket(conn, err)
			d.api.QueryCompleted(connID, rewritten, 0, err)
			return
		}
		sendRows(conn, overlaid) // wire-protocol detail
	} else if queryErr != nil {
		sendErrorPacket(conn, queryErr)
	} else {
		sendOK(conn, rowsAffected) // wire-protocol detail
	}

	// 4. Notify the framework that the query is done (fire-and-forget).
	d.api.QueryCompleted(connID, rewritten, rowsAffected, queryErr)
}
```

---

## Error Handling Summary

| `DriverAPI` method | Error behaviour |
|---|---|
| `ConnectionOpened` | Non-nil → reject connection; send error to client, close socket |
| `TransactionBegun` | Non-nil → surface as transaction error to client |
| `TransactionCommitted` | Non-nil → surface as commit error to client |
| `QueryReceived` | Non-nil → surface as query error to client; skip upstream execution |
| `RecordsSource` | Non-nil → surface as query error to client |
| `RecordsMerged` | Non-nil → logged internally; pre-error records still returned |
| `RecordInserted` | Non-nil → surface as write error to client |
| `RecordUpdated` | Non-nil → surface as write error to client |
| `RecordDeleted` | Non-nil → surface as write error to client |
| `ConnectionClosed` | No return — always proceeds |
| `TransactionRolledBack` | No return — always proceeds |
| `QueryCompleted` | No return — fire-and-forget |
| `SchemaLoaded` | No return |
| `SchemaInvalidated` | No return |