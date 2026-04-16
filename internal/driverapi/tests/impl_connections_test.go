package driverapi_test

import (
	"errors"
	"testing"
	"time"

	"github.com/AnqorDX/vdb-core/internal/connection"
	. "github.com/AnqorDX/vdb-core/internal/driverapi"
	"github.com/AnqorDX/vdb-core/internal/framework"
	"github.com/AnqorDX/vdb-core/internal/points"
	"github.com/AnqorDX/vdb-core/internal/schema"
)

// newTestImpl wires a fully-sealed global context and returns the Impl together
// with the four subsystems so individual tests can register pipelines/events.
func newTestImpl(t *testing.T) (*Impl, *framework.Pipeline, *framework.Bus, *connection.State, *schema.Cache) {
	t.Helper()
	var global framework.GlobalContext
	pipe := framework.NewPipeline(&global)
	bus := framework.NewBus(&global)
	conns := connection.NewState()
	sch := schema.NewCache()
	global = framework.SealContext(framework.NewGlobalContextBuilder(), bus, pipe)
	return New(pipe, bus, conns, sch), pipe, bus, conns, sch
}

// waitDone blocks until done is closed or 200 ms elapse, at which point it
// fails the test with msg.
func waitDone(t *testing.T, done <-chan struct{}, msg string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timeout waiting for %s", msg)
	}
}

// ---------------------------------------------------------------------------
// ConnectionOpened
// ---------------------------------------------------------------------------

func TestConnectionOpened_InvokesPipeline(t *testing.T) {
	impl, pipe, _, _, _ := newTestImpl(t)

	var called bool
	pipe.DeclarePipeline(points.PipelineConnectionOpened, []string{points.PointConnectionOpenedTrack})
	pipe.MustAttach(points.PointConnectionOpenedTrack, 10, func(ctx any, p any) (any, any, error) {
		called = true
		return ctx, p, nil
	})

	if err := impl.ConnectionOpened(1, "alice", "127.0.0.1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected pipeline handler to be called, but it was not")
	}
}

func TestConnectionOpened_EmptyPipeline_ReturnsNil(t *testing.T) {
	impl, pipe, _, _, _ := newTestImpl(t)

	// Declare the pipeline but attach no handler — Process runs zero handlers.
	pipe.DeclarePipeline(points.PipelineConnectionOpened, []string{points.PointConnectionOpenedTrack})

	if err := impl.ConnectionOpened(1, "alice", "127.0.0.1"); err != nil {
		t.Fatalf("expected nil error for empty pipeline, got %v", err)
	}
}

func TestConnectionOpened_HandlerError_IsReturned(t *testing.T) {
	impl, pipe, _, _, _ := newTestImpl(t)

	pipe.DeclarePipeline(points.PipelineConnectionOpened, []string{points.PointConnectionOpenedTrack})
	pipe.MustAttach(points.PointConnectionOpenedTrack, 10, func(ctx any, p any) (any, any, error) {
		return ctx, p, errors.New("injected connection opened error")
	})

	if err := impl.ConnectionOpened(1, "alice", "127.0.0.1"); err == nil {
		t.Fatal("expected non-nil error from handler, got nil")
	}
}

// ---------------------------------------------------------------------------
// ConnectionClosed
// ---------------------------------------------------------------------------

// TestConnectionClosed_InvokesPipeline verifies that ConnectionClosed calls
// pipe.Process synchronously. Errors are logged and not returned, but the
// handler is still invoked.
func TestConnectionClosed_InvokesPipeline(t *testing.T) {
	impl, pipe, _, _, _ := newTestImpl(t)

	var called bool
	pipe.DeclarePipeline(points.PipelineConnectionClosed, []string{points.PointConnectionClosedRelease})
	pipe.MustAttach(points.PointConnectionClosedRelease, 10, func(ctx any, p any) (any, any, error) {
		called = true
		return ctx, p, nil
	})

	impl.ConnectionClosed(1, "alice", "127.0.0.1")

	if !called {
		t.Fatal("expected pipeline handler to be called, but it was not")
	}
}
