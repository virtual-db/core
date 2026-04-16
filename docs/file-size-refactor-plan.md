# vdb-core — File Size Refactor Plan

> **Status: DRAFT — pending implementation**

---

## 1. Context

Invariant #9 of the structural refactor plan states:

> No file exceeds 200 lines.

This is not a cosmetic rule. Files that exceed 200 lines are a signal that a
single file owns more than one domain concern. When a file serves more than one
concern, changes to one concern create accidental noise in diffs for another;
developers must scan past unrelated code to find what they need; and unit tests
that want to exercise one concern must instantiate all concerns the file touches.

This document audits every production source file in vdb-core, identifies the five
files that currently violate the invariant, diagnoses the entangled concerns in
each, and specifies the target file structure to correct them.

The rule of thumb applied throughout: **split on domain boundaries, not on
line counts**. A correct split produces files where every line in the file
belongs to the same conceptual concern. A split that happens to reduce the line
count without separating concerns is not a correct split.

---

## 2. Current Audit

```
internal/plugin/manager.go     347 lines  ← VIOLATION
app.go                         293 lines  ← VIOLATION
driver_api.go                  282 lines  ← VIOLATION
internal/plugin/codec.go       269 lines  ← VIOLATION
internal/delta/delta.go        271 lines  ← VIOLATION
internal/points/names.go       126 lines  ✓
internal/framework/context.go   96 lines  ✓
internal/emit/handlers.go      150 lines  ✓
internal/write/handlers.go     117 lines  ✓
internal/lifecycle/handlers.go 108 lines  ✓
internal/framework/pipeline.go  76 lines  ✓
internal/transaction/handlers.go 97 lines ✓
internal/connection/handlers.go  78 lines ✓
internal/framework/bus.go        52 lines ✓
internal/connection/state.go     61 lines ✓
internal/write/overlay.go        60 lines ✓
internal/schema/cache.go         55 lines ✓
internal/framework/correlation.go 34 lines ✓
app_api.go                       62 lines ✓
interfaces.go                    57 lines ✓
(all payload files)             < 30 lines ✓
```

Five files require attention. No other production file is above 200 lines.

---

## 3. Diagnosis and Plan

### 3.1 `internal/plugin/manager.go` — 347 lines

**What is entangled here:**

`manager.go` serves four distinct concerns:

1. **Manifest loading** — reading `manifest.json` / `manifest.yaml` / `manifest.yml`
   from disk, parsing them into a `Manifest` struct, and assembling the OS
   environment for the subprocess. This is pure file I/O and format parsing.
   It has no dependency on anything else in the plugin package.

2. **Process launching** — creating a Unix-domain socket, starting the subprocess
   via `exec.Cmd`, and accepting the subprocess's initial connection within a
   deadline. This is pure OS-level work: sockets, processes, timeouts. It takes
   a `Manifest` in and gives a `*pluginInstance` and a `net.Conn` out. Nothing
   about pipelines, events, or framework types.

3. **Protocol declare handling** — reading the first JSON-RPC message from the
   new connection, parsing it as a `declare` notification, and registering
   pipeline handlers and event subscriptions with the framework. This is where
   `*framework.Bus` and `*framework.Pipeline` first appear. It is completely
   separate from OS process management and from manifest parsing.

4. **Plugin supervision** — the `Manager` struct, `pluginInstance`, `ConnectAll`,
   `monitorPlugin`, and `Shutdown`. This is the orchestration layer: it scans
   the directory, invokes the launch and declare concerns, starts the monitor
   goroutine, and coordinates graceful shutdown with per-plugin timeouts.

A developer changing the manifest file format currently shares a file with a
developer changing the Unix socket setup. A developer adding a new framework
integration point (e.g., new subscription type in `readDeclare`) shares a file
with a developer fixing a process supervision race. These concerns have zero
shared state; they happen to be called in sequence but are otherwise independent.

**Target files:**

