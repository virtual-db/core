# Pipeline Reference

VirtualDB's pipeline engine uses [`github.com/AnqorDX/pipeline`](https://github.com/AnqorDX/pipeline) internally.

## Concepts

### Pipelines and Points

A **pipeline** is a named, ordered sequence of **points**. Each point is a named slot within the pipeline. Pipelines are identified by dot-separated names (e.g. `vdb.query.received`).

### Handlers

A **handler** (`PointFunc`) attaches to a point at a numeric **priority**. When a pipeline runs, all handlers registered at each point execute in ascending priority order before the pipeline advances to the next point.

Multiple handlers may share the same point and priority; within a shared priority their execution order is not guaranteed.

### Attaching Handlers

```/dev/null/example.go#L1-5
app.Attach("vdb.query.received.intercept", 20, func(ctx any, payload any) (any, any, error) {
    p := payload.(*framework.QueryReceivedPayload)
    p.Query = strings.ReplaceAll(p.Query, "SELECT *", "SELECT id, name")
    return ctx, p, nil
})
```

The full point name passed to `app.Attach` is `<pipeline>.<point>` — e.g. `vdb.query.received.intercept`.

### Priority Conventions

| Priority range | Use |
|---|---|
| 1–9 | Run **before** the built-in framework handler |
| **10** | Reserved for built-in framework handlers |
| 11–99 | Run **after** the built-in framework handler |

### Error Behaviour

A handler returning a non-nil error **aborts all remaining points** in that pipeline and propagates the error back to the `DriverAPI` caller. The only exceptions are pipelines where the documentation below explicitly states that errors are logged and execution continues.

### `build_context` Points

Every pipeline includes a `build_context` point as its first point. The framework registers a handler at priority 10 on every `build_context` point. This handler stamps a fresh `CorrelationID` onto the `HandlerContext` before any domain logic runs.

### Payload Type Assertions

Handlers receive `payload any` and must type-assert it to the concrete type documented for the relevant point. The payload pointer is shared across all handlers in a pipeline run; mutations to the payload are visible to all subsequent handlers.

```/dev/null/example.go#L1-6
app.Attach("vdb.records.source.transform", 20, func(ctx any, payload any) (any, any, error) {
    p := payload.(*framework.RecordsSourcePayload)
    // p.Records already has the delta overlay applied (framework ran at priority 10)
    _ = p.Records
    return ctx, p, nil
})
```

---

## Host-Only Pipelines

These pipelines run during the startup or shutdown sequence, before or after out-of-process plugins are connected. **Out-of-process plugin handlers declared in a `declare` notification will never be invoked on these pipelines.**

In-process consumers that embed `core` as a library may still attach handlers via `app.Attach` — that is the intended and supported use for these pipelines.

---

### `vdb.context.create` *(host-only)*

Runs once at startup, synchronously in `App.Run()`, **before** `ConnectAll` is called. Constructs the process-wide `GlobalContext`.

**Why host-only:** This pipeline runs before any plugin subprocess has been launched. By definition, no out-of-process plugin handler can be registered in time to observe it. The `contribute` point is the extension point for in-process host code (drivers, embedders) that need to inject values into the `GlobalContext` before it is sealed.

| Point | Payload type | Notes |
|---|---|---|
| `build_context` | — | Framework stamps `CorrelationID`. Internal. |
| `contribute` | `*framework.GlobalContextBuilder` | **In-process extension point.** Call `b.Set(key, value)` to store values into the global context before it is sealed. Handlers must run at priority < 10 to execute before the seal handler. |
| `seal` | `*framework.GlobalContextBuilder` | Framework seals the context into an immutable `GlobalContext`. Internal. |
| `emit` | `*framework.GlobalContextBuilder` | Post-seal notification. Internal. No event is emitted here; no built-in handler is registered on this point. |

If any handler returns an error, `App.Run()` returns that error immediately and the startup sequence halts.

