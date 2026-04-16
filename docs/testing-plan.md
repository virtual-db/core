# vdb-core — Testing Plan

## Status: Phase 1 COMPLETE | Phase 2 DRAFT — pending implementation

---

## Overview

Phase 1 (complete): All 9 testable internal packages have test files. All tests
pass under `-race`. One production bug was found and fixed in
`internal/delta/query.go` (`TableState` was returning shallow copies of inner
records, allowing callers to corrupt delta state).

Phase 2 (this document): Two structural problems remain.

**Problem 1 — Wrong perspective.** The Phase 1 tests were written from the
implementer's perspective — same package, access to unexported symbols,
verifying internal wiring. Tests should be written from the *client's*
perspective: the package is a black box; only its public API is visible; the
test only cares that behaviour is correct, not how it is achieved internally.
This matters because implementation-perspective tests break on refactors that
preserve behaviour, and they give false confidence by testing details that no
caller depends on.

**Problem 2 — Wrong file structure.** All test files sit alongside source files
in the same directory. Test files should live in `tests/` subdirectories, one
per package. This keeps source directories clean and makes the package
boundary explicit.

**The 200-line marker is a smell indicator, not a target.** A test file that
exceeds 200 lines is a signal to ask whether it owns more than one concern —
not a directive to split at the 200-line mark. A 250-line file that tests one
cohesive concern is fine. A 150-line file that tests three unrelated things is
the real problem. Every file split in this plan is justified by a distinct
concern, not by line count.

---

## Core Conventions for Phase 2

### External test package

Every test file in a `tests/` directory declares `package X_test`. This
enforces the client perspective at the compiler level — unexported symbols are
invisible, so they cannot be tested by accident.

### Dot import of the package under test

To avoid cluttering every call with a package qualifier while remaining outside
the package:

```go
package framework_test

import . "github.com/AnqorDX/vdb-core/internal/framework"

// NewPipeline(...) instead of framework.NewPipeline(...)
// HandlerContext{} instead of framework.HandlerContext{}
```

Only the package under test receives the dot import. All other imports
(framework, payloads, points, delta, connection, schema) keep their qualifiers.
This makes it immediately clear which names belong to the package under test
and which come from elsewhere.

### No export_test.go

If a test requires exposing an unexported symbol to exercise it, the test is
wrong — not the symbol. The correct fix is to test the behaviour through the
public API instead. `internal/driverapi` previously had a proposed
`export_test.go` to expose the private validator functions; that approach is
dropped. The validator functions are an implementation detail. What matters is
that `Impl.QueryReceived` returns the correct rewritten query string; *how* it
validates the pipeline result is irrelevant to any caller.

### No stubRegistrar

The Phase 1 tests used a `stubRegistrar` to verify that `Handlers.Register`
attached functions to specific point-name strings. That is internal wiring
verification, not behaviour verification. A client of `connection.Handlers`
does not care which string keys were registered. It cares that after calling
`Register` on a pipeline and processing that pipeline, connections appear in
State.

The correct approach: construct a real `framework.Pipeline`, declare the
relevant pipeline on it, call `Register`, call `pipeline.Process(...)`, and
assert the observable side effect on the concrete dependency (state, delta,
schema cache, bus event). This tests the actual contract.

### Real Pipeline setup pattern

```go
// Example: testing connection.Handlers behaviour

var global framework.GlobalContext
pipe := framework.NewPipeline(&global)
bus  := framework.NewBus(&global)
global = framework.SealContext(framework.NewGlobalContextBuilder(), bus, pipe)

pipe.DeclarePipeline(points.PipelineConnectionOpened, []string{
    points.PointConnectionOpenedBuildContext,
    points.PointConnectionOpenedTrack,
    points.PointConnectionOpenedEmit,
})

state := connection.NewState()
h     := connection.New(state)
if err := h.Register(pipe); err != nil {
    t.Fatal(err)
}

pipe.Process(points.PipelineConnectionOpened,
    payloads.ConnectionOpenedPayload{ConnectionID: 1, User: "alice", Address: "127.0.0.1"})

conn, ok := state.Get(1)
// assert ok == true, conn.User == "alice", etc.
```

The test imports `points` and `payloads` for their constants and types — those
are part of the public framework contract, not internal details.

### go test invocation

Tests live in `tests/` subdirectories. Because `tests/` contains only
`*_test.go` files, `go build ./...` is unaffected. `go test ./...` visits them
automatically — no Makefile required:

```
go test -race -count=1 ./...
```

---

## Package-by-Package Plan

For each package: what the client cares about, what the test files are, and
why each file boundary exists.

---

### `internal/framework`

The framework has four distinct types that a caller uses independently.

**`tests/context_test.go` — `GlobalContextBuilder` and `GlobalContext`**

