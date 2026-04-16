# vdb-core — Structural Refactor Plan

## Status: DRAFT — pending implementation

---

## 1. Problem Statement

The module has three compounding problems that feed each other.

**Monolithic files.** `core.go` (695 lines) owns six unrelated concerns: context
types, the Bus abstraction, the Pipeline abstraction, public interface contracts,
the `App` struct and its full lifecycle, and the developer extension API. `handlers.go`
(539 lines) contains nine conceptually distinct handler groups with no separation
between them.

**Anonymous functions as the unit of logic.** Every handler in `handlers.go` is an
anonymous closure that captures `*App` directly. This means handler logic is
untestable without constructing a full `App`, the dependencies of each handler are
invisible (they are implicit in the closure, not declared in a type), and there is no
named artifact to navigate to, document, or mock.

**Flat package structure.** Everything is in `package core`. There is no mechanism
to enforce domain boundaries at compile time. Any file can reach any unexported
symbol. The result is accidental coupling — `handlers.go` knows about `app.delta`,
`app.conns`, `app.schema`, `app.bus`, `app.pipe`, `app.global`, `app.server`,
`app.plugins`, and `app.serverErrCh`. Changing any one of those fields requires
reading the entirety of `handlers.go`.

**Incorrectly public sub-packages.** `types`, `payloads`, `points`, and `plugin`
are currently public sub-packages but none of them are part of the external API.
`types.Record` is a type alias for `map[string]any` with no independent value.
`payloads` contains Go structs that only the framework's built-in internal handlers
ever type-assert. `points` contains Go constants that only the framework uses
internally to register handlers — the stable external API is the string name
itself, which works across any language or integration. `plugin` is documented in
the module's own package comment as an internal implementation detail. Exposing
all four implies to consumers that they should import them, which is wrong.

---

## 2. The Public API Boundary

The framework is explicitly designed so that the pipeline and event names — plain
strings such as `"vdb.connection.opened"` and `"vdb.query.received"` — are the
stable, language-agnostic extension points. `Pipeline.Process` and `Bus.Emit` both
accept a string name for exactly this reason: a Go plugin, a Python plugin, and a
Ruby plugin all attach by name. No compiled Go type is required.

This means:

- **`points`** is `internal/`. The string constants are used only by the framework's
  own built-in handler registrations. External consumers use the string names directly
  and do not need a Go constant to do so.

- **`payloads`** is `internal/`. The concrete payload structs are only type-asserted
  inside the framework's built-in handlers, which are all `internal/` under this
  refactor. An external Go handler receives `any` and works with the data at whatever
  level of abstraction it chooses. It does not need a framework-supplied Go struct.

- **`types`** is eliminated. `types.Record` is `= map[string]any`. The alias adds
  no semantic value and creates a dependency solely to give `DriverAPI` method
  signatures a name. The `DriverAPI` interface will use `map[string]any` directly.
  `vdb-mysql-driver` already uses `map[string]any` natively for exactly this reason.

- **`plugin`** is `internal/`. The module's own package doc already states this.
  Plugin authors communicate with the framework via the JSON-RPC protocol, not by
  importing the `plugin` package.

- **`delta`** is `internal/`. The `Delta` type is the framework's single,
  unconditional mutation store. It is always constructed in `New()` and is never
  nil after construction. There is no swappable backend interface; there is no
  external extension point for replacing it. A plugin that wants to observe or
  replicate mutation events subscribes to the standard `vdb.record.*` and
  `vdb.transaction.*` events via `core.Subscribe` — the same mechanism any other
  plugin uses. The `delta` package never appears in a plugin's import graph.

---

## 3. Design Goals

1. **Domain packages over domain files.** Each domain gets its own package under
   `internal/`. The Go compiler enforces the boundary. Nothing leaks by accident.

2. **Named structs, not anonymous closures.** Handler logic lives in named methods
   on named structs. Dependencies are declared in the struct, visible to the reader
   and to the test. Anonymous functions are permitted only for trivial one-liners
   (e.g. wrapping a single method call with no branching).

3. **Interface at the point of consumption.** Each internal package defines the
   interface it needs from the outside world. It does not import `*App`. It does not
   import the root package. Concrete types flow in through constructors.

4. **Root package is a thin public facade.** `vdb-core` (root) re-exports types from
   `internal/framework`, wires domain packages together in `New()`, and exposes
   the developer API. Nothing else.

5. **Every domain is independently testable.** A test in `internal/connection` needs
   only a `*connection.State` and a stub `Registrar`. It does not need `App`, it does
   not need `Run()`, it does not need a pipeline registry.

6. **No file exceeds 200 lines.** This is a forcing function. A file near the limit
   is a signal to look for an unidentified sub-domain.

---

## 4. Import DAG

The rule: edges point downward only. No package imports anything above it.

