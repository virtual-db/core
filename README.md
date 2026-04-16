# vdb-core

`vdb-core` is the Go framework that powers VirtualDB -- a database proxy that sits transparently in front of an existing database server. The framework intercepts every connection, transaction, query, and record-level operation, routes each through an ordered pipeline of handlers, and maintains an in-memory delta store that overlays committed writes on top of source data without modifying the underlying database.

`vdb-core` is the engine. It does not speak any wire protocol on its own. A driver (e.g. `vdb-mysql-driver`) implements the `Server` interface, receives the `DriverAPI` bridge, and calls back into the framework as database events occur.

---

## Requirements

- Go 1.23 or later
- Module path: `github.com/AnqorDX/vdb-core`

---

## Dependencies

| Module | Role |
|---|---|
| `github.com/AnqorDX/pipeline` | Ordered, priority-based pipeline engine |
| `github.com/AnqorDX/dispatch` | Fire-and-forget event bus |
| `gopkg.in/yaml.v3` | Plugin manifest parsing |

---

## Architecture Overview

```
                  +------------------------------------------+
                  |                 vdb-core                  |
                  |                                           |
  Driver ------->  |  DriverAPI  -->  Pipelines  -->  Delta  |
  (e.g. MySQL)    |                      |                    |
                  |               Event Bus                   |
                  |                      |                    |
                  |              Plugin Manager               |
                  +------------------------------------------+
                                         |
                              Out-of-process Plugins
                              (JSON-RPC 2.0 / Unix socket)
```

### Core subsystems

**Pipeline engine** -- Named `vdb.*` pipelines cover the full lifecycle of a database session. Each pipeline has an ordered sequence of named points. Handlers attach to individual points at a declared priority. Multiple handlers at the same point run in priority order; lower numbers run first.

**Event bus** -- Named `vdb.*` events are emitted after key operations complete. Event delivery is fire-and-forget; a subscriber error is logged but does not affect other subscribers or the emitting caller.

**Delta store** -- An in-memory store that records INSERT, UPDATE, and DELETE operations without forwarding them to the source database. On every read, the delta is overlaid on top of source records: updates replace matching source rows, tombstones suppress deleted rows, and net-new inserts are appended.

**Transaction isolation** -- When a connection opens a transaction (BEGIN), it receives a private staging delta (`TxDelta`). All writes within the transaction go to `TxDelta` and are invisible to other connections. On COMMIT, `TxDelta` is merged into the shared live delta using last-write-wins semantics. On ROLLBACK, `TxDelta` is discarded -- the live delta is never touched by in-transaction writes, so no undo work is required.

**Schema cache** -- Stores table column lists and primary key columns as reported by the driver. The cache is consulted during delta overlay to build stable record keys.

**Plugin system** -- Plugins are out-of-process executables. Each plugin directory contains a manifest (`manifest.json`, `manifest.yaml`, or `manifest.yml`) that declares the plugin name, version, launch command, and optional environment variables. At startup, the plugin manager launches each plugin as a subprocess, communicates over a Unix domain socket using JSON-RPC 2.0, and wires the plugin's declared pipeline handlers and event subscriptions into the live framework.

---

## Pipelines

Each pipeline is identified by a dot-separated name. Points within a pipeline are named `<pipeline>.<point>`.

### Lifecycle

| Pipeline | Points |
|---|---|
| `vdb.context.create` | `build_context` -> `contribute` -> `seal` -> `emit` |
| `vdb.server.start` | `build_context` -> `configure` -> `launch` -> `emit` |
| `vdb.server.stop` | `build_context` -> `drain` -> `halt` -> `emit` |

### Connection

| Pipeline | Points |
|---|---|
| `vdb.connection.opened` | `build_context` -> `accept` -> `track` -> `emit` |
| `vdb.connection.closed` | `build_context` -> `cleanup` -> `release` -> `emit` |

### Transaction

| Pipeline | Points |
|---|---|
| `vdb.transaction.begin` | `build_context` -> `authorize` -> `emit` |
| `vdb.transaction.commit` | `build_context` -> `apply` -> `emit` |
| `vdb.transaction.rollback` | `build_context` -> `apply` -> `emit` |

### Query

| Pipeline | Points |
|---|---|
| `vdb.query.received` | `build_context` -> `intercept` -> `emit` |

### Records

| Pipeline | Points |
|---|---|
| `vdb.records.source` | `build_context` -> `transform` -> `emit` |
| `vdb.records.merged` | `build_context` -> `transform` -> `emit` |