---

### `vdb.server.stop` *(host-only)*

Runs during `App.Stop()`, after the idle loop is unblocked but before `Run()` returns.

**Why host-only:** The `drain` point at priority 10 calls `plugin.Manager.Shutdown()`, which sends every connected plugin a `shutdown` JSON-RPC request and waits for their processes to exit. Plugin cleanup on shutdown is handled by that RPC protocol — the plugin receives a `shutdown` request, performs any necessary cleanup, and exits. All `vdb.server.stop` pipeline points are therefore internal to the framework shutdown sequence; there is no plugin expansion value here that is not already served by the `shutdown` RPC.

| Point | Payload type | Notes |
|---|---|---|
| `build_context` | — | Framework stamps `CorrelationID`. Internal. |
| `drain` | `*framework.ServerStopPayload` | Framework sends `shutdown` to all live plugins concurrently and waits for them to exit (priority 10). Internal. |
| `halt` | `*framework.ServerStopPayload` | Framework calls `Server.Stop()` (priority 10). Internal. |
| `emit` | `*framework.ServerStopPayload` | Framework fires `vdb.server.stopped` (priority 10). Internal. |

**`ServerStopPayload` fields:**

| Field | Type | Description |
|---|---|---|
| `Reason` | `string` | Human-readable reason for the shutdown. |

> **Note on `vdb.server.stopped` event:** Because plugins are terminated at the `drain` point, out-of-process plugins will not receive this event in practice. See the [Events reference](events.md) for details.

---

## Plugin-Accessible Pipelines

These pipelines run during normal operation, after all plugins have completed their startup handshake. Both in-process handlers (via `app.Attach`) and out-of-process handlers (declared in `declare`) may attach to these pipelines.

---

### `vdb.server.start`

Starts the server listener. Runs in `App.Run()` after all plugins have connected.

| Point | Payload type | Notes |
|---|---|---|
| `build_context` | — | Framework stamps `CorrelationID`. |
| `configure` | `*framework.ServerStartPayload` | Inspect or modify server configuration before the server is started. |
| `launch` | `*framework.ServerStartPayload` | Framework calls `Server.Run()` in a goroutine at priority 10. |
| `emit` | `*framework.ServerStartPayload` | Post-launch notification. |

**`ServerStartPayload` fields:**

| Field | Type | Description |
|---|---|---|
| `ListenAddr` | `string` | Address the server will bind to. |
| `DBName` | `string` | Name of the backing database. |
| `TLSConfig` | `any` | TLS configuration, or `nil` for plaintext. |
| `MaxConns` | `int` | Maximum number of concurrent connections. |

---

### `vdb.connection.opened`

Fires when a client connection is accepted.

| Point | Payload type | Notes |
|---|---|---|
| `build_context` | — | Framework stamps `CorrelationID`. |
| `accept` | `*framework.ConnectionOpenedPayload` | Initial acceptance logic. |
| `track` | `*framework.ConnectionOpenedPayload` | Framework registers the connection in the connection table at priority 10. |
| `emit` | `*framework.ConnectionOpenedPayload` | Framework fires `vdb.connection.opened` at priority 10. |

**`ConnectionOpenedPayload` fields:**

| Field | Type | Description |
|---|---|---|
| `ConnectionID` | `uint32` | Unique identifier for the connection. |
| `User` | `string` | Authenticated username. |
| `Address` | `string` | Remote address of the client. |

---

### `vdb.connection.closed`

Fires when a client connection is torn down.

| Point | Payload type | Notes |
|---|---|---|
| `build_context` | — | Framework stamps `CorrelationID`. |
| `cleanup` | `*framework.ConnectionClosedPayload` | Plugin cleanup logic. |
| `release` | `*framework.ConnectionClosedPayload` | Framework removes the connection from the connection table at priority 10. |
| `emit` | `*framework.ConnectionClosedPayload` | Framework fires `vdb.connection.closed` at priority 10. |

