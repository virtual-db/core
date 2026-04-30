package write_test

import (
	"testing"

	"github.com/virtual-db/core/internal/connection"
	"github.com/virtual-db/core/internal/delta"
	"github.com/virtual-db/core/internal/framework"
	"github.com/virtual-db/core/internal/payloads"
	"github.com/virtual-db/core/internal/points"
	"github.com/virtual-db/core/internal/schema"
	. "github.com/virtual-db/core/internal/write"
)

func newWritePipe(t *testing.T) (*framework.Pipeline, *schema.Cache, *delta.Delta) {
	t.Helper()
	var global framework.GlobalContext
	pipe := framework.NewPipeline(&global)
	bus := framework.NewBus(&global)
	global = framework.SealContext(framework.NewGlobalContextBuilder(), bus, pipe)

	pipe.DeclarePipeline(points.PipelineWriteInsert, []string{
		points.PointWriteInsertBuildContext, points.PointWriteInsertApply, points.PointWriteInsertEmit,
	})
	pipe.DeclarePipeline(points.PipelineWriteUpdate, []string{
		points.PointWriteUpdateBuildContext, points.PointWriteUpdateApply, points.PointWriteUpdateEmit,
	})
	pipe.DeclarePipeline(points.PipelineWriteDelete, []string{
		points.PointWriteDeleteBuildContext, points.PointWriteDeleteApply, points.PointWriteDeleteEmit,
	})
	pipe.DeclarePipeline(points.PipelineRecordsSource, []string{
		points.PointRecordsSourceBuildContext, points.PointRecordsSourceTransform, points.PointRecordsSourceEmit,
	})
	pipe.DeclarePipeline(points.PipelineRecordsMerged, []string{
		points.PointRecordsMergedBuildContext, points.PointRecordsMergedTransform, points.PointRecordsMergedEmit,
	})
	pipe.DeclarePipeline(points.PipelineWriteTruncate, []string{
		points.PointWriteTruncateBuildContext, points.PointWriteTruncateApply, points.PointWriteTruncateEmit,
	})

	sc := schema.NewCache()
	d := delta.New()
	conns := connection.NewState()
	h := New(sc, d, conns)
	if err := h.Register(pipe); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return pipe, sc, d
}

func TestHandlers_WriteInsert_AddsToDelta(t *testing.T) {
	pipe, sc, d := newWritePipe(t)

	sc.Load("users", []string{"id", "name"}, "id")

	_, err := pipe.Process(points.PipelineWriteInsert, payloads.WriteInsertPayload{
		Table:  "users",
		Record: map[string]any{"id": 1, "name": "alice"},
	})
	if err != nil {
		t.Fatalf("Process(WriteInsert): %v", err)
	}

	state, err := d.TableState("users")
	if err != nil {
		t.Fatalf("TableState: %v", err)
	}
	if got := len(state.Inserts); got != 1 {
		t.Errorf("expected 1 insert in delta, got %d", got)
	}
}

func TestHandlers_WriteUpdate_AddsToDelta(t *testing.T) {
	pipe, sc, d := newWritePipe(t)

	sc.Load("users", []string{"id", "name"}, "id")

	_, err := pipe.Process(points.PipelineWriteUpdate, payloads.WriteUpdatePayload{
		Table:     "users",
		OldRecord: map[string]any{"id": 1, "name": "alice"},
		NewRecord: map[string]any{"id": 1, "name": "alicia"},
	})
	if err != nil {
		t.Fatalf("Process(WriteUpdate): %v", err)
	}

	state, err := d.TableState("users")
	if err != nil {
		t.Fatalf("TableState: %v", err)
	}
	if got := len(state.Updates); got != 1 {
		t.Errorf("expected 1 update in delta, got %d", got)
	}
}

func TestHandlers_WriteDelete_AddsTombstone(t *testing.T) {
	pipe, sc, d := newWritePipe(t)

	sc.Load("users", []string{"id", "name"}, "id")

	_, err := pipe.Process(points.PipelineWriteDelete, payloads.WriteDeletePayload{
		Table:  "users",
		Record: map[string]any{"id": 1, "name": "alice"},
	})
	if err != nil {
		t.Fatalf("Process(WriteDelete): %v", err)
	}

	state, err := d.TableState("users")
	if err != nil {
		t.Fatalf("TableState: %v", err)
	}
	if got := len(state.Tombstones); got != 1 {
		t.Errorf("expected 1 tombstone in delta, got %d", got)
	}
}

func TestHandlers_RecordsSource_AppliesOverlay(t *testing.T) {
	pipe, sc, d := newWritePipe(t)

	sc.Load("items", []string{"id"}, "id")

	if err := d.ApplyInsert("items", map[string]any{"id": 99}); err != nil {
		t.Fatalf("ApplyInsert: %v", err)
	}

	result, err := pipe.Process(points.PipelineRecordsSource, payloads.RecordsSourcePayload{
		Table:   "items",
		Records: []map[string]any{},
	})
	if err != nil {
		t.Fatalf("Process(RecordsSource): %v", err)
	}

	p, ok := result.(payloads.RecordsSourcePayload)
	if !ok {
		t.Fatalf("expected result to be RecordsSourcePayload, got %T", result)
	}
	if got := len(p.Records); got != 1 {
		t.Fatalf("expected 1 record in overlay result, got %d", got)
	}
	if got := p.Records[0]["id"]; got != 99 {
		t.Errorf("expected inserted record id=99, got %v", got)
	}
}

func TestHandlers_Register_ReturnsNilError(t *testing.T) {
	var global framework.GlobalContext
	pipe := framework.NewPipeline(&global)
	bus := framework.NewBus(&global)
	global = framework.SealContext(framework.NewGlobalContextBuilder(), bus, pipe)

	pipe.DeclarePipeline(points.PipelineWriteInsert, []string{
		points.PointWriteInsertBuildContext, points.PointWriteInsertApply, points.PointWriteInsertEmit,
	})
	pipe.DeclarePipeline(points.PipelineWriteUpdate, []string{
		points.PointWriteUpdateBuildContext, points.PointWriteUpdateApply, points.PointWriteUpdateEmit,
	})
	pipe.DeclarePipeline(points.PipelineWriteDelete, []string{
		points.PointWriteDeleteBuildContext, points.PointWriteDeleteApply, points.PointWriteDeleteEmit,
	})
	pipe.DeclarePipeline(points.PipelineRecordsSource, []string{
		points.PointRecordsSourceBuildContext, points.PointRecordsSourceTransform, points.PointRecordsSourceEmit,
	})
	pipe.DeclarePipeline(points.PipelineRecordsMerged, []string{
		points.PointRecordsMergedBuildContext, points.PointRecordsMergedTransform, points.PointRecordsMergedEmit,
	})
	pipe.DeclarePipeline(points.PipelineWriteTruncate, []string{
		points.PointWriteTruncateBuildContext, points.PointWriteTruncateApply, points.PointWriteTruncateEmit,
	})

	sc := schema.NewCache()
	d := delta.New()
	conns := connection.NewState()
	h := New(sc, d, conns)
	if err := h.Register(pipe); err != nil {
		t.Fatalf("expected Register to return nil error, got: %v", err)
	}
}
