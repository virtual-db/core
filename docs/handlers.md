# Handlers Reference

Technical reference for writing pipeline handlers and event subscribers in VirtualDB `core`.

---

## `PointFunc` — the handler signature

```/dev/null/example.go#L1-1
type PointFunc func(ctx any, payload any) (any, any, error)
```

Every handler attached to a pipeline point must conform to this signature.

| Parameter / return | Description |
|---|---|
| `ctx any` | A `framework.HandlerContext`. Type-assert to access `Global` and `CorrelationID`. |
| `payload any` | The typed payload struct for the pipeline. Varies per pipeline — see `pipelines.md`. |
| First return (`any`) | The `ctx` value to pass to the next handler. Pass it through unchanged unless you have a deliberate reason to replace it. |
| Second return (`any`) | The `payload` value to pass to the next handler. Return the (possibly modified) payload to propagate changes downstream. |
| `error` | Returning a non-nil error aborts all remaining points in the pipeline and propagates the error back to the `DriverAPI` caller. |

---

## `EventFunc` — the subscriber signature

```/dev/null/example.go#L1-1
type EventFunc func(ctx any, payload any) error
```

Subscribers registered on the event bus use this signature.

| Parameter / return | Description |
|---|---|
| `ctx any` | A `framework.HandlerContext`. |
| `payload any` | The typed payload struct for the event being delivered. |
| `error` | Returning a non-nil error is logged but does not abort delivery to other subscribers. |

---

## Handler context — `framework.HandlerContext`

Every handler receives a `framework.HandlerContext` as its `ctx any` argument. Type-assert it before use.

```/dev/null/example.go#L1-8
import "github.com/virtual-db/core/internal/framework"

func myHandler(ctx any, payload any) (any, any, error) {
    hctx := ctx.(framework.HandlerContext)
    _ = hctx.Global        // framework.GlobalContext
    _ = hctx.CorrelationID // framework.CorrelationID
    return ctx, payload, nil
}
```

### Fields

#### `Global framework.GlobalContext`

The sealed, immutable process-wide key-value store. Populated during the `vdb.context.create` lifecycle and available to all handlers for the entire process lifetime. It cannot be written to after the `seal` point completes.

Methods:

| Method | Description |
|---|---|
| `Get(key string) any` | Returns a value set during `vdb.context.create.contribute`, or `nil` if the key was never set. |
| `Bus() framework.EventBus` | Returns the event bus. Handlers can call `Bus().Emit(...)` to publish custom events. |

#### `CorrelationID framework.CorrelationID`

Tracing identifiers for the current operation. Stamped fresh by the built-in framework handler at priority 10 on every `build_context` point before any domain logic runs.

| Field | Type | Description |
|---|---|---|
| `Root` | `string` | UUID of the originating operation in the causal chain. |
| `Parent` | `string` | UUID of the immediately preceding operation. |
| `ID` | `string` | UUID of the current operation. |

---

## Priority system

Handlers are executed in ascending priority order within a pipeline point. Lower numbers run first.

| Priority range | Intended use |
|---|---|
| 1 – 9 | Pre-processing — runs before built-in framework handlers. |
| 10 | Built-in framework handlers (reserved). |
| 11 – 99 | Post-processing — runs after built-in framework handlers. |

Every pipeline point named `build_context` has a built-in framework handler at priority 10 that stamps a fresh `CorrelationID`. If you attach a handler at a priority lower than 10 on a `build_context` point, it will run before the `CorrelationID` has been set. If you need a valid `CorrelationID`, attach at priority 11 or higher.

---

## `GlobalContext` contributions

The `vdb.context.create.contribute` point is the **only** place where values can be written into the `GlobalContext`. The payload at this point is a `*framework.GlobalContextBuilder`. Once the `vdb.context.create.seal` point completes, the context is immutable for the rest of the process lifetime.

### Writing a value during startup

```/dev/null/example.go#L1-8
app.Attach("vdb.context.create.contribute", 5, func(ctx any, payload any) (any, any, error) {
    b := payload.(*framework.GlobalContextBuilder)
    b.Set("my-service-config", loadConfig())
    return ctx, payload, nil
})
```

- The key is an arbitrary string. Use a name that is unlikely to collide with other contributors.
- The value can be any type. Use a concrete pointer type for safe type-assertion on the read side.

### Reading a contributed value in any subsequent handler

```/dev/null/example.go#L1-9
app.Attach("vdb.query.received.intercept", 20, func(ctx any, payload any) (any, any, error) {
    hctx := ctx.(framework.HandlerContext)
    cfg := hctx.Global.Get("my-service-config").(*MyConfig)
    if cfg == nil {
        return ctx, payload, nil // key was not contributed; handle gracefully
    }
    // use cfg ...
    return ctx, payload, nil
})
```