Concern: a builder accumulates key-value pairs before startup; sealing it
produces an immutable `GlobalContext` that returns those values; the builder
is inert after sealing.

All builder and sealed-context behaviour belongs here because they are two
halves of one data-lifecycle concern. There is no reason to split them further.

**`tests/pipeline_test.go` — `Pipeline`**

Concern: declaring pipelines, attaching handlers, processing with correct
priority order, aborting on error, and the zero-value safety guarantee.

`BuildContext` is tested here because it is a pipeline handler — it is not a
type of its own; it only makes sense in the context of pipeline processing.

**`tests/bus_test.go` — `Bus`**

Concern: declaring events, subscribing, emitting, fire-and-forget error
semantics, and concurrency safety.

Bus and Pipeline are entirely independent types with different delivery
semantics (bus is fire-and-forget; pipeline is synchronous with a result).
They belong in separate files.

**`tests/correlation_test.go` — `CorrelationID` and `NewID`**

Concern: ID generation and causal-chain construction. This is a self-contained
utility with no dependency on the other framework types.

---

### `internal/delta`

Delta's public API has three distinct behavioural areas: applying mutations,
querying state, and snapshotting. Each area has non-trivial logic that warrants
its own file.

**`tests/mutations_test.go` — `ApplyInsert`, `ApplyUpdate`, `ApplyDelete`**

Concern: how the delta state changes in response to write operations. This
includes the non-obvious cases: updating a net-new insert re-keys it rather
than creating an update entry; deleting an updated source row resolves through
the stable key; deleting a net-new insert removes it with no tombstone.

All three mutation methods are in one file because the cases interact — a test
for `ApplyUpdate` after `ApplyInsert` spans both methods. The concern is
"mutation semantics", not individual methods.

**`tests/query_test.go` — `RecordKey`, `Records`, `TableState`**

Concern: reading from the delta. These are query operations on accumulated
state. `RecordKey` belongs here because it is the key contract for how records
are identified — callers need to understand it to interpret `TableState`
results.

**`tests/snapshot_test.go` — `Snapshot` and `Restore`**

Concern: point-in-time capture and restoration. Snapshot/Restore is a
self-contained feature (used by the transaction system) with its own
correctness properties: a restore reverts all mutations made after the
snapshot, including across different tables; wrong connection ID is rejected;
nil snapshot is rejected.

*Note: `TestTableFor_CreatesOnFirstAccess` from Phase 1 is dropped. It
accessed `d.tables` directly — an internal field. The equivalent client-visible
behaviour is fully covered by `mutations_test.go`: after `ApplyInsert`, the
table's state is observable through `TableState`.*

---

### `internal/schema`

**`tests/cache_test.go` — `Cache`**

Concern: the cache correctly stores, retrieves, and invalidates schema entries,
with defensive copies at both `Load` and `Get` to prevent callers from
corrupting stored state.

One file. The cache is one type with one responsibility.

---

### `internal/connection`

**`tests/state_test.go` — `State`**

Concern: the per-connection store correctly tracks, retrieves, updates, and
removes `Conn` entries. `GetDatabase` is a derived read and belongs here.

**`tests/handlers_test.go` — `Handlers`**

Concern: when the connection lifecycle pipelines are processed, connection state
changes correctly. This is tested by constructing a real pipeline, calling
`Register`, processing payloads, and asserting `State` changes — not by
inspecting which point strings were registered.

Three behaviours, one concern (managing State in response to pipeline events):
- Processing `vdb.connection.opened` → connection appears in State
- Processing `vdb.connection.closed` → connection removed from State
- Processing `vdb.query.received` → `conn.Database` is updated

These three belong in one file because they are the same concern: the
connection handlers maintain State. Splitting by method would obscure the
unifying purpose.

---

### `internal/transaction`

**`tests/handlers_test.go` — `Handlers`**

Concern: the transaction lifecycle pipelines interact correctly with the delta
and connection state — specifically that the snapshot discipline (take at
begin, discard at commit, restore at rollback) is upheld.

Three behaviours, one concern (snapshot discipline):
- Processing `vdb.transaction.begin` → snapshot stored on connection
- Processing `vdb.transaction.commit` → snapshot cleared
- Processing `vdb.transaction.rollback` → delta restored to snapshot state

One file. The three are facets of the same invariant and should be read
together.

---

### `internal/write`

**`tests/overlay_test.go` — `Overlay`**

Concern: the pure overlay function correctly merges delta state onto a source
record slice. `Overlay` is an exported function with well-defined semantics:
inserts are appended, updates replace source records by key, tombstones exclude
source records, source is never mutated.