```
┌────────────────────────────────────────────────────────┐
│  vdb-core (root)                                       │
│  package core                                          │
│                                                        │
│  app.go · app_api.go · driver_api.go · interfaces.go  │
└──────┬─────────────────────────────────────────────────┘
       │ imports
       ▼
┌────────────────────────────────────────────────────────┐
│  internal/lifecycle    internal/connection             │
│  internal/transaction  internal/write                  │
│  internal/emit                                         │
└──────┬─────────────────────────────────────────────────┘
       │ all import
       ▼
┌────────────────────────────────────────────────────────┐
│  internal/framework                                    │
│  (HandlerContext, GlobalContext, Bus, Pipeline,        │
│   CorrelationID, Registrar)                            │
└──────┬──────────────────────────┬──────────────────────┘
       │                          │
       ▼                          ▼
┌─────────────────────┐  ┌──────────────────────────────┐
│  internal/payloads  │  │  github.com/AnqorDX/pipeline │
│  internal/points    │  │  github.com/AnqorDX/dispatch │
│  internal/connection│  │  (imported ONLY by framework)│
│    /state           │  └──────────────────────────────┘
│  internal/schema    │
│  internal/plugin    │
│  internal/delta     │
└─────────────────────┘
```

`internal/framework` is the only package that imports `pipeline` and `dispatch`.
All other packages work through the `Bus`, `Pipeline`, and `Registrar` types that
`internal/framework` defines.

---

## 5. Target File Tree

```
vdb-core/
│
│   ── Public API (package core) ──────────────────────────────────────────
│
├── app.go              # App struct, New(), Run(), Stop(), UseDriver(), DriverAPI()
├── app_api.go          # Attach(), Subscribe(), DeclareEvent(), DeclarePipeline(),
│                       #   Emit(), Process()
├── driver_api.go       # driverAPIImpl + result validators
├── interfaces.go       # Config, Server, DriverAPI, PointFunc, EventFunc
│                       #   (public contracts; zero implementation;
│                       #    map[string]any used directly — no types.Record)
│
│   ── Internal ────────────────────────────────────────────────────────────
│
├── internal/
│   │
│   ├── framework/          # Shared framework types — imported by all domains
│   │   ├── context.go      # HandlerContext, GlobalContext,
│   │   │                   #   GlobalContextBuilder, sealContext
│   │   ├── bus.go          # Bus, EventBus interface, noopBus, EventFunc
│   │   ├── pipeline.go     # Pipeline, PointFunc, Registrar interface,
│   │   │                   #   BuildContext handler func
│   │   └── correlation.go  # CorrelationID, newCorrelationID, newID
│   │
│   ├── points/             # Pipeline point and event name constants
│   │   └── names.go        # (moved from public points/; delta.provide.*
│   │                       #  constants removed — that pipeline is eliminated)
│   │
│   ├── payloads/           # Concrete payload structs for built-in pipelines
│   │   ├── connection.go
│   │   ├── context.go
│   │   ├── query.go
│   │   ├── rows.go
│   │   ├── schema.go
│   │   ├── server.go
│   │   ├── transaction.go
│   │   └── write.go
│   │                       # Note: payloads/delta.go is not carried forward.
│   │                       # DeltaProvidePayload had no purpose once the
│   │                       # delta.provide pipeline was eliminated.
│   │
│   ├── plugin/             # Plugin subprocess manager (moved from public plugin/)
│   │   ├── codec.go
│   │   ├── contract.go
│   │   └── manager.go
│   │
│   ├── delta/              # Mutation store — always present, never swappable
│   │   └── delta.go        # Delta struct, Snapshot struct, DeltaTableState,
│   │                       #   New(), RecordKey(), and all methods
│   │
│   ├── connection/         # Connection tracking domain
│   │   ├── state.go        # State (connMap) + conn (connState)
│   │   ├── handlers.go     # Handlers struct + Register + named methods
│   │   ├── state_test.go
│   │   └── handlers_test.go
│   │
│   ├── schema/             # Schema cache domain
│   │   ├── cache.go        # Cache struct (schemaCache + schemaEntry unified)
│   │   └── cache_test.go
│   │
│   ├── lifecycle/          # Lifecycle pipeline handlers
│   │   ├── app.go          # App interface (what lifecycle needs from core.App)
│   │   ├── handlers.go     # Handlers struct + Register + named methods
│   │   └── handlers_test.go
│   │
│   ├── transaction/        # Transaction domain
│   │   ├── handlers.go     # Handlers struct + Register + named methods
│   │   └── handlers_test.go
│   │
│   ├── write/              # Write domain (insert/update/delete + delta overlay)
│   │   ├── handlers.go     # Handlers struct + Register + named methods
│   │   ├── overlay.go      # Overlay func (extracted, named, testable)
│   │   ├── handlers_test.go
│   │   └── overlay_test.go
│   │
│   └── emit/               # Emit handlers (pure bus-firing)
│       ├── handlers.go     # Handlers struct + Register + named methods
│       └── handlers_test.go
│
│   ── Tests ────────────────────────────────────────────────────────────────
│
├── app_test.go             # New(), UseDriver(), double-Run guard
├── app_api_test.go         # Attach(), Subscribe() — integration, needs App
├── driver_api_test.go      # (renamed from core_handlers_test.go)
├── interfaces_test.go      # Compile-time interface checks (unchanged)
├── run_test.go             # Run() / Stop() lifecycle (unchanged)
└── testhelpers_test.go     # newTestApp(), waitEvent() — shared helpers
```

---

## 6. What the Public API Actually Is

After this refactor, `vdb-core` consumers import exactly one package:
`github.com/AnqorDX/vdb-core`. The public surface is:

**Construction and lifecycle**
- `New(Config) *App`
- `(*App) UseDriver(Server) *App`
- `(*App) DriverAPI() DriverAPI`
- `(*App) Run() error`
- `(*App) Stop() error`

