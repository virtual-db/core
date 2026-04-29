# Event Reference

VirtualDB core emits structured events over an internal event bus backed by [`github.com/AnqorDX/dispatch`](https://github.com/AnqorDX/dispatch).

## Fundamentals

- Events are **fire-and-forget**. A subscriber error is logged but does not stop other subscribers and does not affect the emitting caller.
- Subscribe to an event with `app.Subscribe(name, fn EventFunc)`.
- If the event name has not been declared, `Subscribe` logs a warning and drops the subscription silently.
- Declare a new event with `app.DeclareEvent(name)`.
- Emit a declared event with `app.Emit(name, payload)`.

The `EventFunc` signature:

```/dev/null/example.go#L1-3
type EventFunc func(ctx HandlerContext, payload any) error
```

Handlers receive `payload any` and must type-assert it to the expected payload type for the event.

---

## Standard Events

### `vdb.server.stopped`

Emitted after `Server.Stop()` returns and all plugins have exited.

**Payload:** none (`nil`).

---

### `vdb.connection.opened`

Emitted when a new client connection has been accepted and tracked.

| Field | Type | Description |
|---|---|---|
| `ConnectionID` | `uint32` | Unique ID assigned to the connection. |
| `User` | `string` | Authenticated username. |
| `Address` | `string` | Remote address of the client. |

---

### `vdb.connection.closed`

Emitted after a client connection has been released and cleaned up.

| Field | Type | Description |
|---|---|---|
| `ConnectionID` | `uint32` | ID of the connection that closed. |
| `User` | `string` | Authenticated username. |
| `Address` | `string` | Remote address of the client. |

---

### `vdb.transaction.started`

Emitted after a transaction has been successfully opened and a `TxDelta` allocated.

| Field | Type | Description |
|---|---|---|
| `ConnectionID` | `uint32` | Connection that opened the transaction. |
| `ReadOnly` | `bool` | Whether the transaction is read-only. |

---

### `vdb.transaction.committed`

Emitted after a transaction's `TxDelta` has been merged into the live delta.

| Field | Type | Description |
|---|---|---|
| `ConnectionID` | `uint32` | Connection that committed the transaction. |

---

### `vdb.transaction.rolledback`

Emitted after a transaction's `TxDelta` has been discarded.

| Field | Type | Description |
|---|---|---|
| `ConnectionID` | `uint32` | Connection that rolled back. |
| `Savepoint` | `string` | Savepoint name, or empty for a full rollback. |

---

### `vdb.query.completed`

Emitted after a query has been fully processed and a result returned to the driver.

| Field | Type | Description |
|---|---|---|
| `ConnectionID` | `uint32` | Connection that issued the query. |
| `Query` | `string` | The (possibly rewritten) query string. |
| `Database` | `string` | Target database name. |
| `DurationMs` | `float64` | Wall-clock execution time in milliseconds. |
| `RowsAffected` | `int64` | Number of rows affected or returned. |
| `Error` | `string` | Stringified query error, or empty on success. |

---

### `vdb.record.inserted`

Emitted after a record has been routed to the live delta or `TxDelta` via `vdb.write.insert`.

| Field | Type | Description |
|---|---|---|
| `ConnectionID` | `uint32` | Connection that performed the insert. |
| `Table` | `string` | Target table name. |
| `Record` | `map[string]any` | The inserted record. |

---

### `vdb.record.updated`

Emitted after an overlay has been written and stable-key tracking updated via `vdb.write.update`.

| Field | Type | Description |
|---|---|---|
| `ConnectionID` | `uint32` | Connection that performed the update. |
| `Table` | `string` | Target table name. |
| `OldRecord` | `map[string]any` | Record state before the update. |
| `NewRecord` | `map[string]any` | Record state after the update. |

---

### `vdb.record.deleted`

Emitted after a tombstone has been written and any prior update overlay removed via `vdb.write.delete`.

| Field | Type | Description |
|---|---|---|
| `ConnectionID` | `uint32` | Connection that performed the delete. |
| `Table` | `string` | Target table name. |
| `Record` | `map[string]any` | The record that was deleted. |

---

### `vdb.schema.loaded`

Emitted when schema metadata for a table has been loaded or refreshed.

| Field | Type | Description |
|---|---|---|
| `Table` | `string` | Table whose schema was loaded. |
| `Columns` | `[]string` | Ordered list of column names. |
| `PKCol` | `string` | Name of the primary key column. |

---

### `vdb.schema.invalidated`

Emitted when cached schema metadata for a table has been invalidated and must be re-fetched.

| Field | Type | Description |
|---|---|---|
| `Table` | `string` | Table whose schema was invalidated. |

---

## Custom Events

### Declaring and emitting from a product or plugin

Declare the event before any subscriber or emitter can use it. Declaration is typically done during plugin or product initialisation.

```/dev/null/example.go#L1-10
// Declare the event once, e.g. in plugin initialisation.
app.DeclareEvent("myplugin.cache.evicted")

// Emit the event from anywhere after declaration.
app.Emit("myplugin.cache.evicted", MyCacheEvictedPayload{
    Table:    "orders",
    KeyCount: 42,
})
```

Subscribe from any other component:

```/dev/null/example.go#L1-7
app.Subscribe("myplugin.cache.evicted", func(ctx HandlerContext, payload any) error {
    p := payload.(MyCacheEvictedPayload)
    log.Printf("cache evicted %d keys from table %s", p.KeyCount, p.Table)
    return nil
})
```

There is no requirement that custom event names begin with `vdb.`.

### Plugin declaration via JSON-RPC

Plugins that run as out-of-process JSON-RPC servers must declare any events they intend to emit inside their `declare` notification payload, under the `"events"` key. The framework registers those names at plugin load time so that subsequent `emit_event` calls are accepted.

```/dev/null/example.go#L1-14
// declare notification payload (sent by the plugin at startup)
{
    "events": [
        "myplugin.cache.evicted",
        "myplugin.sync.completed"
    ]
}

// emit_event JSON-RPC method call (sent by the plugin at runtime)
{
    "method": "emit_event",
    "params": {
        "name":    "myplugin.cache.evicted",
        "payload": { "Table": "orders", "KeyCount": 42 }
    }
}
```

If a plugin calls `emit_event` for a name it did not declare in its `declare` notification, the framework logs a warning and drops the emission.