// Package lifecycle provides pipeline handlers for the framework's lifecycle
// pipelines: vdb.context.create, vdb.server.start, and vdb.server.stop.
package lifecycle

import (
	"github.com/AnqorDX/vdb-core/internal/framework"
	"github.com/AnqorDX/vdb-core/internal/plugin"
)

// Server is the minimal shape lifecycle needs from the DB server.
// Defined here so lifecycle does not import the root vdb-core package.
// core.Server satisfies this interface structurally.
type Server interface {
	Run() error
	Stop() error
}

// App is the subset of core.App that lifecycle handlers mutate or observe.
// core.App satisfies this interface. Tests supply a stub.
type App interface {
	Bus() *framework.Bus
	Pipe() *framework.Pipeline
	SetGlobal(framework.GlobalContext)
	GetServer() Server
	ServerErrCh() chan<- error
	Plugins() *plugin.Manager
}
