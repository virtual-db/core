# Plugin System Reference

## Overview

Plugins extend `core` as out-of-process executables. They attach to pipeline points and subscribe to events just like in-process handlers, but run in a separate process and communicate over a Unix domain socket using JSON-RPC 2.0 with line-delimited framing.

The framework manages the full plugin lifecycle: discovery, launch, handshake, wiring, and shutdown.

---

## Plugin Directory Layout

The `PluginDir` config field points to the top-level plugins directory. The framework scans one level deep — each immediate subdirectory is treated as a potential plugin. A subdirectory without a manifest file is silently skipped.

```
plugins/
└── my-plugin/
    ├── manifest.json    (or manifest.yaml / manifest.yml)
    └── my-plugin        (the executable)
```

---

## Manifest

The manifest describes how to launch the plugin. It must be named `manifest.json`, `manifest.yaml`, or `manifest.yml` and placed in the plugin's subdirectory.

```
{
  "name":    "my-plugin",
  "version": "1.0.0",
  "command": ["./my-plugin", "--config", "/etc/my-plugin.yaml"],
  "env":     {
    "MY_PLUGIN_TOKEN": "secret"
  }
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Human-readable name. Used in log messages. |
| `version` | string | yes | Informational version string. |
| `command` | []string | yes | Executable and arguments. Relative paths are resolved from the plugin's subdirectory. |
| `env` | map[string]string | no | Additional environment variables injected into the plugin process. |

The framework always injects `VDB_SOCKET` into the plugin environment with the path of the Unix domain socket the plugin must connect to. This variable must not be specified manually in `env`.

---

## Startup Handshake Sequence

1. Framework creates a Unix socket at a temporary path and sets `VDB_SOCKET` in the plugin's environment.
2. Framework launches the plugin subprocess using `command` from the manifest.
3. Plugin connects to the socket within the configured timeout (default: 10 seconds). If the plugin does not connect in time it is killed and skipped — the framework continues with remaining plugins.
4. Plugin sends a single `declare` JSON-RPC 2.0 **notification** (no `id` field) immediately after connecting.
5. Framework reads the `declare` payload and wires adapter handlers and subscriptions into the live pipeline and event bus.
6. Framework starts a background read loop for the plugin's socket connection.

Steps 3–5 occur concurrently across all discovered plugins. The framework waits for all plugins to finish their handshake (or time out) before proceeding with the rest of the startup sequence.

> **Host-only pipelines:** `vdb.context.create` runs in Step 1 of `App.Run()`, before plugin discovery begins. `vdb.server.stop` runs during `App.Stop()`, after all plugin processes have been terminated via the `shutdown` RPC. Handlers declared in a plugin's `declare` notification will never be invoked on either of these pipelines. See the [Pipeline reference](pipelines.md) for the full accessibility table.

---

## The `declare` Notification

Sent by the plugin exactly once, immediately after connecting. This is a JSON-RPC 2.0 notification: it has no `id` field and no response is expected from the host.

**Method:** `declare`

### Params fields

| Field | Type | Description |
|---|---|---|
| `plugin_id` | string | Unique identifier for this plugin instance. Used in log messages and error attribution. |
| `pipeline_handlers` | []HandlerEntry | Pipeline points the plugin wants to handle. |
| `event_subscriptions` | []string | Event names the plugin subscribes to. |
| `event_declarations` | []string | New event names this plugin will emit via `emit_event`. |
| `pipeline_declarations` | []PipelineDecl | New pipelines this plugin declares. |

**HandlerEntry fields:**

| Field | Type | Description |
|---|---|---|
| `point` | string | Fully-qualified pipeline point name (e.g. `vdb.query.received.intercept`). |
| `priority` | int | Execution priority within the point. Lower values run first. |

**PipelineDecl fields:**

| Field | Type | Description |
|---|---|---|
| `name` | string | Fully-qualified pipeline name. |
| `description` | string | Human-readable description. |
| `points` | []string | Ordered list of point names that make up the pipeline. |

### Example

```
{
  "jsonrpc": "2.0",
  "method": "declare",
  "params": {
    "plugin_id": "my-plugin-1",
    "pipeline_handlers": [
      { "point": "vdb.query.received.intercept", "priority": 5 }
    ],
    "event_subscriptions": ["vdb.record.inserted"],
    "event_declarations": ["my-plugin.record.processed"],
    "pipeline_declarations": []
  }
}
```

---

## JSON-RPC Methods — Host → Plugin

These methods are sent by the framework to the plugin over the Unix socket.

### `handle_pipeline_point` (request)

Delivers a pipeline point invocation to the plugin. The plugin must send a JSON-RPC response containing the (possibly modified) payload. The framework blocks the pipeline until it receives the response or the call times out.

**Params:**

```
{
  "point":   "vdb.query.received.intercept",
  "ctx":     { ... },
  "payload": {
    "ConnectionID": 1,
    "Query":        "SELECT 1",
    "Database":     "mydb"
  }
}
```

- `point` — the fully-qualified pipeline point name.
- `ctx` — a JSON-serialised snapshot of the current pipeline context. Read-only.
- `payload` — the pipeline payload for this point. The plugin may return a modified copy.

**Response result:**

```
{
  "payload": {
    "ConnectionID": 1,
    "Query":        "SELECT 2",
    "Database":     "mydb"
  },
  "error": null
}
```

- `payload` — the (possibly rewritten) payload. Must have the same structure as the input payload.
- `error` — if non-null, the error message is propagated through the pipeline as if a normal in-process handler had returned an error.

If the plugin returns a JSON-RPC error (i.e. sets the top-level `error` field of the response object), that error is also propagated through the pipeline.

### `handle_event` (notification)

Delivers an event to the plugin. No response is expected or awaited. If the plugin is unreachable or slow, delivery is best-effort.

**Params:**

```
{
  "event":   "vdb.record.inserted",
  "payload": {
    "ConnectionID": 1,
    "Table":        "users",
    "Record":       { "id": 42, "email": "user@example.com" }
  }
}
```

### `shutdown` (request)

Signals the plugin to clean up and exit. The plugin should perform any necessary cleanup, send a success response, then exit. The framework waits for the plugin process to exit up to the configured timeout (default: 10 seconds) before sending `SIGKILL`.

**Params:** none (empty object or omitted).

**Response result:** any value (ignored by the framework).

---

## JSON-RPC Methods — Plugin → Host

These methods are sent by the plugin to the framework.

### `declare` (notification)

Sent once at startup immediately after connecting. See [The `declare` Notification](#the-declare-notification) above.

### `emit_event` (request)

Asks the host to emit a plugin-owned event on the framework event bus. The event name must appear in the `event_declarations` field of the plugin's `declare` payload. The framework responds once the event has been dispatched to all subscribers.

**Params:**

```
{
  "event":   "my-plugin.record.processed",
  "payload": { "table": "users", "count": 3 }
}
```

**Response result:** empty object on success. A JSON-RPC error is returned if the event name was not declared or the framework could not dispatch it.

---

## Pipeline and Event Accessibility

Not all pipelines are accessible to out-of-process plugins. Two pipelines are **host-only** and will never invoke plugin handlers:

| Pipeline | Reason |
|---|---|
| `vdb.context.create` | Runs before plugins are launched. No plugin handler can be registered in time. The `contribute` point is an in-process extension point for host code that embeds `core` as a library. |
| `vdb.server.stop` | Plugin cleanup on shutdown is handled by the `shutdown` RPC (see below), not by pipeline hooks. All points in this pipeline are internal to the framework shutdown sequence. |

All other built-in pipelines (`vdb.server.start`, `vdb.connection.*`, `vdb.transaction.*`, `vdb.query.*`, `vdb.records.*`, `vdb.write.*`) are fully accessible to plugins.

Regarding events: all 12 standard events are declared on the bus and may be subscribed to in a `declare` notification. The `vdb.server.stopped` event is emitted at the end of the `vdb.server.stop` pipeline — because plugins are terminated before that point is reached, out-of-process plugins will not receive this event in practice.

---

## Shutdown

During `App.Stop()`, the framework sends `shutdown` to all live plugins concurrently. Plugins that do not exit within the configured timeout (default: 10 seconds) are sent `SIGKILL`. The shutdown of all plugins completes before `Server.Stop()` is called.

Plugins should listen for `shutdown` and exit cleanly rather than relying on `SIGKILL`. Ungraceful termination may leave work mid-flight and produce log noise.

---

## Plugin Failure Handling

| Scenario | Framework behaviour |
|---|---|
| Plugin does not connect within timeout | Logged, plugin process killed, framework continues startup. |
| `declare` notification is malformed or missing required fields | Logged, plugin process killed, framework continues startup. |
| Plugin crashes during normal operation | Logged. The framework does not attempt to restart crashed plugins. |
| `handle_pipeline_point` returns an error | Error propagates through the pipeline as if a normal handler returned it. |
| `handle_event` fails or plugin is unreachable | Logged. Other event subscribers are still notified. |
| `emit_event` names an undeclared event | JSON-RPC error returned to the plugin. |

Crashed-plugin detection is best-effort based on socket read/write failures. There is no heartbeat mechanism.

---

## Framing

All JSON-RPC messages are delimited by a single newline character (`\n`). Each message is a complete JSON object on one line with no embedded newlines. Both host and plugin must adhere to this framing.

Plugins must not write partial messages or multi-line JSON to the socket.

---

## Concurrency

- The framework may send multiple `handle_pipeline_point` requests to the same plugin concurrently if multiple connections are active. Plugins must be safe to call re-entrantly.
- `handle_event` notifications may arrive concurrently with `handle_pipeline_point` requests.
- The framework serialises the `declare` read: the plugin must send `declare` and only `declare` as its first message. Any other message sent before `declare` will cause the plugin to be killed.