package driverapi_test

import (
	"errors"
	"testing"

	"github.com/AnqorDX/vdb-core/internal/connection"
	. "github.com/AnqorDX/vdb-core/internal/driverapi"
	"github.com/AnqorDX/vdb-core/internal/payloads"
	"github.com/AnqorDX/vdb-core/internal/points"
)

// compile-time check: Impl is reachable via the dot import.
var _ *Impl

// ---------------------------------------------------------------------------
// QueryReceived
// ---------------------------------------------------------------------------

func TestQueryReceived_HandlerCanRewriteQuery(t *testing.T) {
	impl, pipe, _, _, _ := newTestImpl(t)

	pipe.DeclarePipeline(points.PipelineQueryReceived, []string{points.PointQueryReceivedIntercept})
	pipe.MustAttach(points.PointQueryReceivedIntercept, 10, func(ctx any, p any) (any, any, error) {
		rewritten := payloads.QueryReceivedPayload{Query: "SELECT 2"}
		return ctx, rewritten, nil
	})

	got, err := impl.QueryReceived(1, "SELECT 1", "db")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "SELECT 2" {
		t.Fatalf("expected rewritten query %q, got %q", "SELECT 2", got)
	}
}

func TestQueryReceived_HandlerError_IsReturned(t *testing.T) {
	impl, pipe, _, _, _ := newTestImpl(t)

	pipe.DeclarePipeline(points.PipelineQueryReceived, []string{points.PointQueryReceivedIntercept})
	pipe.MustAttach(points.PointQueryReceivedIntercept, 10, func(ctx any, p any) (any, any, error) {
		return ctx, p, errors.New("injected query received error")
	})

	got, err := impl.QueryReceived(1, "SELECT 1", "db")
	if err == nil {
		t.Fatal("expected non-nil error from handler, got nil")
	}
	if got != "" {
		t.Fatalf("expected empty string on error, got %q", got)
	}
}

func TestQueryReceived_NoRewrite_PassesOriginal(t *testing.T) {
	impl, pipe, _, _, _ := newTestImpl(t)

	pipe.DeclarePipeline(points.PipelineQueryReceived, []string{points.PointQueryReceivedIntercept})
	pipe.MustAttach(points.PointQueryReceivedIntercept, 10, func(ctx any, p any) (any, any, error) {
		// Return the payload unchanged; original query must survive.
		return ctx, payloads.QueryReceivedPayload{Query: "SELECT 1"}, nil
	})

	got, err := impl.QueryReceived(1, "SELECT 1", "db")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "SELECT 1" {
		t.Fatalf("expected original query %q, got %q", "SELECT 1", got)
	}
}

// ---------------------------------------------------------------------------
// QueryCompleted
// ---------------------------------------------------------------------------

func TestQueryCompleted_EmitsEventOnBus(t *testing.T) {
	impl, _, bus, _, _ := newTestImpl(t)

	done := make(chan struct{})
	bus.DeclareEvent(points.EventQueryCompleted)
	if err := bus.Subscribe(points.EventQueryCompleted, func(_ any, _ any) error {
		close(done)
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	impl.QueryCompleted(1, "SELECT 1", 0, nil)

	waitDone(t, done, "EventQueryCompleted to fire")
}

func TestQueryCompleted_IncludesDatabaseFromConnectionState(t *testing.T) {
	impl, _, bus, conns, _ := newTestImpl(t)

	// Pre-populate the connection with a known database name.
	conns.Set(1, &connection.Conn{ID: 1, Database: "testdb"})

	done := make(chan struct{})
	var received any

	bus.DeclareEvent(points.EventQueryCompleted)
	if err := bus.Subscribe(points.EventQueryCompleted, func(_ any, p any) error {
		received = p
		close(done)
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	impl.QueryCompleted(1, "SELECT 1", 5, nil)

	waitDone(t, done, "EventQueryCompleted with database field")

	qp, ok := received.(payloads.QueryCompletedPayload)
	if !ok {
		t.Fatalf("expected QueryCompletedPayload, got %T", received)
	}
	if qp.Database != "testdb" {
		t.Fatalf("expected Database %q, got %q", "testdb", qp.Database)
	}
	if qp.RowsAffected != 5 {
		t.Fatalf("expected RowsAffected 5, got %d", qp.RowsAffected)
	}
}