**Extension API** (called before `Run`)
- `(*App) Attach(point string, priority int, fn PointFunc) *App`
- `(*App) Subscribe(event string, fn EventFunc) *App`
- `(*App) DeclareEvent(event string)`
- `(*App) DeclarePipeline(name string, points []string)`
- `(*App) Emit(event string, payload any)`
- `(*App) Process(pipeline string, payload any) (any, error)`

These six methods are the exact capabilities exposed to plugins over JSON-RPC via
the `declare` notification (`PipelineHandlers`, `EventSubscriptions`,
`EventDeclarations`, `PipelineDeclarations`) and the trigger channels. The public
Go API is intentionally isomorphic to the plugin protocol — nothing more, nothing
less. The symmetry is explicit:

| Concern | Declare | Handle | Trigger |
|---------|---------|--------|---------|
| Events | `DeclareEvent` | `Subscribe` | `Emit` |
| Pipelines | `DeclarePipeline` | `Attach` | `Process` |

`RegisterPlugin`, `SubscribePlugin`, and `DeclarePipelinePlugin` are **not** public
methods. They were internal bridge adapters consumed only by `plugin.Manager`. Under
this refactor, `Manager.ConnectAll` accepts `*framework.Bus` and `*framework.Pipeline`
directly and wires plugin handlers itself, removing the round-trip through `App`.
The `pluginHost` interface in `internal/plugin/manager.go` is deleted.

**Types** (in `interfaces.go`)
- `Config`
- `Server`
- `DriverAPI`
- `PointFunc`
- `EventFunc`

Nothing else. No `points`, no `payloads`, no `types`, no `plugin`, no `delta`, no
`HandlerContext`, no `GlobalContext`, no `Bus`, no `Pipeline`, no `DriverReceiver`.
These types are internal until a concrete external consumer requires them. A consumer
who wants to attach a handler at `vdb.connection.opened.accept` writes:

```go
app.Attach("vdb.connection.opened.accept", 10, func(ctx any, p any) (any, any, error) {
    // p is map[string]any or a concrete type — the handler decides
    // how to interpret it. No payloads import required.
    return ctx, p, nil
})
```

The pipeline string is the API. The Go constant in `internal/points` is an
implementation aid used by the framework itself.

---

## 7. `DriverAPI` Without `types.Record`

`types.Record` is `= map[string]any`. The alias provides no semantic information
beyond the underlying type. Removing it makes `DriverAPI` self-contained in
`interfaces.go` with no sub-package dependency:

```go
// interfaces.go
package core

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

`vdb-mysql-driver` already uses `map[string]any` throughout its callbacks. This
change requires zero modification there. The `types/` directory is deleted.

---

## 8. Package Definitions

---

### 8.1 `internal/framework`

**Purpose:** Owns every type that crosses an internal package boundary.
No handler logic, no registration, no application state.

**`context.go`**

```go
package framework

type CorrelationID struct {
    Root   string
    Parent string
    ID     string
}

type HandlerContext struct {
    Global        GlobalContext
    CorrelationID CorrelationID
}

type GlobalContext struct {
    values map[string]any
    bus    *Bus
    pipe   *Pipeline
}

func (g GlobalContext) Get(key string) any
func (g GlobalContext) Bus() EventBus
func (g GlobalContext) Pipeline() Pipeline

type GlobalContextBuilder struct { ... }
func NewGlobalContextBuilder() *GlobalContextBuilder
func (b *GlobalContextBuilder) Set(key string, val any)
func (b *GlobalContextBuilder) Get(key string) any

func SealContext(b *GlobalContextBuilder, bus *Bus, pipe *Pipeline) GlobalContext
```

**`bus.go`**

```go
package framework

type EventFunc func(ctx any, payload any) error

type EventBus interface {
    Emit(name string, payload any, ctx ...HandlerContext)
}

type Bus struct { ... }           // wraps *dispatch.EventBus
func NewBus(g *GlobalContext) *Bus
func (b *Bus) DeclareEvent(name string)
func (b *Bus) Subscribe(name string, fn EventFunc) error
func (b *Bus) Emit(name string, payload any, ctx ...HandlerContext)
```

**`pipeline.go`**

```go
package framework

type PointFunc = pipeline.HandlerFunc

// Registrar is the interface each domain Handlers struct accepts for
// registration. *Pipeline satisfies it. Tests can satisfy it with a stub.
type Registrar interface {
    Register(point string, priority int, fn PointFunc) error
}

type Pipeline struct { ... }      // wraps *pipeline.Registry
func NewPipeline(g *GlobalContext) *Pipeline
func (p *Pipeline) DeclarePipeline(name string, points []string)
func (p *Pipeline) Register(point string, priority int, fn PointFunc) error
func (p *Pipeline) MustRegister(point string, priority int, fn PointFunc)
func (p  Pipeline) Process(name string, payload any, ctx ...HandlerContext) (any, error)

// BuildContext is a ready-made PointFunc that stamps a fresh CorrelationID
// onto HandlerContext. Register it at every *.build_context point.
func BuildContext(ctx any, p any) (any, any, error)
```

**`correlation.go`**

```go
package framework

func NewCorrelationID(parent CorrelationID) CorrelationID
func NewID() string
```

**Key properties:**
- Imports `github.com/AnqorDX/pipeline` and `github.com/AnqorDX/dispatch`.
  No other package in this module may import those libraries.
- Zero application logic. Zero handler registration. Zero state.

---

### 8.2 `internal/points`

**Purpose:** String constants for all pipeline point names and event names. Used
only by the framework's internal packages. Not exported to consumers.

```go
package points