`Get` returns `nil` if the key was never set. Always guard against nil before type-asserting when the contribution is optional or provided by another component.

---

## Common handler patterns

### 1. Query rewriting

Attach to `vdb.query.received.intercept` to inspect or rewrite the incoming SQL string before any processing occurs.

```/dev/null/example.go#L1-13
app.Attach("vdb.query.received.intercept", 5, func(ctx any, payload any) (any, any, error) {
    p := payload.(*pipelines.QueryReceivedPayload)

    // Replace a legacy table name with the current one.
    p.Query = strings.ReplaceAll(p.Query, "legacy_orders", "orders")

    return ctx, p, nil
})
```

Priority 5 ensures the rewrite happens before the framework parses or routes the query at priority 10.

### 2. Record filtering on read

Attach to `vdb.records.source.transform` to remove rows from the result set before they are returned to the driver.

```/dev/null/example.go#L1-18
app.Attach("vdb.records.source.transform", 20, func(ctx any, payload any) (any, any, error) {
    p := payload.(*pipelines.RecordsSourcePayload)

    filtered := p.Records[:0]
    for _, rec := range p.Records {
        if rec["deleted_at"] == nil {
            filtered = append(filtered, rec)
        }
    }
    p.Records = filtered

    return ctx, p, nil
})
```

### 3. Record enrichment on write

Attach to `vdb.write.insert.apply` to add or overwrite fields on a record before it is stored in the delta.

```/dev/null/example.go#L1-12
app.Attach("vdb.write.insert.apply", 20, func(ctx any, payload any) (any, any, error) {
    p := payload.(*pipelines.WriteInsertPayload)

    if p.Record["created_at"] == nil {
        p.Record["created_at"] = time.Now().UTC().Format(time.RFC3339)
    }

    return ctx, p, nil
})
```

### 4. Blocking a transaction

Attach to `vdb.transaction.begin.authorize` and return an error to deny the transaction before it is opened.

```/dev/null/example.go#L1-14
app.Attach("vdb.transaction.begin.authorize", 5, func(ctx any, payload any) (any, any, error) {
    hctx := ctx.(framework.HandlerContext)
    cfg := hctx.Global.Get("tx-policy").(*TxPolicy)

    if cfg.ReadOnly {
        return ctx, payload, errors.New("transactions are disabled in read-only mode")
    }

    return ctx, payload, nil
})
```

Returning a non-nil error here propagates back to the `DriverAPI` caller. The transaction is never opened.

### 5. Emitting a custom event from inside a handler

Use `hctx.Global.Bus().Emit(...)` to publish an event to all registered subscribers for a given topic.

```/dev/null/example.go#L1-15
app.Attach("vdb.write.insert.apply", 50, func(ctx any, payload any) (any, any, error) {
    hctx := ctx.(framework.HandlerContext)
    p := payload.(*pipelines.WriteInsertPayload)

    hctx.Global.Bus().Emit("my-app.record.inserted", &MyRecordInsertedEvent{
        Table:  p.Table,
        Record: p.Record,
    })

    return ctx, p, nil
})
```

Subscribers registered for `"my-app.record.inserted"` will be called synchronously before `Emit` returns. Errors returned by subscribers are logged and do not affect the handler that called `Emit`.

---

## Error handling behaviour summary

| Pipeline | Handler error behaviour |
|---|---|
| `vdb.query.received.*` | Aborts pipeline, error propagated to `DriverAPI` caller. |
| `vdb.write.insert.*` | Aborts pipeline, error propagated to `DriverAPI` caller. |
| `vdb.write.update.*` | Aborts pipeline, error propagated to `DriverAPI` caller. |
| `vdb.write.delete.*` | Aborts pipeline, error propagated to `DriverAPI` caller. |
| `vdb.transaction.begin.*` | Aborts pipeline, error propagated to `DriverAPI` caller. |
| `vdb.connection.opened.*` | Aborts pipeline, error propagated to `DriverAPI` caller. |
| `vdb.connection.closed.*` | Logged; caller proceeds normally. |
| `vdb.transaction.rollback.*` | Logged; caller proceeds normally. |
| `vdb.records.merged.*` | Logged; original (pre-merge) records are returned to the driver. |
| All event subscribers | Logged; remaining subscribers for the same event continue to run. |

When a handler error is "logged and caller proceeds", the error is recorded at `WARN` level with the pipeline name, point name, and handler priority attached as structured fields.