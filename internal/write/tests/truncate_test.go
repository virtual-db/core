package write_test

import (
	"testing"

	"github.com/virtual-db/core/internal/delta"
	"github.com/virtual-db/core/internal/payloads"
	"github.com/virtual-db/core/internal/points"
	"github.com/virtual-db/core/internal/schema"
	. "github.com/virtual-db/core/internal/write"
)

// ---------------------------------------------------------------------------
// WriteTruncate pipeline handler
// ---------------------------------------------------------------------------

func TestHandlers_WriteTruncate_MarksDeltaTruncated(t *testing.T) {
	pipe, sc, d := newWritePipe(t)
	sc.Load("orders", []string{"id", "status"}, "id")

	// Pre-insert a row so we can confirm it is cleared.
	_ = d.ApplyInsert("orders", map[string]any{"id": 1, "status": "active"})

	_, err := pipe.Process(points.PipelineWriteTruncate, payloads.WriteTruncatePayload{
		Table: "orders",
	})
	if err != nil {
		t.Fatalf("Process(WriteTruncate): %v", err)
	}

	state, err := d.TableState("orders")
	if err != nil {
		t.Fatalf("TableState: %v", err)
	}
	if !state.Truncated {
		t.Error("expected delta to be marked truncated after WriteTruncate pipeline")
	}
	if len(state.Inserts) != 0 {
		t.Errorf("expected pre-truncate insert to be cleared, got %d inserts", len(state.Inserts))
	}
}

func TestHandlers_WriteTruncate_SubsequentInsert_IsVisible(t *testing.T) {
	pipe, sc, d := newWritePipe(t)
	sc.Load("orders", []string{"id"}, "id")

	_, err := pipe.Process(points.PipelineWriteTruncate, payloads.WriteTruncatePayload{Table: "orders"})
	if err != nil {
		t.Fatalf("Process(WriteTruncate): %v", err)
	}
	_, err = pipe.Process(points.PipelineWriteInsert, payloads.WriteInsertPayload{
		Table:  "orders",
		Record: map[string]any{"id": 99},
	})
	if err != nil {
		t.Fatalf("Process(WriteInsert after truncate): %v", err)
	}

	state, _ := d.TableState("orders")
	if len(state.Inserts) != 1 {
		t.Errorf("expected 1 post-truncate insert, got %d", len(state.Inserts))
	}
}

// ---------------------------------------------------------------------------
// Overlay — truncated table behaviour
// ---------------------------------------------------------------------------

func TestOverlay_TruncatedTable_SourceRowsSuppressed(t *testing.T) {
	d := delta.New()
	sc := schema.NewCache()
	sc.Load("orders", []string{"id", "status"}, "id")

	d.ApplyTruncate("orders")

	source := []map[string]any{
		{"id": 1, "status": "active"},
		{"id": 2, "status": "pending"},
	}

	result, err := Overlay(d, sc, "orders", source)
	if err != nil {
		t.Fatalf("Overlay: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 rows from truncated table, got %d", len(result))
	}
}

func TestOverlay_TruncatedTable_PostTruncateInsertsVisible(t *testing.T) {
	d := delta.New()
	sc := schema.NewCache()
	sc.Load("orders", []string{"id", "status"}, "id")

	d.ApplyTruncate("orders")
	_ = d.ApplyInsert("orders", map[string]any{"id": 99, "status": "new"})

	source := []map[string]any{
		{"id": 1, "status": "active"},
	}

	result, err := Overlay(d, sc, "orders", source)
	if err != nil {
		t.Fatalf("Overlay: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 post-truncate insert, got %d", len(result))
	}
	if result[0]["id"] != 99 {
		t.Errorf("expected post-truncate row id=99, got %v", result[0]["id"])
	}
}

func TestOverlay_TruncatedTable_ExistingNonTruncatedTableUnaffected(t *testing.T) {
	d := delta.New()
	sc := schema.NewCache()
	sc.Load("orders", []string{"id"}, "id")
	sc.Load("customers", []string{"id"}, "id")

	_ = d.ApplyInsert("customers", map[string]any{"id": 10})
	d.ApplyTruncate("orders")

	result, err := Overlay(d, sc, "customers", []map[string]any{{"id": 5}})
	if err != nil {
		t.Fatalf("Overlay(customers): %v", err)
	}
	// customers delta insert (id=10) + source row (id=5) = 2 rows
	if len(result) != 2 {
		t.Errorf("expected 2 customer rows, got %d", len(result))
	}
}
