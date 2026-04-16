package driverapi_test

import (
	"errors"
	"testing"

	. "github.com/AnqorDX/vdb-core/internal/driverapi"
	"github.com/AnqorDX/vdb-core/internal/points"
)

// compile-time check: Impl is reachable via the dot import.
var _ *Impl

// ---------------------------------------------------------------------------
// TransactionBegun
// ---------------------------------------------------------------------------

func TestTransactionBegun_InvokesPipeline(t *testing.T) {
	impl, pipe, _, _, _ := newTestImpl(t)

	var called bool
	pipe.DeclarePipeline(points.PipelineTransactionBegin, []string{points.PointTransactionBeginAuthorize})
	pipe.MustAttach(points.PointTransactionBeginAuthorize, 10, func(ctx any, p any) (any, any, error) {
		called = true
		return ctx, p, nil
	})

	if err := impl.TransactionBegun(1, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected pipeline handler to be called, but it was not")
	}
}

func TestTransactionBegun_HandlerError_IsReturned(t *testing.T) {
	impl, pipe, _, _, _ := newTestImpl(t)

	pipe.DeclarePipeline(points.PipelineTransactionBegin, []string{points.PointTransactionBeginAuthorize})
	pipe.MustAttach(points.PointTransactionBeginAuthorize, 10, func(ctx any, p any) (any, any, error) {
		return ctx, p, errors.New("injected transaction begin error")
	})

	if err := impl.TransactionBegun(1, false); err == nil {
		t.Fatal("expected non-nil error from handler, got nil")
	}
}

// ---------------------------------------------------------------------------
// TransactionCommitted
// ---------------------------------------------------------------------------

func TestTransactionCommitted_InvokesPipeline(t *testing.T) {
	impl, pipe, _, _, _ := newTestImpl(t)

	var called bool
	pipe.DeclarePipeline(points.PipelineTransactionCommit, []string{points.PointTransactionCommitApply})
	pipe.MustAttach(points.PointTransactionCommitApply, 10, func(ctx any, p any) (any, any, error) {
		called = true
		return ctx, p, nil
	})

	if err := impl.TransactionCommitted(1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected pipeline handler to be called, but it was not")
	}
}

// ---------------------------------------------------------------------------
// TransactionRolledBack
// ---------------------------------------------------------------------------

// TestTransactionRolledBack_InvokesPipeline verifies that TransactionRolledBack
// calls pipe.Process synchronously. Errors are logged and not returned, but the
// handler is still invoked.
func TestTransactionRolledBack_InvokesPipeline(t *testing.T) {
	impl, pipe, _, _, _ := newTestImpl(t)

	var called bool
	pipe.DeclarePipeline(points.PipelineTransactionRollback, []string{points.PointTransactionRollbackApply})
	pipe.MustAttach(points.PointTransactionRollbackApply, 10, func(ctx any, p any) (any, any, error) {
		called = true
		return ctx, p, nil
	})

	impl.TransactionRolledBack(1, "")

	if !called {
		t.Fatal("expected pipeline handler to be called, but it was not")
	}
}