```
internal/plugin/
  manifest.go      # loadManifest, buildEnv
  launch.go        # launchPlugin, acceptWithTimeout, errNoManifest
  declare.go       # readDeclare
  manager.go       # Manager, pluginInstance, NewManager, ConnectAll,
                   # monitorPlugin, Shutdown
```

**Expected sizes:**
- `manifest.go` — ~35 lines
- `launch.go`   — ~65 lines
- `declare.go`  — ~65 lines
- `manager.go`  — ~185 lines

**Domain rule:** `manifest.go` imports only `encoding/json`, `os`, `path/filepath`,
and `gopkg.in/yaml.v3`. `launch.go` imports only `os`, `os/exec`, `net`, `path/filepath`,
`time`, `fmt`, and `errors`. Neither imports `internal/framework`. `declare.go`
imports `internal/framework` and `encoding/json`. `manager.go` imports
`internal/framework`, `launch.go`'s symbols, and `declare.go`'s symbol.
The import graph stays strictly layered.

---

### 3.2 `internal/plugin/codec.go` — 269 lines

**What is entangled here:**

`codec.go` serves three distinct concerns:

1. **Wire message types** — the eight JSON-RPC struct types used for serialisation:
   `rpcRequest`, `rpcNotification`, `rpcResponse`, `rpcError`, `pipelineParams`,
   `pipelineResult`, `eventParams`, `emitEventParams`. These are pure data
   definitions. They import only `encoding/json`. Every other file in the package
   imports these types, so they belong in isolation.

2. **Connection I/O** — `pluginConn`, `newPluginConn`, `start`, `readNextLine`,
   `writeMessage`, `sendRequest`, `sendNotification`, and `Close`. These are the
   raw transport layer: reading newline-delimited JSON from a `net.Conn`,
   writing it back, and managing the in-flight pending request map and ID counter.
   The concern is "move bytes on the wire reliably"; it has no knowledge of what
   those bytes mean in terms of pipeline points or event names.

3. **Application protocol** — `readLoop`, `handleEmitEvent`,
   `sendHandlePipelinePoint`, `sendHandleEvent`, and `sendShutdown`. This is the
   interpretation layer: what to do when a specific method arrives or when we
   need to invoke a specific operation on a plugin. This layer depends on
   `*framework.Bus` (to re-emit plugin events) and understands protocol-level
   method names like `"handle_pipeline_point"`, `"emit_event"`, and
   `"shutdown"`.

The transport concern (`writeMessage`, `sendRequest`) has no business being in
the same file as the application protocol concern (`handleEmitEvent` calling
`c.bus.Emit`).

**Target files:**

```
internal/plugin/
  messages.go   # rpcRequest, rpcNotification, rpcResponse, rpcError,
                # pipelineParams, pipelineResult, eventParams, emitEventParams
  conn.go       # pluginConn struct, newPluginConn, start, readNextLine,
                # writeMessage, sendRequest, sendNotification, Close
  protocol.go   # readLoop, handleEmitEvent, sendHandlePipelinePoint,
                # sendHandleEvent, sendShutdown
```

**Expected sizes:**
- `messages.go`  — ~50 lines
- `conn.go`      — ~110 lines
- `protocol.go`  — ~110 lines

**Domain rule:** `messages.go` imports only `encoding/json`. `conn.go` imports
`net`, `bufio`, `encoding/json`, `fmt`, `log`, `sync`, and `sync/atomic` — no
framework types. `protocol.go` imports `internal/framework` (for `bus.Emit`) and
`encoding/json`. The `pluginConn` struct lives in `conn.go`; its methods may
appear in any of the three files since they are all in `package plugin`.

---

### 3.3 `app.go` — 293 lines

**What is entangled here:**

`app.go` serves four distinct concerns:

1. **Framework object and construction** — the `App` struct definition,
   `New()`, and `mustRegisterBuiltinHandlers()`. This is the composition root of
   the framework: it allocates every subsystem and wires them together. Its job
   is to answer: "what does an App own and how is it initialised?"

2. **Pipeline and event registration data** — `declarePipelines()` and
   `declareEvents()`. These two functions contain a structured list of the 14
   standard vdb.* pipelines (each with an ordered point sequence) and the 12
   standard vdb.* event names. They are pure data declarations expressed as
   function calls. A developer adding a new pipeline point should only need to
   touch one file. A developer changing the App struct should never be in the
   same diff as a developer adjusting a pipeline sequence.

3. **Driver integration** — `UseDriver()` and `DriverAPI()`. These two methods
   are the bridge between `*App` and the composition root that connects a driver
   to the framework. They are small, but they belong conceptually with the other
   public configuration methods in `app_api.go`, not with struct allocation or
   pipeline data.

4. **Lifecycle execution** — `Run()` and `Stop()`. These implement the observable
   lifecycle of the running process: signal handling, the startup pipeline
   sequence, the idle select loop, and graceful shutdown. A developer debugging
   a shutdown race should not need to page through pipeline declaration lists.

The `lifecycleAppAdapter` struct (6 one-line methods) is an internal wiring shim
that gives `lifecycle.Handlers` access to private `*App` fields without exposing
those fields on the public `*App` surface. It is tightly coupled to the `App`
struct definition and belongs beside it.

**Target files:**

```
app.go         # App struct, lifecycleAppAdapter, New(), mustRegisterBuiltinHandlers()
pipelines.go   # declarePipelines(), declareEvents()
app_run.go     # Run(), Stop()
```

The three existing driver-integration methods (`UseDriver`, `DriverAPI`) move into
`app_api.go`, which already holds the other public configuration methods
(`Attach`, `Subscribe`, `DeclareEvent`, `DeclarePipeline`, `Emit`, `Process`).

**Expected sizes:**
- `app.go`       — ~115 lines
- `pipelines.go` — ~100 lines
- `app_run.go`   — ~60 lines
- `app_api.go`   — ~90 lines (was 62; grows by the two moved methods)

**Domain rule:** `pipelines.go` imports only `internal/points` and
`internal/framework`. `app_run.go` imports `internal/points`, `internal/framework`,
`log`, `os`, `os/signal`, and `syscall`. Neither imports `internal/delta`,
`internal/connection`, or any other domain package — those imports stay in `app.go`
where the subsystems are constructed.

---

### 3.4 `driver_api.go` — 282 lines

**What is wrong here:**

`driver_api.go` is not merely oversized — it is in the wrong place. The file
contains the framework's full DriverAPI dispatch logic: a struct
(`driverAPIImpl`) that holds `*App` and uses it to reach `app.pipe`,
`app.bus`, `app.conns`, and `app.schema`. The struct carries no state of its
own; it exists solely as a dispatch layer over four injected dependencies.

Every other domain concern in vdb-core follows the same pattern and lives in
its own `internal/` package:

- `connection.Handlers` takes `*connection.State`
- `transaction.Handlers` takes `*connection.State` and `*delta.Delta`
- `write.Handlers` takes `*schema.Cache` and `*delta.Delta`
- `emit.Handlers` takes no dependencies

`driverAPIImpl` takes exactly four things from `*App`: `pipe`, `bus`, `conns`,
and `schema`. These are named, explicit dependencies — not the whole App. There
is no reason this logic must live in the root package. It does so only because
it was written when `*App` was the only available handle.

Moving `driverAPIImpl` to `internal/driverapi` produces the same benefit it
produces for every other domain package: the logic becomes independently
testable without constructing a full `*App`, the boundary is compiler-enforced,
and the root package's `driver_api.go` is **deleted entirely** rather than
merely trimmed.

**Target package:** `internal/driverapi`