const (
    PipelineContextCreate = "vdb.context.create"
    PointContextCreateBuildContext = "vdb.context.create.build_context"
    // ... etc.

    EventConnectionOpened = "vdb.connection.opened"
    // ... etc.
)
```

No logic. No imports beyond `package points`. Moved verbatim from the current
public `points/` directory, with all `vdb.delta.provide.*` constants removed —
that pipeline is eliminated.

---

### 8.3 `internal/payloads`

**Purpose:** Concrete payload structs for all built-in pipelines. Used only by the
framework's internal handler packages for type-asserting pipeline payloads. Not
exported to consumers — a handler author receives `any` and works with the data
at whatever level of abstraction they choose.

Files moved from the current public `payloads/` directory. The package path changes
from `vdb-core/payloads` to `vdb-core/internal/payloads`. `payloads/delta.go`
(`DeltaProvidePayload`) is not carried forward — the `vdb.delta.provide` pipeline
no longer exists.

---

### 8.4 `internal/plugin`

**Purpose:** Plugin subprocess manager. Already documented as an internal
implementation detail. Plugin authors interact via JSON-RPC protocol; they do not
import this package. Moved verbatim from the current public `plugin/` directory.

---

### 8.5 `internal/delta`

**Purpose:** The framework's single, unconditional mutation store. `Delta` is
always constructed in `core.New()` and is always non-nil for the lifetime of the
`App`. There is no interface, no swappable backend, and no external extension point.
Plugins that need to observe or replicate mutations subscribe to the standard
`vdb.record.*` and `vdb.transaction.*` events.

**`delta.go`**

```go
package delta

// Delta is the mutation store for a VirtualDB session. It records writes
// that clients issue — inserts, updates, and deletes — without forwarding
// them to the source database. The framework overlays the stored state on
// top of source rows when assembling read results.
//
// Delta is safe for concurrent use by multiple goroutines.
type Delta struct {
    mu     sync.RWMutex
    tables map[string]*tableState
}

// New allocates and returns a ready-to-use Delta.
func New() *Delta

func (d *Delta) ApplyInsert(table string, record map[string]any) error
func (d *Delta) ApplyUpdate(table string, old, new map[string]any) error
func (d *Delta) ApplyDelete(table string, record map[string]any) error
func (d *Delta) Snapshot(connID uint32) (*Snapshot, error)
func (d *Delta) Restore(connID uint32, snap *Snapshot) error
func (d *Delta) Records(table string) ([]map[string]any, error)
func (d *Delta) TableState(table string) (DeltaTableState, error)

// Snapshot is an opaque point-in-time capture of the Delta state for one
// connection. Produced by Delta.Snapshot, consumed by Delta.Restore.
// Treat as immutable once returned.
type Snapshot struct {
    connID uint32
    tables map[string]*tableState
}

// DeltaTableState is a point-in-time snapshot of the delta for a single
// table, categorised by mutation type. All maps are keyed by RecordKey.
// The returned value is a copy; callers may read it without holding any lock.
type DeltaTableState struct {
    // Inserts holds net-new rows: rows recorded via ApplyInsert that have
    // no counterpart in the source database.
    Inserts map[string]map[string]any

    // Updates holds upsert overlays: rows recorded via ApplyUpdate whose
    // PK exists in the source database. Keyed by the stable source RecordKey.
    Updates map[string]map[string]any

    // Tombstones is the set of deleted source rows. Source rows whose
    // RecordKey appears here are excluded from query results entirely.
    Tombstones map[string]struct{}
}

// RecordKey produces a canonical string key for a record based on all its
// fields, sorted lexicographically by field name.
func RecordKey(r map[string]any) string
```

**Key properties:**
- No `DeltaBackend` interface — `Delta` is the sole concrete type.
- No `NewMemoryDeltaBackend` — the constructor is simply `New()`.
- `Snapshot` is a concrete struct with unexported fields, not an empty interface.
  This provides compile-time type safety at `Snapshot`/`Restore` call sites.
- `tableState` and all copy helpers remain unexported implementation details.

---

### 8.6 `internal/connection`

**Purpose:** Owns connection tracking state and the handlers that manage it.

**`state.go`**

```go
package connection

// State is the concurrency-safe store of per-connection bookkeeping.
type State struct { ... }

func NewState() *State
func (s *State) Set(id uint32, c *conn)
func (s *State) Get(id uint32) (*conn, bool)
func (s *State) Delete(id uint32)

type conn struct {
    ID       uint32
    User     string
    Addr     string
    Database string
    Snapshot *delta.Snapshot
}
```

**`handlers.go`**

```go
package connection

// Handlers owns all pipeline points that interact with connection state.
// Its only dependency is *State — no App, no Bus, no Pipeline.
type Handlers struct {
    state *State
}

func New(state *State) *Handlers

// Register attaches all connection handlers to r.
// Points covered:
//   connection.opened.build_context  (10) → framework.BuildContext
//   connection.opened.track          (10) → h.TrackOpened
//   connection.closed.build_context  (10) → framework.BuildContext
//   connection.closed.release        (10) → h.ReleaseOnClose
//   query.received.build_context     (10) → framework.BuildContext
//   query.received.intercept         (10) → h.UpdateDatabase
func (h *Handlers) Register(r framework.Registrar) error

