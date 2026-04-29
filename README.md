# core

`github.com/virtual-db/core`

The Go framework behind VirtualDB. `core` is a transparent intercepting proxy engine — it routes every database operation through an ordered pipeline, maintains an in-memory delta store that overlays writes on top of source data, and manages an event bus and plugin system. It does not speak any wire protocol itself.

A **driver** implements the `Server` interface and calls back into `DriverAPI` as database events occur. A **product** wires a driver to `core`, attaches handlers, and calls `Run`.

---

## Requirements

- Go 1.23+
- Module: `github.com/virtual-db/core`

---

## Quick Start

```go
app := core.New(core.Config{PluginDir: "plugins"})

api := app.DriverAPI()
srv := mydriver.New(cfg, api)

app.
    UseDriver(srv).
    Attach("vdb.query.received.intercept", 5, func(ctx any, p any) (any, any, error) {
        // inspect or rewrite queries before they hit the source DB
        return ctx, p, nil
    }).
    Subscribe("vdb.record.inserted", func(ctx any, p any) error {
        // react to inserts after they land in the delta
        return nil
    })

log.Fatal(app.Run())
```

---

## Core Concepts

**Pipeline** — Every database operation runs through a named `vdb.*` pipeline. Each pipeline has an ordered sequence of named points. Handlers attach to a point at a numeric priority; lower numbers run first. Built-in framework handlers reserve priority **10**.

**Event bus** — After key operations complete the framework emits a named `vdb.*` event. Delivery is fire-and-forget; a subscriber error is logged but does not stop other subscribers.

**Delta store** — Writes (INSERT, UPDATE, DELETE) are recorded in memory without touching the source database. On every read, the delta is overlaid on top of source records. Transactions get a private staging delta (`TxDelta`) that is merged into the live delta on COMMIT and discarded on ROLLBACK.

**Plugin system** — Plugins are out-of-process executables discovered from `PluginDir` at startup. Each communicates over a Unix socket using JSON-RPC 2.0 and declares which pipeline points it handles and which events it subscribes to.

---

## `App` API

| Method | Description |
|---|---|
| `New(cfg Config) *App` | Construct and initialise the framework. |
| `DriverAPI() DriverAPI` | Get the framework's `DriverAPI` bridge. Call before `UseDriver`. |
| `UseDriver(s Server) *App` | Register the database server. Must be called before `Run`. |
| `Attach(point string, priority int, fn PointFunc) *App` | Register a handler at a pipeline point. Panics on unknown point or after `Run`. |
| `Subscribe(event string, fn EventFunc) *App` | Register an event subscriber. Panics after `Run`. |
| `DeclareEvent(event string)` | Declare a custom event (for extensions and plugins). |
| `DeclarePipeline(name string, points []string)` | Declare a custom pipeline (for extensions and plugins). |
| `Emit(event string, payload any)` | Dispatch a payload to all subscribers of the named event. |
| `Process(pipeline string, payload any) (any, error)` | Run a pipeline manually. |
| `Run() error` | Execute the startup sequence and block. May only be called once. |
| `Stop()` | Graceful shutdown. Idempotent. Unblocks `Run`. |

---

## Interfaces

```go
// Implemented by the driver.
type Server interface {
    Run() error
    Stop() error
}

// Implemented by the framework. Called by the driver.
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

---

## Pipelines

| Pipeline | Points |
|---|---|
| `vdb.context.create` | `build_context` → `contribute` → `seal` → `emit` |
| `vdb.server.start` | `build_context` → `configure` → `launch` → `emit` |
| `vdb.server.stop` | `build_context` → `drain` → `halt` → `emit` |
| `vdb.connection.opened` | `build_context` → `accept` → `track` → `emit` |
| `vdb.connection.closed` | `build_context` → `cleanup` → `release` → `emit` |
| `vdb.transaction.begin` | `build_context` → `authorize` → `emit` |
| `vdb.transaction.commit` | `build_context` → `apply` → `emit` |
| `vdb.transaction.rollback` | `build_context` → `apply` → `emit` |
| `vdb.query.received` | `build_context` → `intercept` → `emit` |
| `vdb.records.source` | `build_context` → `transform` → `emit` |
| `vdb.records.merged` | `build_context` → `transform` → `emit` |
| `vdb.write.insert` | `build_context` → `apply` → `emit` |
| `vdb.write.update` | `build_context` → `apply` → `emit` |
| `vdb.write.delete` | `build_context` → `apply` → `emit` |

Use `<pipeline>.<point>` as the point name when calling `Attach` (e.g. `vdb.query.received.intercept`).

---

## Events

| Event | Emitted after |
|---|---|
| `vdb.server.stopped` | Graceful shutdown completes |
| `vdb.connection.opened` | A client connection is accepted |
| `vdb.connection.closed` | A client connection is released |
| `vdb.transaction.started` | A transaction begins |
| `vdb.transaction.committed` | A transaction commits |
| `vdb.transaction.rolledback` | A transaction rolls back |
| `vdb.query.completed` | A query finishes execution |
| `vdb.record.inserted` | A record lands in the delta as an insert |
| `vdb.record.updated` | A record overlay lands in the delta |
| `vdb.record.deleted` | A tombstone lands in the delta |
| `vdb.schema.loaded` | Table schema is loaded into the cache |
| `vdb.schema.invalidated` | Cached table schema is invalidated |

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Elastic License 2.0 (ELv2). See [LICENSE.md](LICENSE.md).