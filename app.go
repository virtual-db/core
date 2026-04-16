package core

import (
	"sync"

	"github.com/AnqorDX/vdb-core/internal/connection"
	"github.com/AnqorDX/vdb-core/internal/delta"
	"github.com/AnqorDX/vdb-core/internal/driverapi"
	"github.com/AnqorDX/vdb-core/internal/emit"
	"github.com/AnqorDX/vdb-core/internal/framework"
	"github.com/AnqorDX/vdb-core/internal/lifecycle"
	"github.com/AnqorDX/vdb-core/internal/plugin"
	"github.com/AnqorDX/vdb-core/internal/schema"
	"github.com/AnqorDX/vdb-core/internal/transaction"
	"github.com/AnqorDX/vdb-core/internal/write"
)

// Compile-time check: driverapi.Impl must satisfy the root DriverAPI interface.
var _ DriverAPI = (*driverapi.Impl)(nil)

// lifecycleAppAdapter is a private adapter that implements lifecycle.App on behalf
// of *App. This keeps the lifecycle-only methods off the public *App surface.
type lifecycleAppAdapter struct {
	app *App
}

func (a lifecycleAppAdapter) Bus() *framework.Bus                 { return a.app.bus }
func (a lifecycleAppAdapter) Pipe() *framework.Pipeline           { return a.app.pipe }
func (a lifecycleAppAdapter) SetGlobal(g framework.GlobalContext) { a.app.global = g }
func (a lifecycleAppAdapter) GetServer() lifecycle.Server         { return a.app.server }
func (a lifecycleAppAdapter) ServerErrCh() chan<- error           { return a.app.serverErrCh }
func (a lifecycleAppAdapter) Plugins() *plugin.Manager            { return a.app.plugins }

// App is the central framework object. It holds the pipeline registry, event bus,
// global context, server, delta store, schema cache, connection map, plugin
// manager, and configuration for the lifetime of the process.
type App struct {
	cfg    Config
	pipe   *framework.Pipeline
	bus    *framework.Bus
	conns  *connection.State
	schema *schema.Cache
	global framework.GlobalContext

	plugins *plugin.Manager
	server  Server
	delta   *delta.Delta
	apiImpl DriverAPI

	shutdown    chan struct{}
	serverErrCh chan error
	running     bool
	stopped     bool
	mu          sync.Mutex
}

// New creates and returns a fully initialised *App ready for the configuration phase.
func New(cfg Config) *App {
	app := &App{cfg: cfg}

	app.pipe = framework.NewPipeline(&app.global)
	app.bus = framework.NewBus(&app.global)

	app.conns = connection.NewState()
	app.schema = schema.NewCache()
	app.delta = delta.New()

	app.apiImpl = driverapi.New(app.pipe, app.bus, app.conns, app.schema)

	declarePipelines(app.pipe)
	declareEvents(app.bus)

	mustRegisterBuiltinHandlers(app)

	app.plugins = plugin.NewManager(cfg.PluginDir, 0)
	app.shutdown = make(chan struct{})
	app.serverErrCh = make(chan error, 1)

	return app
}

// mustRegisterBuiltinHandlers registers all built-in framework handlers.
// No anonymous functions — all logic lives in named handler struct methods.
func mustRegisterBuiltinHandlers(app *App) {
	groups := []interface {
		Register(framework.Registrar) error
	}{
		lifecycle.New(lifecycleAppAdapter{app}),
		connection.New(app.conns),
		transaction.New(app.conns, app.delta),
		write.New(app.schema, app.delta, app.conns),
		emit.New(),
	}
	for _, g := range groups {
		if err := g.Register(app.pipe); err != nil {
			panic("core: handler registration: " + err.Error())
		}
	}
}