**`ConnectionClosedPayload` fields:**

| Field | Type | Description |
|---|---|---|
| `ConnectionID` | `uint32` | Identifier of the connection being closed. |
| `User` | `string` | Authenticated username. |
| `Address` | `string` | Remote address of the client. |

> **Note:** An error returned by a handler in this pipeline is **logged but does not affect the caller**. Connection teardown always proceeds.

---

### `vdb.transaction.begin`

Fires when a client begins a transaction.

| Point | Payload type | Notes |
|---|---|---|
| `build_context` | — | Framework stamps `CorrelationID`. |
| `authorize` | `*framework.TransactionBeginPayload` | Framework allocates a private `TxDelta` for the connection at priority 10. |
| `emit` | `*framework.TransactionBeginPayload` | Framework fires `vdb.transaction.started` at priority 10. |

**`TransactionBeginPayload` fields:**

| Field | Type | Description |
|---|---|---|
| `ConnectionID` | `uint32` | Connection opening the transaction. |
| `ReadOnly` | `bool` | Whether the transaction is read-only. |

---

### `vdb.transaction.commit`

Fires when a client commits a transaction.

| Point | Payload type | Notes |
|---|---|---|
| `build_context` | — | Framework stamps `CorrelationID`. |
| `apply` | `*framework.TransactionCommitPayload` | Framework merges `TxDelta` into the live delta at priority 10. |
| `emit` | `*framework.TransactionCommitPayload` | Framework fires `vdb.transaction.committed` at priority 10. |

**`TransactionCommitPayload` fields:**

| Field | Type | Description |
|---|---|---|
| `ConnectionID` | `uint32` | Connection committing the transaction. |

---

### `vdb.transaction.rollback`

Fires when a client rolls back a transaction.

| Point | Payload type | Notes |
|---|---|---|
| `build_context` | — | Framework stamps `CorrelationID`. |
| `apply` | `*framework.TransactionRollbackPayload` | Framework discards `TxDelta` at priority 10. |
| `emit` | `*framework.TransactionRollbackPayload` | Framework fires `vdb.transaction.rolledback` at priority 10. |

**`TransactionRollbackPayload` fields:**

| Field | Type | Description |
|---|---|---|
| `ConnectionID` | `uint32` | Connection rolling back. |
| `Savepoint` | `string` | Savepoint name, or empty string for a full rollback. |

> **Note:** An error returned by a handler at the `apply` point is **logged** and the caller proceeds normally.

---

### `vdb.query.received`

The primary interception point for query rewriting. Runs when a query string is received from the driver before execution.

| Point | Payload type | Notes |
|---|---|---|
| `build_context` | — | Framework stamps `CorrelationID`. |
| `intercept` | `*framework.QueryReceivedPayload` | Handlers may rewrite `payload.Query`. |
| `emit` | `*framework.QueryReceivedPayload` | Post-intercept notification. |

**`QueryReceivedPayload` fields:**

| Field | Type | Description |
|---|---|---|
| `ConnectionID` | `uint32` | Connection that submitted the query. |
| `Query` | `string` | The query string. Mutations to this field are honoured. |
| `Database` | `string` | Target database name. |

The value of `Query` on the payload after all `intercept` handlers have run is returned as the string return value of `DriverAPI.QueryReceived`. This is the mechanism for query rewriting.

---

### `vdb.records.source`

Fires when the framework is about to return a set of records to the driver from the backing store.

| Point | Payload type | Notes |
|---|---|---|
| `build_context` | — | Framework stamps `CorrelationID`. |
| `transform` | `*framework.RecordsSourcePayload` | Framework applies the delta overlay at priority 10. Handlers at 11+ see post-overlay records. |
| `emit` | `*framework.RecordsSourcePayload` | Post-transform notification. |

**`RecordsSourcePayload` fields:**

