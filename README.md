# vdb-core

The central framework for VirtualDB. `vdb-core` owns the request pipeline, event bus, in-memory mutation store (delta), transaction isolation, schema cache, connection tracking, and plugin runtime. It has no knowledge of any particular database wire protocol — that belongs to the driver layer above it.

```
vdb-mysql
  └── github.com/AnqorDX/vdb-mysql-driver
        └── github.com/AnqorDX/vdb-core          ← this module
              ├── github.com/AnqorDX/pipeline
              └── github.com/AnqorDX/dispatch
```

External dependencies are intentionally minimal: only the `pipeline` and `dispatch` primitives. No database drivers, no network libraries.

---

## Table of contents

1. [Concepts](#concepts)
2. [App lifecycle](#app-lifecycle)
3. [DriverAPI — the bridge](#driverapi--the-bridge)
4. [Pipelines](#pipelines)
5. [Events](#events)
6. [Extension points](#extension-points)
7. [Delta store](#delta-store)
8. [Transaction isolation](#transaction-isolation)
9. [Schema cache](#schema-cache)
10. [Plugin system](#plugin-system)
11. [Package map](#package-map)

---

## Concepts

### Pipeline

A pipeline is an ordered list of named **points**. Handlers attach to points at a numeric priority. When the pipeline is processed, all handlers at each point run in ascending priority order, passing `(ctx, payload)` through the chain. A handler returns a (possibly mutated) `(ctx, payload, error)` tuple; a non-nil error aborts the run immediately.

Points follow the naming convention `vdb.<pipeline>.<stage>`, e.g. `vdb.write.insert.apply`. The standard stages are `build_context`, the domain stage, and `emit`.

### Event bus

Events are fire-and-forget notifications emitted after a pipeline completes. Any number of subscribers can listen on a named event. A subscriber returning a non-nil error is logged but does not affect other subscribers or the emitting pipeline. Events are not retried.

### Delta store

The delta is a process-wide, concurrency-safe in-memory mutation store. It records every write that clients issue — inserts, updates, and deletes — without forwarding them to the source database. When the driver reads rows, the framework overlays the delta on top of the source result to produce the merged view that clients see.

### Transaction isolation

Each connection that opens a transaction receives a **private staging delta** (`TxDelta`). Writes inside the transaction go to `TxDelta` instead of the shared live delta, keeping them invisible to all other connections. `COMMIT` merges `TxDelta` into the live delta (last-write-wins). `ROLLBACK` simply nils `TxDelta` — because the live delta was never touched, no undo work is required.

---

## App lifecycle

```go
cfg := core.Config{PluginDir: "/opt/vdb/plugins"}
app := core.New(cfg)

// Obtain the DriverAPI bridge and hand it to the driver constructor.
api    := app.DriverAPI()
server := mydriver.New(driverConfig, api)

// Register custom pipeline handlers and event subscribers.
app.Attach("vdb.records.source.transform", 50, myTransformHandler)
app.Subscribe("vdb.record.inserted", myAuditLogger)

// Wire the driver and block until SIGTERM/SIGINT or Stop().
app.UseDriver(server)
if err := app.Run(); err != nil {
    log.Fatal(err)
}
```

`Attach`, `Subscribe`, `DeclareEvent`, and `DeclarePipeline` all panic if called after `Run` has started — the framework is intentionally configuration-time-only.

### Startup sequence inside `Run`

| Step | Pipeline | What happens |
|---|---|---|
| 1 | `vdb.context.create` | Builds and seals the immutable `GlobalContext`. |
| 2 | *(plugin manager)* | Discovers plugin subdirectories, launches subprocesses, and wires their declared handlers and subscriptions. |
| 3 | `vdb.server.start` | Configures and launches the database listener in a goroutine. |
| 4 | *(idle loop)* | Blocks until a OS signal, a server error, or `Stop()`. |

### Shutdown sequence inside `Stop`

`Stop` is idempotent. It runs `vdb.server.stop` (drain → halt → emit) then closes the internal shutdown channel, unblocking `Run`.

---

## DriverAPI — the bridge

`DriverAPI` is the interface the driver calls back into the framework. The framework implements it; the driver consumes it. Application code never implements it.

```go
type DriverAPI interface {
    // Connection lifecycle
    ConnectionOpened(id uint32, user, addr string) error
    ConnectionClosed(id uint32, user, addr string)

    // Transaction lifecycle
    TransactionBegun(connID uint32, readOnly bool) error
    TransactionCommitted(connID uint32) error
    TransactionRolledBack(connID uint32, savepoint string)

    // Query interception
    QueryReceived(connID uint32, query, database string) (string, error)
    QueryCompleted(connID uint32, query string, rowsAffected int64, err error)

    // Row read hooks
    RecordsSource(connID uint32, table string, records []map[string]any) ([]map[string]any, error)
    RecordsMerged(connID uint32, table string, records []map[string]any) ([]map[string]any, error)

    // Write hooks
    RecordInserted(connID uint32, table string, record map[string]any) (map[string]any, error)
    RecordUpdated(connID uint32, table string, old, new map[string]any) (map[string]any, error)
    RecordDeleted(connID uint32, table string, record map[string]any) error

    // Schema management
    SchemaLoaded(table string, columns []string, pkCol string)
    SchemaInvalidated(table string)
}
```

Each method fires one pipeline synchronously and returns when all handlers have run. `QueryReceived` is the exception — it can return a rewritten query string that the driver should execute instead of the original.

---

## Pipelines

Fourteen pipelines are declared at startup. Names, points, and the built-in handler registered at each point are listed below. Application code and plugins can attach additional handlers at any point.

### Lifecycle

#### `vdb.context.create`
| Point | Built-in handler |
|---|---|
| `vdb.context.create.build_context` | `framework.BuildContext` — stamps a fresh `CorrelationID`. |
| `vdb.context.create.contribute` | `lifecycle` — runs the `GlobalContextBuilder` contribution phase. |
| `vdb.context.create.seal` | `lifecycle` — seals the builder into an immutable `GlobalContext`. |
| `vdb.context.create.emit` | `lifecycle` — emits `vdb.server.started`. |

#### `vdb.server.start`
| Point | Built-in handler |
|---|---|
| `vdb.server.start.build_context` | `framework.BuildContext` |
| `vdb.server.start.configure` | `lifecycle` — validates that `UseDriver` was called. |
| `vdb.server.start.launch` | `lifecycle` — calls `Server.Run()` in a goroutine. |
| `vdb.server.start.emit` | *(emit point — no built-in subscriber)* |

#### `vdb.server.stop`
| Point | Built-in handler |
|---|---|
| `vdb.server.stop.build_context` | `framework.BuildContext` |
| `vdb.server.stop.drain` | `lifecycle` — initiates plugin shutdown. |
| `vdb.server.stop.halt` | `lifecycle` — calls `Server.Stop()`. |
| `vdb.server.stop.emit` | `lifecycle` — emits `vdb.server.stopped`. |

### Connection

#### `vdb.connection.opened`
| Point | Built-in handler |
|---|---|
| `vdb.connection.opened.build_context` | `framework.BuildContext` |
| `vdb.connection.opened.accept` | *(available for access-control handlers)* |
| `vdb.connection.opened.track` | `connection` — stores a new `Conn` entry in the connection state map. |
| `vdb.connection.opened.emit` | `emit` — emits `vdb.connection.opened`. |

#### `vdb.connection.closed`
| Point | Built-in handler |
|---|---|
| `vdb.connection.closed.build_context` | `framework.BuildContext` |
| `vdb.connection.closed.cleanup` | *(available for cleanup handlers)* |
| `vdb.connection.closed.release` | `connection` — removes the `Conn` entry. |
| `vdb.connection.closed.emit` | `emit` — emits `vdb.connection.closed`. |

### Transaction

#### `vdb.transaction.begin`
| Point | Built-in handler |
|---|---|
| `vdb.transaction.begin.build_context` | `framework.BuildContext` |
| `vdb.transaction.begin.authorize` | `transaction` — allocates a fresh private `TxDelta` on the connection. |
| `vdb.transaction.begin.emit` | `emit` — emits `vdb.transaction.started`. |

#### `vdb.transaction.commit`
| Point | Built-in handler |
|---|---|
| `vdb.transaction.commit.build_context` | `framework.BuildContext` |
| `vdb.transaction.commit.apply` | `transaction` — merges `TxDelta` into the live delta (last-write-wins), then nils `TxDelta`. |
| `vdb.transaction.commit.emit` | `emit` — emits `vdb.transaction.committed`. |

#### `vdb.transaction.rollback`
| Point | Built-in handler |
|---|---|
| `vdb.transaction.rollback.build_context` | `framework.BuildContext` |
| `vdb.transaction.rollback.apply` | `transaction` — nils `TxDelta` (live delta is untouched; no undo needed). |
| `vdb.transaction.rollback.emit` | `emit` — emits `vdb.transaction.rolledback`. |

### Query

#### `vdb.query.received`
| Point | Built-in handler |
|---|---|
| `vdb.query.received.build_context` | `framework.BuildContext` |
| `vdb.query.received.intercept` | `connection` — updates the tracked current database name for the connection. |
| `vdb.query.received.emit` | *(emit point — no built-in subscriber)* |

### Records (read path)

The read path fires two pipelines per table scan. `vdb.records.source` fires just after source rows are read and before the delta overlay; `vdb.records.merged` fires after the overlay with the final row set.

#### `vdb.records.source`
| Point | Built-in handler |
|---|---|
| `vdb.records.source.build_context` | `framework.BuildContext` |
| `vdb.records.source.transform` | `write` — applies the delta overlay. Pass-1: live delta. Pass-2 (if inside a transaction): connection's `TxDelta`, giving read-your-own-writes visibility. |
| `vdb.records.source.emit` | *(emit point — no built-in subscriber)* |

#### `vdb.records.merged`
| Point | Built-in handler |
|---|---|
| `vdb.records.merged.build_context` | `framework.BuildContext` |
| `vdb.records.merged.transform` | *(available for post-overlay transformation)* |
| `vdb.records.merged.emit` | *(emit point — no built-in subscriber)* |

### Writes

Each write pipeline fires synchronously as the database engine processes the statement. The `apply` point records the mutation in the appropriate delta (live or `TxDelta`).

#### `vdb.write.insert`
| Point | Built-in handler |
|---|---|
| `vdb.write.insert.build_context` | `framework.BuildContext` |
| `vdb.write.insert.apply` | `write` — calls `delta.ApplyInsert`. |
| `vdb.write.insert.emit` | `emit` — emits `vdb.record.inserted`. |

#### `vdb.write.update`
| Point | Built-in handler |
|---|---|
| `vdb.write.update.build_context` | `framework.BuildContext` |
| `vdb.write.update.apply` | `write` — calls `delta.ApplyUpdate` (or `ApplyUpdateWithFallback` when writing to `TxDelta`, to resolve stable keys across implicit transaction boundaries). |
| `vdb.write.update.emit` | `emit` — emits `vdb.record.updated`. |

#### `vdb.write.delete`
| Point | Built-in handler |
|---|---|
| `vdb.write.delete.build_context` | `framework.BuildContext` |
| `vdb.write.delete.apply` | `write` — calls `delta.ApplyDelete`. |
| `vdb.write.delete.emit` | `emit` — emits `vdb.record.deleted`. |

---

## Events

Twelve standard events are declared at startup. All use fire-and-forget delivery. Handlers that return a non-nil error are logged but do not propagate to callers.

| Event name | Fired by | Payload type |
|---|---|---|
| `vdb.server.stopped` | `vdb.server.stop.emit` | `payloads.ServerStoppedPayload` |
| `vdb.connection.opened` | `vdb.connection.opened.emit` | `payloads.ConnectionOpenedPayload` |
| `vdb.connection.closed` | `vdb.connection.closed.emit` | `payloads.ConnectionClosedPayload` |
| `vdb.transaction.started` | `vdb.transaction.begin.emit` | `payloads.TransactionBeginPayload` |
| `vdb.transaction.committed` | `vdb.transaction.commit.emit` | `payloads.TransactionCommitPayload` |
| `vdb.transaction.rolledback` | `vdb.transaction.rollback.emit` | `payloads.TransactionRollbackPayload` |
| `vdb.query.completed` | `QueryCompleted` on `DriverAPI` | `payloads.QueryCompletedPayload` |
| `vdb.record.inserted` | `vdb.write.insert.emit` | `payloads.WriteInsertPayload` |
| `vdb.record.updated` | `vdb.write.update.emit` | `payloads.WriteUpdatePayload` |
| `vdb.record.deleted` | `vdb.write.delete.emit` | `payloads.WriteDeletePayload` |
| `vdb.schema.loaded` | `SchemaLoaded` on `DriverAPI` | `payloads.SchemaLoadedPayload` |
| `vdb.schema.invalidated` | `SchemaInvalidated` on `DriverAPI` | `payloads.SchemaInvalidatedPayload` |

Plugins may declare additional events. Custom events must be declared (via `app.DeclareEvent` or the plugin's `declare` message) before any subscriber or emitter references them.

---

## Extension points

### Attaching a pipeline handler

```go
// Priority 50 runs after the built-in handler at priority 10.
app.Attach("vdb.records.source.transform", 50, func(ctx any, p any) (any, any, error) {
    payload := p.(payloads.RecordsSourcePayload)
    // inspect or mutate payload.Records
    return ctx, payload, nil
})
```

`PointFunc` signature: `func(ctx any, payload any) (any, any, error)`

`ctx` carries a `framework.HandlerContext` with `Global` (the sealed process-wide context) and `CorrelationID` (causal chain identifiers). Return the (possibly mutated) ctx and payload; return a non-nil error to abort the pipeline.

### Subscribing to an event

```go
app.Subscribe("vdb.record.inserted", func(ctx any, p any) error {
    payload := p.(payloads.WriteInsertPayload)
    log.Printf("inserted into %s: %v", payload.Table, payload.Record)
    return nil
})
```

### Declaring custom pipelines and events

```go
app.DeclarePipeline("acl.check", []string{
    "acl.check.build_context",
    "acl.check.evaluate",
    "acl.check.emit",
})
app.DeclareEvent("acl.denied")
```

Custom pipelines and events are first-class — plugins, application handlers, and the `App.Process`/`App.Emit` helpers can all use them.

---

## Delta store

The delta records three categories of mutation per table, all keyed by **`RecordKey`** — a canonical string derived from the sorted field values of a record.

| Category | Storage key | Description |
|---|---|---|
| **Inserts** | `RecordKey(current_state)` | Net-new rows with no counterpart in the source database. |
| **Updates** | `RecordKey(original_source_row)` — the *stable key* | Upsert overlays for existing source rows. The key never changes as the row is updated further. |
| **Tombstones** | Stable key | Deleted source rows. A tombstone supersedes any update overlay for the same row. |

A fourth map, `currentToStable`, tracks `RecordKey(current_state) → stable_key` so that chained updates can always resolve back to the original source key regardless of how many times the row has been modified.

### Overlay algorithm

When `vdb.records.source.transform` runs, it merges the delta over the source row slice:

1. For each source row, compute its `RecordKey`.
2. If the key appears in **tombstones** → drop the row.
3. If the key appears in **updates** → replace the row with the overlay value.
4. Otherwise → pass the source row through unchanged.
5. Append all **inserts** whose keys were not seen in the source rows.

### Delta operations

```go
d.ApplyInsert(table, record)                          // net-new row
d.ApplyUpdate(table, oldRecord, newRecord)            // overlay update
d.ApplyUpdateWithFallback(table, old, new, fallback)  // update with cross-boundary key resolution (see below)
d.ApplyDelete(table, record)                          // tombstone
d.Merge(src)                                          // COMMIT: replay src into d (last-write-wins)
```

All operations are safe for concurrent use.

---

## Transaction isolation

### Design

VirtualDB uses a **private staging delta** model rather than a traditional undo-log:

| Aspect | MySQL (InnoDB) | VDB |
|---|---|---|
| In-transaction writes go to | Shared buffer pool (undo log for rollback) | Per-connection private `TxDelta` |
| `ROLLBACK` mechanism | Apply undo log in reverse | Drop `TxDelta` — live delta was never touched |
| `COMMIT` mechanism | Mark undo log reclaimable (data already shared) | Merge `TxDelta` into live delta |
| Isolation | MVCC read views | Private `TxDelta` invisible to all other connections |

### Read-your-own-writes

When `vdb.records.source.transform` runs for a connection with an open transaction:

- **Pass 1** — overlay the live delta (committed state, visible to all connections).
- **Pass 2** — overlay the connection's `TxDelta` on top of pass-1 results, so the writing connection can immediately read back its own uncommitted changes.

Other connections skip pass 2 entirely.

### Chained updates across implicit transaction boundaries

GMS wraps every autocommit statement in its own implicit `BEGIN` / `COMMIT`. This means a second `UPDATE` whose `WHERE` clause targets a value written by the first `UPDATE` arrives in a freshly allocated `TxDelta` that has no `currentToStable` mapping for the intermediate row value.

`ApplyUpdateWithFallback` solves this: before acquiring the write lock, it reads the live delta's `currentToStable` to pre-resolve the stable key. The first UPDATE's mapping was committed into the live delta, so the second UPDATE can still find the original source key and write its overlay at the correct position.

### Known limitations

- **Last-write-wins on COMMIT** is the declared conflict policy. If two concurrent connections modify the same row and both commit, the later `COMMIT` wins with no error raised. Full row-level conflict detection is future work.
- **`RecordKey` includes all field values**, not just the primary key. Insert-level PK deduplication does not exist in the delta; `ON DUPLICATE KEY UPDATE` and duplicate `INSERT` detection require driver-level or engine-level handling.

---

## Schema cache

The schema cache maps table names to their column list and primary key column. It is populated by `SchemaLoaded` calls from the driver (typically triggered when the engine introspects a table for the first time) and cleared by `SchemaInvalidated`.

The delta overlay uses the schema cache to validate that a table's schema is known before applying mutations. An unknown table is skipped with a warning log rather than an error, so a missing cache entry degrades gracefully.

---

## Plugin system

Plugins are independent subprocesses discovered at `Run` time by scanning the configured `PluginDir` one level deep. Each subdirectory may contain a `manifest.json` or `manifest.yaml` file.

### Manifest format

```json
{
  "name":    "my-plugin",
  "version": "1.0.0",
  "command": ["/opt/plugins/my-plugin/bin/my-plugin"],
  "env":     { "LOG_LEVEL": "info" }
}
```

`command` is executed as a subprocess. The framework passes the Unix socket path via the `VDB_PLUGIN_SOCKET` environment variable (merged with `env`). The plugin must connect to that socket within 10 seconds.

### Connection protocol (JSON-RPC 2.0 over Unix socket)

The protocol is newline-delimited JSON-RPC 2.0. The plugin speaks first.

**Step 1 — Plugin sends `declare` notification**

```json
{
  "jsonrpc": "2.0",
  "method":  "declare",
  "params": {
    "plugin_id": "my-plugin",
    "pipeline_handlers": [
      { "point": "vdb.records.source.transform", "priority": 50 }
    ],
    "event_subscriptions":   ["vdb.record.inserted"],
    "event_declarations":    ["my-plugin.alert.fired"],
    "pipeline_declarations": []
  }
}
```

**Step 2 — Framework dispatches `handle_pipeline_point` requests**

When a handler registered by the plugin is reached during pipeline processing, the framework sends a synchronous request and waits for the response:

```json
// request
{ "jsonrpc": "2.0", "id": 1, "method": "handle_pipeline_point",
  "params": { "point": "vdb.records.source.transform", "payload": { ... } } }

// response
{ "jsonrpc": "2.0", "id": 1, "result": { "payload": { ... } } }
```

**Step 3 — Framework sends `handle_event` notifications (fire-and-forget)**

```json
{ "jsonrpc": "2.0", "method": "handle_event",
  "params": { "event": "vdb.record.inserted", "payload": { ... } } }
```

**Step 4 — Plugin may send `emit_event` requests**

A plugin can emit events it declared in its `event_declarations` list:

```json
{ "jsonrpc": "2.0", "id": 42, "method": "emit_event",
  "params": { "event": "my-plugin.alert.fired", "payload": { ... } } }
```

**Shutdown** — the framework sends a `shutdown` request and waits for the ack before killing the process.

### Plugin isolation

- A plugin that crashes does not crash the framework. The monitor goroutine logs the exit and marks the plugin as failed; its pipeline handler adapters remain registered but return errors.
- Shutdown is parallel across all plugins with a per-plugin timeout (default 10 s).

---

## Package map

```
github.com/AnqorDX/vdb-core         — public API (App, DriverAPI, Config, Server)
  internal/
    framework/                       — Pipeline and Bus wrappers; HandlerContext; GlobalContext
    points/                          — Canonical pipeline and event name constants
    delta/                           — Mutation store: ApplyInsert/Update/Delete, Merge, Overlay
    connection/                      — Per-connection state (Conn, State)
    transaction/                     — BEGIN/COMMIT/ROLLBACK handlers; TxDelta lifecycle
    write/                           — Write interception handlers; delta overlay (Overlay func)
    schema/                          — Table schema cache
    driverapi/                       — DriverAPI implementation; pipeline dispatch
    lifecycle/                       — Startup/shutdown pipeline handlers
    emit/                            — Standard event emission handlers
    plugin/                          — Plugin manager; manifest loading; JSON-RPC protocol
    payloads/                        — Payload structs for all pipelines and events
```

All packages under `internal/` are implementation details. Only the root `core` package is part of the public API.