This is separate from the handler tests because `Overlay` is a pure function
that can be tested without any pipeline machinery. Its correctness is
independent of how the pipeline invokes it.

**`tests/handlers_test.go` — `Handlers`**

Concern: the write and records pipelines correctly mutate the delta and produce
merged record sets. Tested through real pipeline processing:
- Processing `vdb.write.insert` → record appears in delta inserts
- Processing `vdb.write.update` → delta update entry correct
- Processing `vdb.write.delete` → delta tombstone correct
- Processing `vdb.records.source` → overlay applied to source records

These are in one file because they share the same dependency pair (delta +
schema cache) and the same testing pattern. If write mutations and record
overlay ever grew substantially, they could be split — but the split must be
justified by diverging concerns, not growing line count.

---

### `internal/emit`

**`tests/handlers_test.go` — `Handlers`**

Concern: each pipeline emit point fires the correct bus event with the correct
payload type.

All nine emit handlers belong in one file because they are the same concern
repeated nine times: pipeline point → bus event. The test pattern is identical
for all nine. Splitting by domain (connection events vs transaction events)
would be arbitrary — the concern is the same regardless of domain.

Test pattern: construct a real bus + pipeline + sealed GlobalContext, subscribe
to the expected event on the bus, process the relevant pipeline, assert the
subscriber received the correct payload type.

---

### `internal/lifecycle`

The lifecycle handlers cover two distinct stages of the framework's own startup
and shutdown, which have no shared dependencies or test setup.

**`tests/context_test.go` — `ContextCreateBuild`, `ContextCreateSeal`**

Concern: the context-create pipeline constructs a builder and seals it into the
app's global context. The observable outcome is that `app.SetGlobal` is called
with a sealed `GlobalContext` containing the expected values.

**`tests/server_test.go` — `ServerStartLaunch`, `ServerStopDrain`, `ServerStopHalt`**

Concern: the server start and stop pipelines correctly invoke the server's
`Run` and `Stop` methods and the plugin manager's `Shutdown`, including the
nil-server no-op case.

The split is concern-based: context creation (framework initialisation) is a
different lifecycle stage from server management (external process management).
They share no state and have different failure modes.

A `stubApp` satisfying the `lifecycle.App` interface is needed here. This is
not a `stubRegistrar` — it is a legitimate test double for an *external
dependency* (the App is injected into lifecycle.Handlers from outside). The
stub records which lifecycle callbacks were invoked so tests can assert
observable outcomes.

---

### `internal/driverapi`

`Impl` has 14 public methods that divide naturally along feature domains. The
`results_test.go` file from Phase 1 (which tested the unexported validator
functions directly) is **deleted**. Those validators are an implementation
detail; their correctness is verified indirectly through the public `Impl`
methods.

All `Impl` tests share a `newTestImpl` constructor helper:

```go
func newTestImpl(t *testing.T) (*Impl, *Pipeline, *Bus, *connection.State, *schema.Cache) {
    t.Helper()
    var global GlobalContext
    pipe  := NewPipeline(&global)
    bus   := NewBus(&global)
    conns := connection.NewState()
    sch   := schema.NewCache()
    global = SealContext(NewGlobalContextBuilder(), bus, pipe)
    return New(pipe, bus, conns, sch), pipe, bus, conns, sch
}
```

The helper is defined in whichever `tests/` file it appears first
alphabetically; all files in the directory share the `package driverapi_test`
namespace and can use it.

**`tests/impl_connections_test.go`**

Concern: `ConnectionOpened` and `ConnectionClosed` correctly invoke the
connection lifecycle pipelines. `ConnectionOpened` propagates errors;
`ConnectionClosed` is fire-and-forget (errors are logged, not returned).

**`tests/impl_transactions_test.go`**

Concern: `TransactionBegun`, `TransactionCommitted`, and `TransactionRolledBack`
correctly invoke the transaction lifecycle pipelines. `TransactionRolledBack`
is fire-and-forget.

**`tests/impl_query_test.go`**

Concern: `QueryReceived` returns the query string produced by the pipeline (a
handler may rewrite it), propagates errors, and correctly returns the original
query when no handler is attached. `QueryCompleted` emits the correct event on
the bus, including the database name read from `conns`.

These two belong together because they are the two halves of query lifecycle
from the driver's perspective.

**`tests/impl_records_test.go`**

Concern: `RecordsSource` returns the records produced by the pipeline;
`RecordsMerged` returns the original input records on pipeline error (not an
error to the caller).

**`tests/impl_write_test.go`**

Concern: `RecordInserted`, `RecordUpdated`, and `RecordDeleted` correctly
invoke the write pipelines and propagate errors.

**`tests/impl_schema_test.go`**