func (h *Handlers) TrackOpened(ctx any, p any) (any, any, error)
func (h *Handlers) ReleaseOnClose(ctx any, p any) (any, any, error)
func (h *Handlers) UpdateDatabase(ctx any, p any) (any, any, error)
```

Test: `handlers_test.go` creates `New(connection.NewState())`, exercises each
named method directly with typed inputs, and asserts state mutations. No `App`.
No `Run()`. No goroutines.

---

### 8.7 `internal/schema`

**Purpose:** Schema cache. Self-contained; no handler logic.

```go
package schema

type Cache struct { ... }

type Entry struct {
    Columns []string
    PKCol   string
}

func NewCache() *Cache
func (c *Cache) Load(table string, columns []string, pkCol string)
func (c *Cache) Get(table string) (Entry, bool)
func (c *Cache) Invalidate(table string)
```

`schemaEntry` and `schemaCache` merge into `Entry` and `Cache`. `Entry` has exported
fields. The defensive copy logic stays inside `Get`.

---

### 8.8 `internal/lifecycle`

**Purpose:** Three lifecycle pipeline handlers (`context.create`, `server.start`,
`server.stop`). The `delta.provide` pipeline is eliminated; `Delta` is constructed
unconditionally in `core.New()` and requires no lifecycle step.

**`app.go`** — the interface lifecycle needs from the outside world

```go
package lifecycle

// App is the subset of core.App that lifecycle handlers mutate or observe.
// core.App satisfies this interface. Tests supply a stub.
// Defined here — at the point of consumption — not in core.
type App interface {
    Bus() *framework.Bus
    Pipe() *framework.Pipeline
    SetGlobal(framework.GlobalContext)
    GetServer() server
    ServerErrCh() chan<- error
    Plugins() *plugin.Manager
}

// server is the minimal shape lifecycle needs from the DB server.
// Defined locally so lifecycle does not import vdb-core (root).
type server interface {
    Run() error
    Stop() error
}
```

**`handlers.go`**

```go
package lifecycle

type Handlers struct {
    app App
}

func New(app App) *Handlers

// Register attaches all lifecycle handlers to r.
// Points covered:
//   context.create.build_context  (10) → h.ContextCreateBuild
//   context.create.seal           (10) → h.ContextCreateSeal
//   server.start.build_context    (10) → h.ServerStartBuild
//   server.start.launch           (10) → h.ServerStartLaunch
//   server.stop.build_context     (10) → h.ServerStopBuild
//   server.stop.drain             (10) → h.ServerStopDrain
//   server.stop.halt              (10) → h.ServerStopHalt
func (h *Handlers) Register(r framework.Registrar) error

func (h *Handlers) ContextCreateBuild(ctx any, p any) (any, any, error)
func (h *Handlers) ContextCreateSeal(ctx any, p any) (any, any, error)
func (h *Handlers) ServerStartBuild(ctx any, p any) (any, any, error)
func (h *Handlers) ServerStartLaunch(ctx any, p any) (any, any, error)
func (h *Handlers) ServerStopBuild(ctx any, p any) (any, any, error)
func (h *Handlers) ServerStopDrain(ctx any, p any) (any, any, error)
func (h *Handlers) ServerStopHalt(ctx any, p any) (any, any, error)
```

---

### 8.9 `internal/transaction`

**Purpose:** Transaction snapshot lifecycle.

```go
package transaction

type Handlers struct {
    conns *connection.State
    delta *delta.Delta
}

func New(conns *connection.State, d *delta.Delta) *Handlers

// Register attaches all transaction handlers to r.
// Points covered:
//   transaction.begin.build_context    (10) → framework.BuildContext
//   transaction.begin.authorize        (10) → h.Authorize
//   transaction.commit.build_context   (10) → framework.BuildContext
//   transaction.commit.apply           (10) → h.CommitApply
//   transaction.rollback.build_context (10) → framework.BuildContext
//   transaction.rollback.apply         (10) → h.RollbackApply
func (h *Handlers) Register(r framework.Registrar) error

func (h *Handlers) Authorize(ctx any, p any) (any, any, error)
func (h *Handlers) CommitApply(ctx any, p any) (any, any, error)
func (h *Handlers) RollbackApply(ctx any, p any) (any, any, error)
```

`delta *delta.Delta` is a direct field, not a `getDelta func()` closure. Because
`Delta` is constructed in `core.New()` before handler registration, it is always
non-nil when any handler is invoked. The late-binding indirection is unnecessary.

---

### 8.10 `internal/write`

**Purpose:** Write interception and delta overlay.

**`handlers.go`**

```go
package write

type Handlers struct {
    schema *schema.Cache
    delta  *delta.Delta
}

func New(schema *schema.Cache, d *delta.Delta) *Handlers

// Register attaches all write handlers to r.
// Points covered:
//   write.insert.build_context     (10) → framework.BuildContext
//   write.insert.apply             (10) → h.InsertApply
//   write.update.build_context     (10) → framework.BuildContext
//   write.update.apply             (10) → h.UpdateApply
//   write.delete.build_context     (10) → framework.BuildContext
//   write.delete.apply             (10) → h.DeleteApply
//   records.source.build_context   (10) → framework.BuildContext
//   records.source.transform       (10) → h.RecordsOverlay
//   records.merged.build_context   (10) → framework.BuildContext
func (h *Handlers) Register(r framework.Registrar) error