```
internal/driverapi/
  impl.go      # Impl struct, New(), all 14 DriverAPI method implementations
  results.go   # validateQueryResult, validateRecordsSourceResult,
               # validateRecordsMergedResult, validateWriteInsertResult,
               # validateWriteUpdateResult, extractRecord, extractRecordSlice
```

`Impl` is constructed with explicit dependencies, identical in spirit to every
other handler package constructor:

```go
package driverapi

type Impl struct {
    pipe   *framework.Pipeline
    bus    *framework.Bus
    conns  *connection.State
    schema *schema.Cache
}

func New(
    pipe   *framework.Pipeline,
    bus    *framework.Bus,
    conns  *connection.State,
    schema *schema.Cache,
) *Impl
```

The root package wires it in `New()` alongside all the other handler groups:

```go
app.apiImpl = driverapi.New(app.pipe, app.bus, app.conns, app.schema)
```

`driver_api.go` is deleted. No root-package file contains any DriverAPI logic.

**Expected sizes:**
- `internal/driverapi/impl.go`     — ~150 lines
- `internal/driverapi/results.go`  — ~80 lines

**Import DAG for `internal/driverapi`:**

```
internal/driverapi
  → internal/framework   (Pipeline, Bus)
  → internal/payloads    (all payload types)
  → internal/points      (pipeline and event name constants)
  → internal/connection  (State.GetDatabase)
  → internal/schema      (Cache.Load, Cache.Invalidate)
```

None of these packages import `internal/driverapi`. The root package imports
`internal/driverapi` and no longer owns any DriverAPI dispatch logic. The
invariant — no `internal/` package imports the root — continues to hold.

**Domain rule:** `results.go` imports only `fmt` and `internal/payloads`. It
has no dependency on `*framework.Pipeline`, `*connection.State`, `*schema.Cache`,
or the root package. It is a pure type-assertion and coercion library for
pipeline result payloads and is testable in complete isolation.

---

### 3.5 `internal/delta/delta.go` — 271 lines

**What is entangled here:**

`delta.go` serves four distinct concerns:

1. **Internal storage** — the `tableState` struct, `newTableState`, the `Delta`
   struct, `New()`, the unexported `tableFor()` accessor, and the `copyRecord`
   and `copyTableState` copy helpers. This is the data structure definition and
   memory management layer. Nothing above it in the call stack needs to know
   about `tableState` directly.

2. **Write path** — `ApplyInsert`, `ApplyUpdate`, and `ApplyDelete`. These are
   the mutation operations. They contain the delta's core invariants: how a
   net-new insert is distinguished from a source-row update, how `currentToStable`
   tracks key renames across successive updates, and how a delete of a net-new
   insert is handled differently from a delete of a source row. This logic
   deserves its own file because it is the hardest part of the delta to reason
   about and will be the most frequently modified as correctness edge cases are
   discovered.

3. **Snapshot and restore** — the `Snapshot` struct, `Delta.Snapshot()`, and
   `Delta.Restore()`. This is the transaction checkpoint mechanism: capturing the
   full delta state at BEGIN TRANSACTION time and restoring it on ROLLBACK. It is
   a distinct lifecycle concern from day-to-day mutations and from the read path.

4. **Read path and keying** — `Records()`, `DeltaTableState`, `TableState()`,
   and `RecordKey()`. These are the query-side operations: materialising the
   delta's current state for the overlay engine, exposing structured per-table
   views, and producing the canonical string key used to identify records across
   all four concerns. `RecordKey` in particular is referenced by every other
   concern (mutations, snapshot diffing, read assembly) — it is a foundational
   primitive that deserves to be findable at a glance without scrolling past
   mutation logic.

**Target files:**

```
internal/delta/
  delta.go      # tableState, newTableState, Delta, New(), tableFor(),
                # copyRecord, copyTableState
  mutations.go  # ApplyInsert, ApplyUpdate, ApplyDelete
  snapshot.go   # Snapshot struct, Delta.Snapshot(), Delta.Restore()
  query.go      # Records(), DeltaTableState, Delta.TableState(), RecordKey()
```

