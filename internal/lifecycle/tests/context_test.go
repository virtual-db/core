// Package lifecycle_test exercises the lifecycle.Handlers using a real
// framework.Pipeline and framework.Bus. No stubs for the registrar.
package lifecycle_test

import (
	"testing"

	"github.com/AnqorDX/vdb-core/internal/framework"
	. "github.com/AnqorDX/vdb-core/internal/lifecycle"
	"github.com/AnqorDX/vdb-core/internal/plugin"
	"github.com/AnqorDX/vdb-core/internal/points"
)

// ---------------------------------------------------------------------------
// stubApp — satisfies lifecycle.App without importing the root package.
// Shared across context_test.go and server_test.go (same package namespace).
// ---------------------------------------------------------------------------

type stubApp struct {
	bus     *framework.Bus
	pipe    *framework.Pipeline
	global  framework.GlobalContext
	server  Server
	errCh   chan error
	plugins *plugin.Manager
}

func (a *stubApp) Bus() *framework.Bus                 { return a.bus }
func (a *stubApp) Pipe() *framework.Pipeline           { return a.pipe }
func (a *stubApp) SetGlobal(g framework.GlobalContext) { a.global = g }
func (a *stubApp) GetServer() Server                   { return a.server }
func (a *stubApp) ServerErrCh() chan<- error           { return a.errCh }
func (a *stubApp) Plugins() *plugin.Manager            { return a.plugins }

// ---------------------------------------------------------------------------
// Helper: newLifecycleEnv
// ---------------------------------------------------------------------------

// newLifecycleEnv builds a sealed env with the context.create pipeline
// declared and the lifecycle handlers registered.
func newLifecycleEnv(t *testing.T) (*framework.Pipeline, *stubApp) {
	t.Helper()

	var global framework.GlobalContext
	pipe := framework.NewPipeline(&global)
	bus := framework.NewBus(&global)
	global = framework.SealContext(framework.NewGlobalContextBuilder(), bus, pipe)

	app := &stubApp{
		bus:     bus,
		pipe:    pipe,
		errCh:   make(chan error, 1),
		plugins: plugin.NewManager("", 0),
	}

	pipe.DeclarePipeline(points.PipelineContextCreate, []string{
		points.PointContextCreateBuildContext,
		points.PointContextCreateContribute,
		points.PointContextCreateSeal,
		points.PointContextCreateEmit,
	})
	pipe.DeclarePipeline(points.PipelineServerStart, []string{
		points.PointServerStartBuildContext,
		points.PointServerStartConfigure,
		points.PointServerStartLaunch,
		points.PointServerStartEmit,
	})
	pipe.DeclarePipeline(points.PipelineServerStop, []string{
		points.PointServerStopBuildContext,
		points.PointServerStopDrain,
		points.PointServerStopHalt,
	})

	h := New(app)
	if err := h.Register(pipe); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return pipe, app
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestContextCreate_SealIsCalledWithBuilderPayload verifies that processing
// vdb.context.create causes SetGlobal to be called with a real sealed context
// (one whose Bus() is non-nil).
func TestContextCreate_SealIsCalledWithBuilderPayload(t *testing.T) {
	pipe, app := newLifecycleEnv(t)

	if _, err := pipe.Process(points.PipelineContextCreate, nil); err != nil {
		t.Fatalf("Process: %v", err)
	}

	if app.global.Bus() == nil {
		t.Fatal("app.global.Bus() is nil — SetGlobal was not called with a sealed context")
	}
}

// TestContextCreate_ContributeHandlerCanAddValues verifies that a handler
// attached at vdb.context.create.contribute (priority 5, before the seal
// handler at 10) can add arbitrary values to the GlobalContextBuilder, and
// that those values survive sealing and are accessible on the final global.
func TestContextCreate_ContributeHandlerCanAddValues(t *testing.T) {
	pipe, app := newLifecycleEnv(t)

	// Attach a contributor at priority 5 — runs before the seal handler (10).
	if err := pipe.Attach(points.PointContextCreateContribute, 5, func(ctx any, p any) (any, any, error) {
		b, ok := p.(*framework.GlobalContextBuilder)
		if !ok {
			t.Errorf("contribute handler: expected *framework.GlobalContextBuilder, got %T", p)
			return ctx, p, nil
		}
		b.Set("key", "val")
		return ctx, p, nil
	}); err != nil {
		t.Fatalf("Attach contribute handler: %v", err)
	}

	if _, err := pipe.Process(points.PipelineContextCreate, nil); err != nil {
		t.Fatalf("Process: %v", err)
	}

	if got := app.global.Get("key"); got != "val" {
		t.Errorf(`app.global.Get("key") = %q, want "val"`, got)
	}
}
