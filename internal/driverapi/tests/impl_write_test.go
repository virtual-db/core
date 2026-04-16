package driverapi_test

import (
	"errors"
	"testing"

	. "github.com/AnqorDX/vdb-core/internal/driverapi"
	"github.com/AnqorDX/vdb-core/internal/payloads"
	"github.com/AnqorDX/vdb-core/internal/points"
)

// compile-time check: Impl is reachable via the dot import.
var _ *Impl

// ---------------------------------------------------------------------------
// RecordInserted
// ---------------------------------------------------------------------------

func TestRecordInserted_InvokesPipeline(t *testing.T) {
	impl, pipe, _, _, _ := newTestImpl(t)

	pipe.DeclarePipeline(points.PipelineWriteInsert, []string{points.PointWriteInsertApply})
	pipe.MustAttach(points.PointWriteInsertApply, 10, func(ctx any, p any) (any, any, error) {
		out := payloads.WriteInsertPayload{Record: map[string]any{"id": 1}}
		return ctx, out, nil
	})

	got, err := impl.RecordInserted(1, "t", map[string]any{"id": 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil record, got nil")
	}
	if got["id"] != 1 {
		t.Fatalf("expected record id 1, got %v", got["id"])
	}
}

func TestRecordInserted_HandlerError_IsReturned(t *testing.T) {
	impl, pipe, _, _, _ := newTestImpl(t)

	pipe.DeclarePipeline(points.PipelineWriteInsert, []string{points.PointWriteInsertApply})
	pipe.MustAttach(points.PointWriteInsertApply, 10, func(ctx any, p any) (any, any, error) {
		return ctx, p, errors.New("injected insert error")
	})

	got, err := impl.RecordInserted(1, "t", map[string]any{"id": 1})
	if err == nil {
		t.Fatal("expected non-nil error from handler, got nil")
	}
	if got != nil {
		t.Fatalf("expected nil record on error, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// RecordUpdated
// ---------------------------------------------------------------------------

func TestRecordUpdated_InvokesPipeline(t *testing.T) {
	impl, pipe, _, _, _ := newTestImpl(t)

	pipe.DeclarePipeline(points.PipelineWriteUpdate, []string{points.PointWriteUpdateApply})
	pipe.MustAttach(points.PointWriteUpdateApply, 10, func(ctx any, p any) (any, any, error) {
		out := payloads.WriteUpdatePayload{NewRecord: map[string]any{"id": 1, "name": "b"}}
		return ctx, out, nil
	})

	old := map[string]any{"id": 1, "name": "a"}
	new := map[string]any{"id": 1, "name": "b"}

	got, err := impl.RecordUpdated(1, "t", old, new)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil record, got nil")
	}
	if got["name"] != "b" {
		t.Fatalf("expected name %q, got %v", "b", got["name"])
	}
}

func TestRecordUpdated_HandlerError_IsReturned(t *testing.T) {
	impl, pipe, _, _, _ := newTestImpl(t)

	pipe.DeclarePipeline(points.PipelineWriteUpdate, []string{points.PointWriteUpdateApply})
	pipe.MustAttach(points.PointWriteUpdateApply, 10, func(ctx any, p any) (any, any, error) {
		return ctx, p, errors.New("injected update error")
	})

	got, err := impl.RecordUpdated(1, "t", map[string]any{"id": 1}, map[string]any{"id": 1})
	if err == nil {
		t.Fatal("expected non-nil error from handler, got nil")
	}
	if got != nil {
		t.Fatalf("expected nil record on error, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// RecordDeleted
// ---------------------------------------------------------------------------

func TestRecordDeleted_InvokesPipeline(t *testing.T) {
	impl, pipe, _, _, _ := newTestImpl(t)

	var called bool
	pipe.DeclarePipeline(points.PipelineWriteDelete, []string{points.PointWriteDeleteApply})
	pipe.MustAttach(points.PointWriteDeleteApply, 10, func(ctx any, p any) (any, any, error) {
		called = true
		return ctx, p, nil
	})

	rec := map[string]any{"id": 1}
	if err := impl.RecordDeleted(1, "t", rec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected pipeline handler to be called, but it was not")
	}
}