Concern: `SchemaLoaded` populates the schema cache *before* emitting the
event (so subscribers see a populated cache); `SchemaInvalidated` clears the
cache *before* emitting (so subscribers see an empty cache). The ordering is
the observable contract.

---

### Root `tests/`

**`tests/run_test.go`** — stays. Testing `App.Run()`, `App.Stop()`, signal
handling, and lifecycle pipeline ordering genuinely requires a running `*App`.
This is Tier 3 integration: no internal package can observe these properties.

Evaluate `run_test.go` for a concern-based split once it moves. At 369 lines
it may own more than one concern (e.g., startup sequencing vs. shutdown
sequencing vs. signal handling). Split only if distinct concerns are identified.

**`tests/interfaces_test.go`** — stays. Compile-time interface satisfaction
checks for the public API belong at the package that defines the interfaces.

---

## Target Directory Structure

```
vdb-core/
├── tests/
│   ├── interfaces_test.go
│   └── run_test.go
└── internal/
    ├── framework/
    │   └── tests/
    │       ├── bus_test.go
    │       ├── context_test.go
    │       ├── correlation_test.go
    │       └── pipeline_test.go
    ├── delta/
    │   └── tests/
    │       ├── mutations_test.go
    │       ├── query_test.go
    │       └── snapshot_test.go
    ├── schema/
    │   └── tests/
    │       └── cache_test.go
    ├── connection/
    │   └── tests/
    │       ├── handlers_test.go
    │       └── state_test.go
    ├── transaction/
    │   └── tests/
    │       └── handlers_test.go
    ├── write/
    │   └── tests/
    │       ├── handlers_test.go
    │       └── overlay_test.go
    ├── emit/
    │   └── tests/
    │       └── handlers_test.go
    ├── lifecycle/
    │   └── tests/
    │       ├── context_test.go
    │       └── server_test.go
    └── driverapi/
        └── tests/
            ├── impl_connections_test.go
            ├── impl_query_test.go
            ├── impl_records_test.go
            ├── impl_schema_test.go
            ├── impl_transactions_test.go
            └── impl_write_test.go
```

Files deleted (replaced by `tests/` equivalents):
- All `*_test.go` files currently in source directories
- `internal/driverapi/results_test.go` — deleted, not migrated (tests internals)
- Root `run_test.go` and `interfaces_test.go` — moved to `tests/`

No `export_test.go` files exist anywhere in the module.

---

## Migration Steps

| Step | Action | Verify |
|------|--------|--------|
| 1 | Create `internal/framework/tests/` with 4 files; use real Pipeline/Bus; dot-import framework | `go test ./internal/framework/tests/` green |
| 2 | Create `internal/delta/tests/` with 3 files; drop `TestTableFor_CreatesOnFirstAccess` | `go test ./internal/delta/tests/` green |
| 3 | Create `internal/schema/tests/cache_test.go` | `go test ./internal/schema/tests/` green |
| 4 | Create `internal/connection/tests/` with 2 files; use real Pipeline | `go test ./internal/connection/tests/` green |
| 5 | Create `internal/transaction/tests/handlers_test.go`; use real Pipeline | `go test ./internal/transaction/tests/` green |
| 6 | Create `internal/write/tests/` with 2 files; use real Pipeline | `go test ./internal/write/tests/` green |
| 7 | Create `internal/emit/tests/handlers_test.go`; use real Pipeline + Bus | `go test ./internal/emit/tests/` green |
| 8 | Create `internal/lifecycle/tests/` with 2 files; use stubApp | `go test ./internal/lifecycle/tests/` green |
| 9 | Create `internal/driverapi/tests/` with 6 impl files; delete results files | `go test ./internal/driverapi/tests/` green |
| 10 | Move root tests to `tests/`; evaluate `run_test.go` for concern split | `go test ./tests/` green |
| 11 | Delete all old `*_test.go` files from source directories | `find . -name "*_test.go" ! -path "*/tests/*"` empty |
| 12 | Run full suite | `go test -race -count=1 ./...` all green, zero data races |

---

## Invariants After Migration

| # | Invariant | Verify with |
|---|-----------|-------------|
| 1 | No test file in any source directory | `find . -name "*_test.go" ! -path "*/tests/*"` returns empty |
| 2 | All test files declare `package X_test` | `grep -r "^package" --include="*.go" internal/*/tests/` all end in `_test` |
| 3 | No `export_test.go` exists anywhere | `find . -name "export_test.go"` returns empty |
| 4 | No test file uses a `stubRegistrar` | `grep -r "stubRegistrar" --include="*_test.go"` returns empty |
| 5 | All tests pass with race detector | `go test -race -count=1 ./...` exits 0 |
| 6 | Every test file over 200 lines is reviewed for multiple concerns | Manual — justify any file over 200 lines with a written concern statement |
