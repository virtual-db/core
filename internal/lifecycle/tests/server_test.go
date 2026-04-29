// Package lifecycle_test exercises the lifecycle.Handlers using a real
// framework.Pipeline and framework.Bus. No stubs for the registrar.
// stubApp is defined in context_test.go (same package namespace).
//
// vdb.server.stop is a host-only pipeline: plugin cleanup on shutdown is
// handled by the shutdown JSON-RPC request sent at the drain point, not by
// plugin pipeline handlers. The tests in this file exercise the internal
// mechanics of the stop sequence using in-process handlers and stubs.
package lifecycle_test

import (
	"testing"
	"time"

	"github.com/virtual-db/core/internal/framework"
	. "github.com/virtual-db/core/internal/lifecycle"
	"github.com/virtual-db/core/internal/plugin"
	"github.com/virtual-db/core/internal/points"
)

// ---------------------------------------------------------------------------
// stubServer — satisfies lifecycle.Server for testing.
// ---------------------------------------------------------------------------

type stubServer struct {
	runCalled  bool
	stopCalled bool
	stopErr    error
}

func (s *stubServer) Run() error  { s.runCalled = true; return nil }
func (s *stubServer) Stop() error { s.stopCalled = true; return s.stopErr }

// ---------------------------------------------------------------------------
// Helper: newServerEnv
// ---------------------------------------------------------------------------

// newServerEnv builds a sealed env with the server.start and server.stop
// pipelines declared, a stubServer wired into the app, and the lifecycle
// handlers registered.
func newServerEnv(t *testing.T) (*framework.Pipeline, *stubApp, *stubServer) {
	t.Helper()

	var global framework.GlobalContext
	pipe := framework.NewPipeline(&global)
	bus := framework.NewBus(&global)
	global = framework.SealContext(framework.NewGlobalContextBuilder(), bus, pipe)

	srv := &stubServer{}
	app := &stubApp{
		bus:     bus,
		pipe:    pipe,
		errCh:   make(chan error, 1),
		plugins: plugin.NewManager("", 0),
		server:  srv,
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
		points.PointServerStopEmit,
	})

	h := New(app)
	if err := h.Register(pipe); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return pipe, app, srv
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestServerStart_Launch_CallsServerRunInGoroutine verifies that processing
// vdb.server.start causes srv.Run() to be invoked asynchronously. The stub
// returns immediately; we drain app.errCh with a deadline to confirm the
// goroutine fired and srv.runCalled is set.
//
// vdb.server.start is plugin-accessible (runs after ConnectAll). This test
// exercises the built-in launch handler in isolation.
func TestServerStart_Launch_CallsServerRunInGoroutine(t *testing.T) {
	pipe, app, srv := newServerEnv(t)

	if _, err := pipe.Process(points.PipelineServerStart, nil); err != nil {
		t.Fatalf("Process: %v", err)
	}

	select {
	case err := <-app.errCh:
		if err != nil {
			t.Errorf("srv.Run returned unexpected error: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout: srv.Run result was not delivered to errCh within 200ms")
	}

	if !srv.runCalled {
		t.Error("srv.runCalled is false — srv.Run was not invoked")
	}
}

// TestServerStop_Halt_CallsServerStop verifies that processing vdb.server.stop
// synchronously calls srv.Stop() before Process returns.
//
// vdb.server.stop is host-only: all points in this pipeline are internal to
// the framework shutdown sequence. Plugin cleanup happens via the shutdown RPC
// at the drain point, not via plugin pipeline handlers.
func TestServerStop_Halt_CallsServerStop(t *testing.T) {
	pipe, _, srv := newServerEnv(t)

	if _, err := pipe.Process(points.PipelineServerStop, nil); err != nil {
		t.Fatalf("Process: %v", err)
	}

	if !srv.stopCalled {
		t.Error("srv.stopCalled is false — srv.Stop was not invoked")
	}
}

// TestServerStop_Drain_CallsPluginsShutdown verifies that the drain step does
// not panic when plugin.Manager.Shutdown is called on an empty manager, and
// that Process returns a nil error.
//
// The drain point is the mechanism by which all connected plugin processes are
// terminated before the server listener is halted. It is internal to the
// framework; there is no plugin expansion value here beyond the shutdown RPC
// that each plugin receives.
func TestServerStop_Drain_CallsPluginsShutdown(t *testing.T) {
	pipe, _, _ := newServerEnv(t)

	if _, err := pipe.Process(points.PipelineServerStop, nil); err != nil {
		t.Fatalf("Process returned unexpected error: %v", err)
	}
}

// TestServerStop_NilServer_NoOp verifies that when app.GetServer() returns nil
// the halt handler is a no-op: no panic and Process returns nil.
//
// This covers the case where App.Run is called without a registered driver.
func TestServerStop_NilServer_NoOp(t *testing.T) {
	pipe, app, _ := newServerEnv(t)
	app.server = nil

	if _, err := pipe.Process(points.PipelineServerStop, nil); err != nil {
		t.Fatalf("Process returned unexpected error with nil server: %v", err)
	}
}
