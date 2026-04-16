package connection_test

import (
	"testing"

	. "github.com/AnqorDX/vdb-core/internal/connection"
	"github.com/AnqorDX/vdb-core/internal/framework"
	"github.com/AnqorDX/vdb-core/internal/payloads"
	"github.com/AnqorDX/vdb-core/internal/points"
)

// newConnPipe constructs a sealed Pipeline with all connection and query
// pipelines declared, registers the connection Handlers, and returns the
// Pipeline together with the State it operates on.
func newConnPipe(t *testing.T) (*framework.Pipeline, *State) {
	t.Helper()
	var global framework.GlobalContext
	pipe := framework.NewPipeline(&global)
	bus := framework.NewBus(&global)
	global = framework.SealContext(framework.NewGlobalContextBuilder(), bus, pipe)

	pipe.DeclarePipeline(points.PipelineConnectionOpened, []string{
		points.PointConnectionOpenedBuildContext,
		points.PointConnectionOpenedAccept,
		points.PointConnectionOpenedTrack,
		points.PointConnectionOpenedEmit,
	})
	pipe.DeclarePipeline(points.PipelineConnectionClosed, []string{
		points.PointConnectionClosedBuildContext,
		points.PointConnectionClosedCleanup,
		points.PointConnectionClosedRelease,
		points.PointConnectionClosedEmit,
	})
	pipe.DeclarePipeline(points.PipelineQueryReceived, []string{
		points.PointQueryReceivedBuildContext,
		points.PointQueryReceivedIntercept,
		points.PointQueryReceivedEmit,
	})

	state := NewState()
	h := New(state)
	if err := h.Register(pipe); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return pipe, state
}

// newDeclaredPipe constructs a sealed Pipeline with all connection and query
// pipelines declared but no handlers registered. Used to test Register in
// isolation without double-registration.
func newDeclaredPipe(t *testing.T) *framework.Pipeline {
	t.Helper()
	var global framework.GlobalContext
	pipe := framework.NewPipeline(&global)
	bus := framework.NewBus(&global)
	global = framework.SealContext(framework.NewGlobalContextBuilder(), bus, pipe)

	pipe.DeclarePipeline(points.PipelineConnectionOpened, []string{
		points.PointConnectionOpenedBuildContext,
		points.PointConnectionOpenedAccept,
		points.PointConnectionOpenedTrack,
		points.PointConnectionOpenedEmit,
	})
	pipe.DeclarePipeline(points.PipelineConnectionClosed, []string{
		points.PointConnectionClosedBuildContext,
		points.PointConnectionClosedCleanup,
		points.PointConnectionClosedRelease,
		points.PointConnectionClosedEmit,
	})
	pipe.DeclarePipeline(points.PipelineQueryReceived, []string{
		points.PointQueryReceivedBuildContext,
		points.PointQueryReceivedIntercept,
		points.PointQueryReceivedEmit,
	})

	return pipe
}

// TestHandlers_ConnectionOpened_TracksConnection verifies that processing the
// PipelineConnectionOpened pipeline stores the connection in State with the
// correct User and Addr fields.
func TestHandlers_ConnectionOpened_TracksConnection(t *testing.T) {
	pipe, state := newConnPipe(t)

	payload := payloads.ConnectionOpenedPayload{
		ConnectionID: 1,
		User:         "alice",
		Address:      "127.0.0.1",
	}
	if _, err := pipe.Process(points.PipelineConnectionOpened, payload); err != nil {
		t.Fatalf("Process: %v", err)
	}

	conn, ok := state.Get(1)
	if !ok {
		t.Fatal("expected ok==true after ConnectionOpened, got false")
	}
	if conn.User != "alice" {
		t.Errorf("User: want %q, got %q", "alice", conn.User)
	}
	if conn.Addr != "127.0.0.1" {
		t.Errorf("Addr: want %q, got %q", "127.0.0.1", conn.Addr)
	}
}

// TestHandlers_ConnectionClosed_RemovesConnection verifies that processing the
// PipelineConnectionClosed pipeline removes the connection from State.
func TestHandlers_ConnectionClosed_RemovesConnection(t *testing.T) {
	pipe, state := newConnPipe(t)

	// Pre-populate state directly so we don't depend on the opened pipeline here.
	state.Set(1, &Conn{ID: 1})

	payload := payloads.ConnectionClosedPayload{ConnectionID: 1}
	if _, err := pipe.Process(points.PipelineConnectionClosed, payload); err != nil {
		t.Fatalf("Process: %v", err)
	}

	_, ok := state.Get(1)
	if ok {
		t.Fatal("expected ok==false after ConnectionClosed, got true")
	}
}

// TestHandlers_QueryReceived_UpdatesDatabase verifies that processing the
// PipelineQueryReceived pipeline updates the Database field on the tracked Conn.
func TestHandlers_QueryReceived_UpdatesDatabase(t *testing.T) {
	pipe, state := newConnPipe(t)

	// Pre-populate state with an empty Database.
	state.Set(1, &Conn{ID: 1, User: "alice"})

	payload := payloads.QueryReceivedPayload{
		ConnectionID: 1,
		Query:        "USE mydb",
		Database:     "mydb",
	}
	if _, err := pipe.Process(points.PipelineQueryReceived, payload); err != nil {
		t.Fatalf("Process: %v", err)
	}

	conn, ok := state.Get(1)
	if !ok {
		t.Fatal("expected ok==true after QueryReceived, got false")
	}
	if conn.Database != "mydb" {
		t.Errorf("Database: want %q, got %q", "mydb", conn.Database)
	}
}

// TestHandlers_Register_ReturnsNilError verifies that Register attaches all
// handlers without returning an error when all required pipeline points have
// been declared.
func TestHandlers_Register_ReturnsNilError(t *testing.T) {
	pipe := newDeclaredPipe(t)

	err := New(NewState()).Register(pipe)
	if err != nil {
		t.Fatalf("Register returned unexpected error: %v", err)
	}
}