**Expected sizes:**
- `delta.go`     — ~70 lines
- `mutations.go` — ~65 lines
- `snapshot.go`  — ~50 lines
- `query.go`     — ~75 lines

**Domain rule:** All four files are `package delta`. No new imports are introduced.
`RecordKey` stays in `query.go` (read path / keying) because its primary role is
in key derivation for read and overlay operations. The mutation methods call it
via the same package — no import needed.

---

## 4. Target File Tree (changed files only)

```
vdb-core/
│
├── app.go                    # App struct, lifecycleAppAdapter, New(),
│                             # mustRegisterBuiltinHandlers()
├── app_api.go                # UseDriver, DriverAPI, Attach, Subscribe,
│                             # DeclareEvent, DeclarePipeline, Emit, Process
├── app_run.go                # Run(), Stop()          ← NEW
├── pipelines.go              # declarePipelines(), declareEvents()  ← NEW
│
└── internal/
    ├── driverapi/
    │   ├── impl.go       # Impl struct, New(), all 14 DriverAPI method impls ← NEW
    │   └── results.go    # validate*Result, extractRecord,
    │                     # extractRecordSlice                                ← NEW
    │
    ├── delta/
    │   ├── delta.go          # storage layer, copy helpers
    │   ├── mutations.go      # ApplyInsert, ApplyUpdate, ApplyDelete  ← NEW
    │   ├── snapshot.go       # Snapshot, Snapshot(), Restore()        ← NEW
    │   └── query.go          # Records(), DeltaTableState, TableState(),
    │                         # RecordKey()                            ← NEW
    │
    └── plugin/
        ├── manager.go        # Manager, pluginInstance, ConnectAll,
        │                     # monitorPlugin, Shutdown
        ├── manifest.go       # loadManifest, buildEnv                ← NEW
        ├── launch.go         # launchPlugin, acceptWithTimeout,
        │                     # errNoManifest                         ← NEW
        ├── declare.go        # readDeclare                           ← NEW
        ├── messages.go       # all rpc* and *Params structs          ← NEW
        ├── conn.go           # pluginConn, transport methods         ← (from codec.go)
        ├── protocol.go       # readLoop, handleEmitEvent, send*      ← (from codec.go)
        └── contract.go       # Manifest, DeclareParams (unchanged)
```

`driver_api.go` is deleted from the root package. `app.go` gains one import
(`internal/driverapi`) and one wiring line in `New()`. All other files not
listed above are unchanged.

---

## 5. What Does Not Change

- **No public API changes.** Every exported symbol retains its current name and
  signature. Splitting a file never changes what the package exports.

- **No import DAG changes.** The existing dependency rules from the structural
  refactor plan remain in force. Moving a function from one file to another
  within the same package never changes the package's import graph.

- **No test changes required.** Tests that currently pass against the existing
  files will continue to pass unchanged, because Go tests are compiled per-package
  and do not depend on which file within a package contains a given symbol.

- **No interface changes.** `Delta`, `Manager`, `App`, `DriverAPI` — all
  interfaces and method sets are identical before and after.

---

## 6. Migration Steps

Execute one step at a time. Run `go test ./... -race` after each step. Never
let the build go red between commits.