### Writes

| Pipeline | Points |
|---|---|
| `vdb.write.insert` | `build_context` -> `apply` -> `emit` |
| `vdb.write.update` | `build_context` -> `apply` -> `emit` |
| `vdb.write.delete` | `build_context` -> `apply` -> `emit` |

---

## Events

Events are emitted after their corresponding operation completes. Subscribers receive a typed payload.

| Event | Emitted after |
|---|---|
| `vdb.server.stopped` | Graceful shutdown completes |
| `vdb.connection.opened` | A new client connection is accepted |
| `vdb.connection.closed` | A client connection is released |
| `vdb.transaction.started` | A transaction begins |
| `vdb.transaction.committed` | A transaction commits |
| `vdb.transaction.rolledback` | A transaction rolls back |
| `vdb.query.completed` | A query finishes execution |
| `vdb.record.inserted` | A record is written to the delta as a new insert |
| `vdb.record.updated` | A record overlay is written to the delta |
| `vdb.record.deleted` | A tombstone is written to the delta |
| `vdb.schema.loaded` | Table schema is loaded into the cache |
| `vdb.schema.invalidated` | Cached table schema is invalidated |

---

## Public API

All public types and functions are in the root package `github.com/AnqorDX/vdb-core`. Nothing under `internal/` is part of the public API.

### Types

```go
type Config struct {
    // PluginDir is the directory scanned one level deep for plugin manifests
    // at startup. An empty string disables plugin loading.
    PluginDir string
}

// PointFunc is the handler signature for pipeline points.
type PointFunc func(ctx any, payload any) (any, any, error)

// EventFunc is the handler signature for event subscribers.
type EventFunc func(ctx any, payload any) error
```

### Server interface

Implemented by the driver. The framework calls `Run` and `Stop`.

```go
type Server interface {
    Run() error   // Binds the port and blocks until Stop is called.
    Stop() error  // Signals Run to return and releases the port.
}
```

### DriverAPI interface

Implemented by the framework. The driver calls these methods as database events occur.

```go
type DriverAPI interface {
    ConnectionOpened(id uint32, user, addr string) error
    ConnectionClosed(id uint32, user, addr string)
    TransactionBegun(connID uint32, readOnly bool) error
    TransactionCommitted(connID uint32) error
    TransactionRolledBack(connID uint32, savepoint string)
    QueryReceived(connID uint32, query, database string) (string, error)
    QueryCompleted(connID uint32, query string, rowsAffected int64, err error)
    RecordsSource(connID uint32, table string, records []map[string]any) ([]map[string]any, error)
    RecordsMerged(connID uint32, table string, records []map[string]any) ([]map[string]any, error)
    RecordInserted(connID uint32, table string, record map[string]any) (map[string]any, error)
    RecordUpdated(connID uint32, table string, old, new map[string]any) (map[string]any, error)
    RecordDeleted(connID uint32, table string, record map[string]any) error
    SchemaLoaded(table string, columns []string, pkCol string)
    SchemaInvalidated(table string)
}
```

### App

```go
// New creates a fully initialised App ready for configuration.
func New(cfg Config) *App

// DriverAPI returns the framework's DriverAPI implementation.
// Pass this to the driver constructor before calling UseDriver.
func (a *App) DriverAPI() DriverAPI

// UseDriver registers the database server. Must be called before Run.
func (a *App) UseDriver(s Server) *App

// Attach registers a handler at the named pipeline point with the given priority.
// Panics if the point name is not declared, or if called after Run.
func (a *App) Attach(point string, priority int, fn PointFunc) *App

// Subscribe registers a handler for the named event.
// Panics if called after Run. Logs a warning and drops if the event is undeclared.
func (a *App) Subscribe(event string, fn EventFunc) *App

// DeclareEvent declares a new event on the bus. Used by plugins and extensions
// that own events not in the standard vdb.* set.
func (a *App) DeclareEvent(event string)

// DeclarePipeline declares a new pipeline with the given point sequence.
// Used by plugins and extensions that own pipelines not in the standard vdb.* set.
func (a *App) DeclarePipeline(name string, pointNames []string)

// Emit dispatches payload to all subscribers of the named event.
func (a *App) Emit(event string, payload any)

// Process runs the named pipeline with the given payload.
func (a *App) Process(pipeline string, payload any) (any, error)

// Run executes the startup sequence and blocks until Stop is called or the
// server exits. May only be called once per App.
func (a *App) Run() error

// Stop executes graceful shutdown and unblocks Run. Idempotent.
func (a *App) Stop()
```

