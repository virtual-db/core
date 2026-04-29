# Startup and Shutdown Reference

Precise documentation of the `App.Run()` and `App.Stop()` sequences.

---

## Overview

`App.Run()` executes a fixed startup sequence and then blocks until the application is stopped. `App.Stop()` can be called from any goroutine at any time after `Run` starts.

`Run` may only be called once per `App` instance — a second call returns an error immediately without executing any part of the startup sequence.

---

## `App.Run()` — Full Sequence

### Step 1: `vdb.context.create` Pipeline *(host-only)*

A `*framework.GlobalContextBuilder` is created and the `vdb.context.create` pipeline executes through the following points in order:

| Point | Description |
|---|---|
| `build_context` | Initialises the builder. Internal. |
| `contribute` | **In-process extension point.** Handlers receive the builder as payload and may call `b.Set(key, value)` to store values into the global context before it is sealed. Handlers must be registered at priority < 10 to run before the seal handler. |
| `seal` | The builder is frozen into an immutable `GlobalContext` and stored on the `App`. All subsequent handlers for the lifetime of the process receive this context. Internal. |
| `emit` | Post-seal notification. No event is emitted here and no built-in handler is registered on this point. Internal. |

**This pipeline is host-only.** It runs synchronously in `App.Run()` before plugin discovery begins (Step 2). Because no plugin subprocess has been launched at this point, out-of-process plugin handlers declared in a `declare` notification will never be invoked on any point in this pipeline. The `contribute` point is the extension point for in-process code — drivers and embedders that call `app.Attach` before calling `app.Run()`.

If any handler in this pipeline returns an error, `Run` returns that error immediately and the startup sequence halts.

### Step 2: Plugin Connect

The framework scans the directory specified by `Config.PluginDir` one level deep. For each immediate subdirectory that contains a manifest file (`manifest.json`, `manifest.yaml`, or `manifest.yml`):

1. The plugin subprocess is launched.
2. The framework waits for the plugin to connect over its Unix domain socket and send a `declare` notification.
3. Declared handlers and event subscriptions are wired into the live pipeline and event bus.

Plugin failures (connection timeout, malformed `declare`, etc.) are logged and skipped. The startup sequence continues with the remaining plugins.

Plugin handlers are wired here, after `vdb.context.create` has already completed. All pipelines from Step 3 onward are plugin-accessible.

### Step 3: `vdb.server.start` Pipeline

The `vdb.server.start` pipeline executes through the following points in order:

| Point | Description |
|---|---|
| `build_context` | Prepares the server start context. |
| `configure` | Handlers may inspect or adjust server configuration before launch. |
| `launch` | At priority 10, the framework calls `Server.Run()` in a new goroutine. Errors from `Run()` are forwarded on an internal error channel. |
| `emit` | The `vdb.server.started` event is emitted on the event bus. |

If any handler in this pipeline returns an error, `Run` triggers `Stop()` and returns the handler error.

### Step 4: Idle Loop

`Run` blocks on a `select` waiting for one of the following conditions:

| Condition | Outcome |
|---|---|
| Internal shutdown channel is closed (by `Stop()`) | `Run()` returns `nil`. |
| `Server.Run()` sends an error on the error channel | `Stop()` is called, then `Run()` returns the server error. |
| `SIGTERM` or `SIGINT` is received | `Stop()` is called, then `Run()` returns `nil`. |

---

## `App.Stop()` — Full Sequence

`Stop()` is idempotent — calling it multiple times has no effect after the first call.

**The `vdb.server.stop` pipeline is host-only.** Plugin cleanup on shutdown is handled by the `shutdown` JSON-RPC request sent at the `drain` point — the plugin receives that request, performs any necessary cleanup, and exits. This is the correct and intended mechanism for plugin shutdown behaviour. There is no plugin pipeline expansion value in `vdb.server.stop` that is not already served by the `shutdown` RPC, so all points in this pipeline are internal to the framework shutdown sequence.

### Step 1: `vdb.server.stop` Pipeline — `drain` Point (priority 10)

The framework sends a `shutdown` JSON-RPC request to all live plugins concurrently. Plugins that do not respond within the configured timeout (default: 10 seconds) are sent `SIGKILL`. The framework waits for all plugin goroutines to exit before proceeding to the next step.

### Step 2: `vdb.server.stop` Pipeline — `halt` Point (priority 10)

The framework calls `Server.Stop()`. `Server.Stop()` must signal `Server.Run()` to return and release all held resources (listener sockets, open connections, etc.).

### Step 3: `vdb.server.stop` Pipeline — `emit` Point (priority 10)

The framework emits the `vdb.server.stopped` event on the event bus. Because all plugin processes were terminated in Step 1, out-of-process plugins will not receive this event.

### Step 4: Shutdown Channel Closed

The internal shutdown channel is closed, unblocking the idle `select` in `Run()` and allowing it to return.

---

## Concurrency Notes

- `Stop()` may be called before `Run()` has been called — this has no effect.
- `Stop()` may be called concurrently from multiple goroutines; only the first call executes the shutdown sequence. All subsequent calls return immediately.
- `Stop()` blocks until the full shutdown sequence completes before returning.
- `Run()` and `Stop()` are safe to call from different goroutines simultaneously.

---

## Error Propagation Summary

| Scenario | `Run()` return value |
|---|---|
| `Stop()` called (normal shutdown) | `nil` |
| OS signal (`SIGTERM` or `SIGINT`) | `nil` |
| `Server.Run()` returns an error | that error |
| Handler error in `vdb.context.create` | that error |
| Handler error in `vdb.server.start` | that error |
| `Run()` called a second time | error |