| Step | Action |
|------|--------|
| 1 | Create `internal/delta/mutations.go` with `ApplyInsert`, `ApplyUpdate`, `ApplyDelete`. Remove those methods from `delta.go`. Build green. |
| 2 | Create `internal/delta/snapshot.go` with `Snapshot` struct, `Snapshot()`, `Restore()`. Remove from `delta.go`. Build green. |
| 3 | Create `internal/delta/query.go` with `Records()`, `DeltaTableState`, `TableState()`, `RecordKey()`. Remove from `delta.go`. Build green. |
| 4 | Create `internal/plugin/messages.go` with all eight `rpc*` / `*Params` structs. Remove from `codec.go`. Build green. |
| 5 | Create `internal/plugin/conn.go` with `pluginConn` struct, `newPluginConn`, `start`, `readNextLine`, `writeMessage`, `sendRequest`, `sendNotification`, `Close`. Remove from `codec.go`. Delete `codec.go`. Build green. |
| 6 | Create `internal/plugin/protocol.go` with `readLoop`, `handleEmitEvent`, `sendHandlePipelinePoint`, `sendHandleEvent`, `sendShutdown`. Remove from `manager.go` (they were on `pluginConn` via `codec.go`). Build green. |
| 7 | Create `internal/plugin/manifest.go` with `loadManifest`, `buildEnv`. Remove from `manager.go`. Build green. |
| 8 | Create `internal/plugin/launch.go` with `launchPlugin`, `acceptWithTimeout`, `errNoManifest`. Remove from `manager.go`. Build green. |
| 9 | Create `internal/plugin/declare.go` with `readDeclare`. Remove from `manager.go`. Build green. |
| 10 | Create `internal/driverapi/results.go` with `validateQueryResult`, `validateRecordsSourceResult`, `validateRecordsMergedResult`, `validateWriteInsertResult`, `validateWriteUpdateResult`, `extractRecord`, `extractRecordSlice`. Build green. |
| 11 | Create `internal/driverapi/impl.go` with `Impl` struct, `New()`, and all 14 method implementations. Wire `app.apiImpl = driverapi.New(app.pipe, app.bus, app.conns, app.schema)` in `app.go`. Delete `driver_api.go`. Update `driver_api_test.go` to import `internal/driverapi` directly. Build green. All tests pass. |
| 12 | Create `pipelines.go` with `declarePipelines()` and `declareEvents()`. Remove from `app.go`. Build green. |
| 13 | Create `app_run.go` with `Run()` and `Stop()`. Remove from `app.go`. Build green. |
| 14 | Move `UseDriver()` and `DriverAPI()` from `app.go` into `app_api.go`. Build green. All tests pass. |
| 15 | Audit all file line counts. Verify every production `.go` file is ≤ 200 lines. |
| 16 | Write unit tests for `internal/driverapi` using stub `*framework.Pipeline`, `*framework.Bus`, `*connection.State`, and `*schema.Cache`. Verify that each method invokes the correct pipeline or event name. |

---

## 7. Invariants After Refactor

| # | Invariant | Verify with |
|---|-----------|-------------|
| 1 | No production `.go` file exceeds 200 lines | `find . -name "*.go" ! -name "*_test.go" \| xargs wc -l \| awk '$1>200{print}'` returns empty |
| 2 | No public API surface changes | `go doc ./...` output is identical before and after |
| 3 | Import DAG is unchanged | `grep -r 'import' internal/ --include="*.go"` shows no new cross-package dependencies |
| 4 | All tests pass | `go test ./... -race` exits 0 |
| 5 | `internal/delta` has exactly four `.go` files | `ls internal/delta/*.go \| wc -l` returns 4 |
| 6 | `internal/plugin` has exactly seven `.go` files | `ls internal/plugin/*.go \| wc -l` returns 7 |
| 7 | `codec.go` does not exist | `ls internal/plugin/codec.go` returns not found |
| 8 | `driver_api.go` does not exist in the root package | `ls driver_api.go` returns not found |
| 9 | `internal/driverapi` imports no root-package symbol | `grep -r '"github.com/AnqorDX/vdb-core"' internal/driverapi/` returns nothing |
| 10 | `internal/driverapi/results.go` imports only `fmt` and `internal/payloads` | Inspect import block |
| 11 | `pipelines.go` imports only `internal/points` and `internal/framework` | Inspect import block |
| 12 | `app_run.go` does not import any domain package (`delta`, `connection`, etc.) | Inspect import block |