---

## Usage

### Minimal wiring

```go
package main

import (
    "log"

    core   "github.com/AnqorDX/vdb-core"
    driver "your/driver"
)

func main() {
    app := core.New(core.Config{
        PluginDir: "plugins",
    })
    app.UseDriver(driver.New(driverCfg, app.DriverAPI()))

    if err := app.Run(); err != nil {
        log.Fatalf("vdb: %v", err)
    }
}
```

### Attaching a pipeline handler

Handlers attach to a named point at a priority. The framework's built-in handlers register at priority 10. Use a lower number to run before them, a higher number to run after.

```go
app.Attach("vdb.query.received.intercept", 5, func(ctx any, payload any) (any, any, error) {
    p := payload.(payloads.QueryReceivedPayload)
    log.Printf("query from conn %d: %s", p.ConnectionID, p.Query)
    return ctx, payload, nil
})
```

### Subscribing to an event

```go
app.Subscribe("vdb.record.inserted", func(ctx any, payload any) error {
    log.Printf("insert: %+v", payload)
    return nil
})
```

### Handler context

Every `PointFunc` and `EventFunc` receives a `framework.HandlerContext` as its first argument. It carries a `GlobalContext` (the sealed, process-wide key-value store) and a `CorrelationID` that traces the causal chain across pipeline runs and event emissions.

The `GlobalContext` is populated during `vdb.context.create`. Handlers at `vdb.context.create.contribute` may store values in the provided `*framework.GlobalContextBuilder` before it is sealed.

---

## Plugin System

Plugins are standalone executables managed by the framework at runtime.

### Plugin directory layout

```
plugins/
L my-plugin/
    +-- manifest.json   (or manifest.yaml / manifest.yml)
    L   my-plugin       (the executable)
```

### Manifest fields

```json
{
  "name":    "my-plugin",
  "version": "1.0.0",
  "command": ["./my-plugin"],
  "env":     { "SOME_VAR": "value" }
}
```

### Startup handshake

1. The framework launches the plugin process with `VDB_SOCKET` set to the path of a Unix domain socket.
2. The plugin connects to the socket.
3. The plugin sends a JSON-RPC 2.0 `declare` notification listing the pipeline points it handles, the events it subscribes to, the events it declares, and any custom pipelines it owns.
4. The framework registers adapter handlers and subscriptions that forward calls to the plugin over the socket.
5. The framework sends a `shutdown` request before process exit; plugins that do not respond within the configured timeout are killed.

### Plugin JSON-RPC methods (host to plugin)

| Method | Direction | Description |
|---|---|---|
| `handle_pipeline_point` | request | Deliver a pipeline point invocation; plugin returns the modified payload |
| `handle_event` | notification | Deliver an event to the plugin |
| `shutdown` | request | Signal the plugin to exit cleanly |

### Plugin JSON-RPC methods (plugin to host)

| Method | Direction | Description |
|---|---|---|
| `declare` | notification | Sent once at startup; declares all handlers, subscriptions, and declarations |
| `emit_event` | request | Ask the host to emit a plugin-owned event onto the bus |

---

## Handler Priorities

Pipeline points accept multiple handlers at different priorities. Lower priority numbers run first.

| Priority range | Intended use |
|---|---|
| 1 - 9 | Pre-processing; runs before built-in framework handlers |
| 10 | Built-in framework handlers (reserved) |
| 11 - 99 | Post-processing; runs after built-in framework handlers |

---

## Startup Sequence

`App.Run()` executes the following steps in order:

1. **`vdb.context.create`** -- Builds and seals the process-wide `GlobalContext`. Handlers at `vdb.context.create.contribute` may contribute key-value pairs before sealing.
2. **Plugin connect** -- Scans `PluginDir`, launches plugin subprocesses, and wires their declared handlers and subscriptions.
3. **`vdb.server.start`** -- Starts the registered `Server` in a goroutine.
4. **Idle** -- Blocks until `Stop()` is called, the server exits, or `SIGTERM`/`SIGINT` is received.

### Shutdown sequence

`App.Stop()` executes:

1. **`vdb.server.stop`** -- Drains plugins (sends `shutdown` to each), then calls `Server.Stop()`.
2. Closes the internal shutdown channel, unblocking `Run()`.

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

---

## License

Elastic License 2.0 (ELv2). See [`LICENSE.md`](LICENSE.md) for the full text.