| Field | Type | Description |
|---|---|---|
| `ConnectionID` | `uint32` | Originating connection. |
| `Table` | `string` | Table the records originate from. |
| `Records` | `[]map[string]any` | The record set. The framework overlay (live delta + `TxDelta` if in-transaction) is applied at priority 10. |

---

### `vdb.records.merged`

A later-stage hook that runs after `vdb.records.source` has completed. Intended for post-overlay inspection or supplemental transformation.

| Point | Payload type | Notes |
|---|---|---|
| `build_context` | — | Framework stamps `CorrelationID`. |
| `transform` | `*framework.RecordsMergedPayload` | Secondary transformation opportunity. |
| `emit` | `*framework.RecordsMergedPayload` | Post-transform notification. |

**`RecordsMergedPayload` fields:**

| Field | Type | Description |
|---|---|---|
| `ConnectionID` | `uint32` | Originating connection. |
| `Table` | `string` | Table the records originate from. |
| `Records` | `[]map[string]any` | The post-overlay record set. |

> **Note:** Errors from handlers in this pipeline are **logged**. The original pre-pipeline records are returned to the driver.

---

### `vdb.write.insert`

Fires when a record is inserted via the `DriverAPI`.

| Point | Payload type | Notes |
|---|---|---|
| `build_context` | — | Framework stamps `CorrelationID`. |
| `apply` | `*framework.WriteInsertPayload` | Framework routes the record to `TxDelta` (if in-transaction) or the live delta at priority 10. |
| `emit` | `*framework.WriteInsertPayload` | Framework fires `vdb.record.inserted` at priority 10. |

**`WriteInsertPayload` fields:**

| Field | Type | Description |
|---|---|---|
| `ConnectionID` | `uint32` | Connection performing the insert. |
| `Table` | `string` | Target table. |
| `Record` | `map[string]any` | The record to insert. |

---

### `vdb.write.update`

Fires when a record is updated via the `DriverAPI`.

| Point | Payload type | Notes |
|---|---|---|
| `build_context` | — | Framework stamps `CorrelationID`. |
| `apply` | `*framework.WriteUpdatePayload` | Framework writes an overlay and updates stable-key tracking at priority 10. |
| `emit` | `*framework.WriteUpdatePayload` | Framework fires `vdb.record.updated` at priority 10. |

**`WriteUpdatePayload` fields:**

| Field | Type | Description |
|---|---|---|
| `ConnectionID` | `uint32` | Connection performing the update. |
| `Table` | `string` | Target table. |
| `OldRecord` | `map[string]any` | The record state before the update. |
| `NewRecord` | `map[string]any` | The record state after the update. |

---

### `vdb.write.delete`

Fires when a record is deleted via the `DriverAPI`.

| Point | Payload type | Notes |
|---|---|---|
| `build_context` | — | Framework stamps `CorrelationID`. |
| `apply` | `*framework.WriteDeletePayload` | Framework writes a tombstone and removes any prior update overlay at priority 10. |
| `emit` | `*framework.WriteDeletePayload` | Framework fires `vdb.record.deleted` at priority 10. |

**`WriteDeletePayload` fields:**

| Field | Type | Description |
|---|---|---|
| `ConnectionID` | `uint32` | Connection performing the delete. |
| `Table` | `string` | Target table. |
| `Record` | `map[string]any` | The record being deleted. |

---

## Custom Pipelines

Products and plugins may declare their own pipelines using `app.DeclarePipeline`. Custom pipeline names do not need to begin with `vdb.`.

```/dev/null/example.go#L1-12
// Declare a custom pipeline with three points.
app.DeclarePipeline("myplugin.enrich", []string{
    "build_context",
    "fetch",
    "transform",
    "emit",
})

// Run the pipeline with a payload.
err := app.Process("myplugin.enrich", &MyEnrichPayload{
    ConnectionID: connID,
})
```