func (h *Handlers) InsertApply(ctx any, p any) (any, any, error)
func (h *Handlers) UpdateApply(ctx any, p any) (any, any, error)
func (h *Handlers) DeleteApply(ctx any, p any) (any, any, error)
func (h *Handlers) RecordsOverlay(ctx any, p any) (any, any, error)
```

**`overlay.go`**

```go
package write

// Overlay merges the delta state for table onto the source record slice.
// Extracted as a named function so it is unit-testable without the handler
// dispatch machinery. Returns a new slice; source is never modified.
func Overlay(
    d *delta.Delta,
    schema *schema.Cache,
    table string,
    source []map[string]any,
) ([]map[string]any, error)
```

---

### 8.11 `internal/emit`

**Purpose:** All `*.emit` point handlers. Pure bus-firing with no state mutations.

```go
package emit

// Handlers fires the standard vdb.* events from pipeline emit points.
// No mutable state.
type Handlers struct{}

func New() *Handlers

// Register attaches all emit handlers to r.
// Points covered:
//   server.stop.emit             (10) → h.ServerStopped
//   connection.opened.emit       (10) → h.ConnectionOpened
//   connection.closed.emit       (10) → h.ConnectionClosed
//   transaction.begin.emit       (10) → h.TransactionStarted
//   transaction.commit.emit      (10) → h.TransactionCommitted
//   transaction.rollback.emit    (10) → h.TransactionRolledBack
//   write.insert.emit            (10) → h.RecordInserted
//   write.update.emit            (10) → h.RecordUpdated
//   write.delete.emit            (10) → h.RecordDeleted
func (h *Handlers) Register(r framework.Registrar) error

func (h *Handlers) ServerStopped(ctx any, p any) (any, any, error)
func (h *Handlers) ConnectionOpened(ctx any, p any) (any, any, error)
func (h *Handlers) ConnectionClosed(ctx any, p any) (any, any, error)
func (h *Handlers) TransactionStarted(ctx any, p any) (any, any, error)
func (h *Handlers) TransactionCommitted(ctx any, p any) (any, any, error)
func (h *Handlers) TransactionRolledBack(ctx any, p any) (any, any, error)
func (h *Handlers) RecordInserted(ctx any, p any) (any, any, error)
func (h *Handlers) RecordUpdated(ctx any, p any) (any, any, error)
func (h *Handlers) RecordDeleted(ctx any, p any) (any, any, error)
```

---

## 9. Root Package After Refactor

### `interfaces.go`

Zero implementation. `map[string]any` used directly — no `types.Record`.
Defines all public types: `Config`, `Server`, `DriverAPI`, `PointFunc`, `EventFunc`.
`DriverReceiver` does not exist. There is no `context.go` re-export file — the
handler plumbing types (`HandlerContext`, `GlobalContext`, `Bus`, `Pipeline`, etc.)
are not part of the public API at this time.

```go
package core

type PointFunc func(ctx any, payload any) (any, any, error)
type EventFunc func(ctx any, payload any) error

type Config struct { ... }

type Server interface {
    Run() error
    Stop() error
}

type DriverAPI interface { ... } // as defined in §7
```

### `app.go`

`UseDriver` stores the server directly. There is no `DriverReceiver` type assertion
and no implicit wiring — the composition root calls `DriverAPI()` explicitly and
passes the result to the driver constructor before calling `UseDriver`.

```go
func (a *App) UseDriver(s Server) *App {
    a.mu.Lock()
    defer a.mu.Unlock()
    if a.running {
        panic("core: UseDriver called after Run")
    }
    a.server = s
    return a
}

// DriverAPI returns the framework's DriverAPI implementation. The composition
// root passes this to the driver constructor so the driver can call back into
// the framework when the database engine signals activity.
//
//   api    := app.DriverAPI()
//   driver := mysql.NewDriver(cfg, api)
//   app.UseDriver(driver)
func (a *App) DriverAPI() DriverAPI {
    return a.apiImpl
}
```


```go
package core

type App struct {
    cfg    Config
    pipe   *framework.Pipeline
    bus    *framework.Bus
    conns  *connection.State
    schema *schema.Cache
    global framework.GlobalContext

    plugins     *plugin.Manager
    server      Server
    delta       *delta.Delta
    apiImpl     *driverAPIImpl

    shutdown    chan struct{}
    serverErrCh chan error
    running     bool
    stopped     bool
    mu          sync.Mutex
}

func New(cfg Config) *App {
    app := &App{cfg: cfg}

    app.pipe = framework.NewPipeline(&app.global)
    app.bus  = framework.NewBus(&app.global)

    app.conns  = connection.NewState()
    app.schema = schema.NewCache()
    app.delta  = delta.New()

    app.apiImpl = newDriverAPIImpl(app)

    declarePipelines(app.pipe)
    declareEvents(app.bus)

    mustRegisterBuiltinHandlers(app)

    app.plugins     = plugin.NewManager(cfg.PluginDir, 0)
    app.shutdown    = make(chan struct{})
    app.serverErrCh = make(chan error, 1)

    return app
}