Attach handlers to custom pipeline points the same way as built-in ones:

```/dev/null/example.go#L1-5
app.Attach("myplugin.enrich.fetch", 10, func(ctx any, payload any) (any, any, error) {
    p := payload.(*MyEnrichPayload)
    p.ExtraData = fetchFromRemote(p.ConnectionID)
    return ctx, p, nil
})
```

---

## Pipeline Accessibility Summary

| Pipeline | Accessible to plugins? | Reason |
|---|---|---|
| `vdb.context.create` | **No** — host-only | Runs before plugins connect |
| `vdb.server.stop` | **No** — host-only | Plugin cleanup uses the shutdown RPC, not pipeline hooks |
| `vdb.server.start` | Yes | Runs after all plugins have connected |
| `vdb.connection.opened` | Yes | Normal operation |
| `vdb.connection.closed` | Yes | Normal operation |
| `vdb.transaction.begin` | Yes | Normal operation |
| `vdb.transaction.commit` | Yes | Normal operation |
| `vdb.transaction.rollback` | Yes | Normal operation |
| `vdb.query.received` | Yes | Normal operation |
| `vdb.records.source` | Yes | Normal operation |
| `vdb.records.merged` | Yes | Normal operation |
| `vdb.write.insert` | Yes | Normal operation |
| `vdb.write.update` | Yes | Normal operation |
| `vdb.write.delete` | Yes | Normal operation |

---

## Payload Type Reference

| Payload type | Pipeline |
|---|---|
| `*framework.GlobalContextBuilder` | `vdb.context.create` *(host-only)* |
| `*framework.ServerStartPayload` | `vdb.server.start` |
| `*framework.ServerStopPayload` | `vdb.server.stop` *(host-only)* |
| `*framework.ConnectionOpenedPayload` | `vdb.connection.opened` |
| `*framework.ConnectionClosedPayload` | `vdb.connection.closed` |
| `*framework.TransactionBeginPayload` | `vdb.transaction.begin` |
| `*framework.TransactionCommitPayload` | `vdb.transaction.commit` |
| `*framework.TransactionRollbackPayload` | `vdb.transaction.rollback` |
| `*framework.QueryReceivedPayload` | `vdb.query.received` |
| `*framework.RecordsSourcePayload` | `vdb.records.source` |
| `*framework.RecordsMergedPayload` | `vdb.records.merged` |
| `*framework.WriteInsertPayload` | `vdb.write.insert` |
| `*framework.WriteUpdatePayload` | `vdb.write.update` |
| `*framework.WriteDeletePayload` | `vdb.write.delete` |

### Field summary

**`ServerStartPayload`** — `ListenAddr string`, `DBName string`, `TLSConfig any`, `MaxConns int`

**`ServerStopPayload`** — `Reason string`

**`ConnectionOpenedPayload`** — `ConnectionID uint32`, `User string`, `Address string`

**`ConnectionClosedPayload`** — `ConnectionID uint32`, `User string`, `Address string`

**`TransactionBeginPayload`** — `ConnectionID uint32`, `ReadOnly bool`

**`TransactionCommitPayload`** — `ConnectionID uint32`

**`TransactionRollbackPayload`** — `ConnectionID uint32`, `Savepoint string`

**`QueryReceivedPayload`** — `ConnectionID uint32`, `Query string`, `Database string`

**`RecordsSourcePayload`** — `ConnectionID uint32`, `Table string`, `Records []map[string]any`

**`RecordsMergedPayload`** — `ConnectionID uint32`, `Table string`, `Records []map[string]any`

**`WriteInsertPayload`** — `ConnectionID uint32`, `Table string`, `Record map[string]any`

**`WriteUpdatePayload`** — `ConnectionID uint32`, `Table string`, `OldRecord map[string]any`, `NewRecord map[string]any`

**`WriteDeletePayload`** — `ConnectionID uint32`, `Table string`, `Record map[string]any`