func mustRegisterBuiltinHandlers(app *App) {
    groups := []interface{ Register(framework.Registrar) error }{
        lifecycle.New(app),
        connection.New(app.conns),
        transaction.New(app.conns, app.delta),
        write.New(app.schema, app.delta),
        emit.New(),
    }
    for _, g := range groups {
        if err := g.Register(app.pipe); err != nil {
            panic("core: handler registration: " + err.Error())
        }
    }
}
```

`delta.New()` is called unconditionally at construction time. No `getDelta` closure.
No `SetDelta` method. `app.delta` is always non-nil after `New()` returns.

`plugin.Manager.ConnectAll` receives `app.bus` and `app.pipe` directly. It wires
plugin handler and subscription adapters onto those types without calling back
through `App`. The `pluginHost` interface is deleted from `internal/plugin`.

---

## 10. The Handler Method Contract

Every handler method on every `Handlers` struct has the signature:

```go
func (h *Handlers) MethodName(ctx any, p any) (any, any, error)
```

Conventions:

1. **Assert ctx first.** `hctx := ctx.(framework.HandlerContext)`. Omit only when
   the method does not use `hctx` at all.
2. **Assert payload second.** Use a type switch or direct assertion. Return a
   well-formed error on unexpected type — never panic.
3. **Return the updated ctx and payload.** The pipeline threads them forward.
4. **No side-effects on the input payload.** Modify a copy; return the copy.

---

## 11. Testing Strategy

### Tier 1: Pure unit tests (no App, no goroutines)

| Package | Test subject |
|---------|-------------|
| `internal/framework` | `GlobalContextBuilder` seal-once; `GlobalContext.Get/Bus/Pipeline`; `BuildContext` stamps CorrelationID; `Pipeline.Process` zero-value safety |
| `internal/delta` | `Delta` concurrent apply/snapshot/restore; `TableState` categorisation; `RecordKey` stability; update-then-delete and insert-then-update edge cases |
| `internal/plugin` | `Manager.ConnectAll` wires adapters directly onto `*framework.Bus` and `*framework.Pipeline` via stub implementations; no `pluginHost` interface |
| `internal/connection` | `State` concurrent get/set/delete; each `Handlers` method with typed payloads and a real `State` |
| `internal/schema` | `Cache` load/get/invalidate; defensive copy |
| `internal/transaction` | Each `Handlers` method; `delta *delta.Delta` accessed directly (not through a closure) |
| `internal/write` | Each `Handlers` method; `Overlay` exhaustive table-driven coverage |
| `internal/emit` | Each `Handlers` method verifies correct event name and payload reach a stub bus |
| `internal/lifecycle` | Each `Handlers` method with a stub `lifecycle.App` |

### Tier 2: Handler integration tests (newTestApp, no Run)

`driver_api_test.go` creates `newTestApp()`, drives the full handler stack through
`app.apiImpl`, and asserts observable side effects. These tests remain white-box
because they need `app.conns`, `app.schema`, and `app.delta` for assertion.

### Tier 3: Lifecycle integration tests (uses Run/Stop)

`run_test.go` drives the full `App.Run()` → `App.Stop()` sequence.

### Shared test helpers

`testhelpers_test.go` (package `core`) provides `newTestApp()` and `waitEvent()`.
One location, no duplication across test files.

---

## 12. Migration Steps

Perform one step at a time. Run `go test ./... -race` after each step.
Never let the build go red between commits.

| Step | Action |
|------|--------|
| 1 | **Restructure `delta/`:** rename `MemoryDeltaBackend` → `Delta`; rename `NewMemoryDeltaBackend` → `New`; remove the `DeltaBackend` interface entirely; replace `DeltaSnapshot interface{}` with a concrete `Snapshot` struct; update `DeltaTableState` to use `map[string]any` instead of `types.Record`; merge `backend.go` into `delta.go`; move `delta/` → `internal/delta/`. Update `core.go`/`handlers.go` callsites. Remove `DeltaProvidePayload` from `payloads/`. Remove `delta.provide` pipeline declaration and `DeltaProvideBuild`/`DeltaProvideInstall` handler registrations. Add `app.delta = delta.New()` to `New()`. Remove `getDelta` closure and `SetDelta`. Build green. |
| 1a | **Clean up plugin bridge and public API:** delete the `pluginHost` interface from `plugin/manager.go`; change `ConnectAll` to accept `*framework.Bus` and `*framework.Pipeline` directly. Remove `RegisterPlugin`, `SubscribePlugin`, `DeclarePipelinePlugin`, and `DriverReceiver` from `core`. Add `DeclarePipeline(name string, points []string) *App` and `DriverAPI() DriverAPI` to `app.go`/`app_api.go`. Delete `context.go` (the re-export file); move `PointFunc` and `EventFunc` into `interfaces.go` as plain type definitions. Remove `UseDriver`'s `DriverReceiver` type assertion. Build green. |
| 2 | Create `internal/framework/` — move `HandlerContext`, `GlobalContext`, `GlobalContextBuilder`, `Bus`, `Pipeline`, `CorrelationID`, `sealContext`, `newCorrelationID`, `newID` from `core.go`. Add `BuildContext` and `Registrar`. |
| 3 | Create `context.go` at root — thin re-export file with type aliases. Remove moved symbols from `core.go`. Build green. |
| 4 | Move `internal/points/` — copy current `points/` contents, change package path, remove all `vdb.delta.provide.*` constants. Update all internal callsites. Delete public `points/`. Build green. |
| 5 | Move `internal/payloads/` — copy remaining payload files (excluding the already-removed `delta.go`), change package path. Update all internal callsites. Delete public `payloads/`. Build green. |
| 6 | Move `internal/plugin/` — same pattern. Delete public `plugin/`. Build green. |
| 7 | Delete `types/`. Replace all remaining `types.Record` references in `interfaces.go` and `driver_api.go` with `map[string]any`. Build green. |
| 8 | Create `internal/schema/cache.go` — move `schemaCache` + `schemaEntry`. Update callsites. |
| 9 | Create `internal/connection/state.go` — move `connState` + `connMap`. Update `conn.Snapshot` field to `*delta.Snapshot`. Update callsites. |
| 10 | Create `internal/connection/handlers.go` — extract `registerConnHandlers` logic into named `Handlers` methods. Write `handlers_test.go`. |
| 11 | Create `internal/emit/handlers.go` — extract emit handlers into named `Handlers` methods. Write `handlers_test.go`. |
| 12 | Create `internal/transaction/handlers.go` — extract transaction handlers. Inject `*delta.Delta` directly as a struct field. Write `handlers_test.go`. |
| 13 | Create `internal/write/handlers.go` and `overlay.go` — extract write handlers and overlay. Inject `*delta.Delta` directly. Write tests. |
| 14 | Create `internal/lifecycle/app.go` and `handlers.go` — extract lifecycle handlers. Define `lifecycle.App` interface without `SetDelta`. Implement on `core.App`. Write `handlers_test.go` with a stub `App`. |
| 15 | Rewrite `app.go` — `New()` constructs `delta.New()` unconditionally; `mustRegisterBuiltinHandlers()` passes `app.delta` directly to domain constructors. Zero anonymous closures in the registration path. |
| 16 | Create `interfaces.go` — move `Config`, `Server`, `DriverReceiver`, `DriverAPI` (with `map[string]any`). |
| 17 | Create `app_api.go` — move extension API methods. |
| 18 | Delete `handlers.go`, `conn.go`, `schema.go`, `core.go`. Rename remainder to `app.go`. |
| 19 | Write missing unit tests for `internal/framework` and `internal/delta`. Add `testhelpers_test.go`. Rename `core_handlers_test.go` → `driver_api_test.go`. |
| 20 | Audit all file line counts. Any file above 200 lines gets a targeted split. |

---

## 13. Invariants After Refactor

| # | Invariant | Verify with |
|---|-----------|-------------|
| 1 | `pipeline` and `dispatch` are imported only by `internal/framework` | `grep -r '"github.com/AnqorDX/pipeline"' . --include="*.go"` |
| 2 | No file in `internal/` imports the root `vdb-core` package | `grep -r '"github.com/AnqorDX/vdb-core"' internal/ --include="*.go"` |
| 3 | `types/` directory does not exist | `ls types/` returns not found |
| 4 | `points/`, `payloads/`, `plugin/`, `delta/` do not exist as public packages | `ls points/ payloads/ plugin/ delta/` returns not found |
| 5 | `interfaces.go` contains no function bodies and no sub-package imports | Manual — file is ~55 lines |
| 6 | `app.go:mustRegisterBuiltinHandlers` contains no anonymous functions | Manual |
| 7 | Every `Handlers.Register` uses only `r.Register(...)` calls | Manual — no direct `app.*` access |
| 8 | `go test ./... -race` passes at every step | CI |
| 9 | No file exceeds 200 lines | `awk 'END{if(NR>200)print FILENAME,NR}' **/*.go` |
| 10 | `DeltaBackend` does not exist anywhere in the module | `grep -r 'DeltaBackend' . --include="*.go"` returns nothing |
| 11 | `MemoryDeltaBackend` does not exist anywhere in the module | `grep -r 'MemoryDeltaBackend' . --include="*.go"` returns nothing |
| 12 | `delta.New()` is called exactly once, in `core.New()` | Manual — `app.delta` is the only `*delta.Delta` in the root package |
| 13 | `app.delta` is never nil after `core.New()` returns | Verified by Invariant 12 — unconditional construction |
| 14 | No `getDelta` closure exists anywhere in the module | `grep -r 'getDelta' . --include="*.go"` returns nothing |
| 15 | `internal/delta` has no imports from this module | `grep -r '"github.com/AnqorDX' internal/delta/ --include="*.go"` returns nothing |

| 16 | `pluginHost` interface does not exist anywhere in the module | `grep -r 'pluginHost' . --include="*.go"` returns nothing |
| 17 | `DriverReceiver`, `RegisterPlugin`, `SubscribePlugin`, `DeclarePipelinePlugin` are not exported from the module | `grep -r 'DriverReceiver\|RegisterPlugin\|SubscribePlugin\|DeclarePipelinePlugin' . --include="*.go"` returns nothing |
| 18 | `HandlerContext`, `GlobalContext`, `GlobalContextBuilder`, `CorrelationID`, `Bus`, `EventBus`, `Pipeline` are not exported from the root package | `grep -rn 'type Hand\|type Global\|type Correl\|type Bus\|type Event\|type Pipe' *.go` returns nothing |
| 19 | Public exported types in root package are exactly: `App`, `Config`, `Server`, `DriverAPI`, `PointFunc`, `EventFunc` | Manual — exported symbol audit of all `*.go` files in root |
| 20 | Public methods on `App` are exactly: `New`, `UseDriver`, `DriverAPI`, `Run`, `Stop`, `Attach`, `Subscribe`, `DeclareEvent`, `DeclarePipeline`, `Emit`, `Process` | Manual — `app.go` + `app_api.go` exported symbol audit |