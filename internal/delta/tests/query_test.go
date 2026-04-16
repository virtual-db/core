package delta_test

import (
	"testing"

	. "github.com/AnqorDX/vdb-core/internal/delta"
)

// ---------------------------------------------------------------------------
// RecordKey
// ---------------------------------------------------------------------------

func TestRecordKey_SingleField_Deterministic(t *testing.T) {
	rec := map[string]any{"id": 42}
	first := RecordKey(rec)
	second := RecordKey(rec)
	if first == "" {
		t.Fatal("expected non-empty key")
	}
	if first != second {
		t.Errorf("RecordKey not deterministic: %q vs %q", first, second)
	}
}

func TestRecordKey_MultiField_SortedByName(t *testing.T) {
	// Build two maps with the same logical content but populated in different
	// field order (Go map iteration is random, so this exercises sort stability).
	a := map[string]any{"id": 1, "name": "alice", "age": 30}
	b := map[string]any{"age": 30, "name": "alice", "id": 1}

	for i := 0; i < 10; i++ {
		ka := RecordKey(a)
		kb := RecordKey(b)
		if ka != kb {
			t.Fatalf("iteration %d: keys differ: %q vs %q", i, ka, kb)
		}
	}
}

func TestRecordKey_SameFieldsAndValues_SameKey(t *testing.T) {
	r1 := map[string]any{"x": 10, "y": "hello"}
	r2 := map[string]any{"x": 10, "y": "hello"}
	if RecordKey(r1) != RecordKey(r2) {
		t.Errorf("identical records produced different keys: %q vs %q", RecordKey(r1), RecordKey(r2))
	}
}

// ---------------------------------------------------------------------------
// Records
// ---------------------------------------------------------------------------

func TestRecords_EmptyDelta_ReturnsNil(t *testing.T) {
	d := New()
	recs, err := d.Records("nonexistent")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if recs != nil {
		t.Errorf("expected nil slice, got %v", recs)
	}
}

func TestRecords_ReturnsInsertsAndUpdates(t *testing.T) {
	d := New()

	// 1 net-new insert
	inserted := map[string]any{"id": 1, "name": "insert-row"}
	if err := d.ApplyInsert("items", inserted); err != nil {
		t.Fatalf("ApplyInsert: %v", err)
	}

	// 1 source-row update (no prior insert → goes to Updates)
	srcRow := map[string]any{"id": 2, "name": "src-row"}
	newRow := map[string]any{"id": 2, "name": "src-row-updated"}
	if err := d.ApplyUpdate("items", srcRow, newRow); err != nil {
		t.Fatalf("ApplyUpdate: %v", err)
	}

	recs, err := d.Records("items")
	if err != nil {
		t.Fatalf("Records error: %v", err)
	}
	if len(recs) != 2 {
		t.Errorf("expected 2 records (1 insert + 1 update), got %d", len(recs))
	}
}

func TestRecords_TombstonedNotReturned(t *testing.T) {
	d := New()

	// Delete a source row directly — no prior insert means it becomes a tombstone.
	srcRow := map[string]any{"order_id": 99, "status": "pending"}
	if err := d.ApplyDelete("orders", srcRow); err != nil {
		t.Fatalf("ApplyDelete: %v", err)
	}

	recs, err := d.Records("orders")
	if err != nil {
		t.Fatalf("Records error: %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("expected 0 records (tombstoned row must not appear), got %d", len(recs))
	}
}

// ---------------------------------------------------------------------------
// TableState
// ---------------------------------------------------------------------------

func TestTableState_UnknownTable_EmptyMaps(t *testing.T) {
	d := New()
	state, err := d.TableState("ghost")
	if err != nil {
		t.Fatalf("TableState error: %v", err)
	}
	if state.Inserts == nil {
		t.Error("Inserts map must be non-nil")
	}
	if state.Updates == nil {
		t.Error("Updates map must be non-nil")
	}
	if state.Tombstones == nil {
		t.Error("Tombstones map must be non-nil")
	}
	if len(state.Inserts) != 0 {
		t.Errorf("expected empty Inserts, got %d entries", len(state.Inserts))
	}
	if len(state.Updates) != 0 {
		t.Errorf("expected empty Updates, got %d entries", len(state.Updates))
	}
	if len(state.Tombstones) != 0 {
		t.Errorf("expected empty Tombstones, got %d entries", len(state.Tombstones))
	}
}

func TestTableState_CategorisesCorrectly(t *testing.T) {
	d := New()

	// 1 net-new insert
	insertedRec := map[string]any{"id": 10, "val": "new"}
	if err := d.ApplyInsert("things", insertedRec); err != nil {
		t.Fatalf("ApplyInsert: %v", err)
	}

	// 1 source-row update
	srcUpdate := map[string]any{"id": 20, "val": "original"}
	updatedRow := map[string]any{"id": 20, "val": "changed"}
	if err := d.ApplyUpdate("things", srcUpdate, updatedRow); err != nil {
		t.Fatalf("ApplyUpdate: %v", err)
	}

	// 1 source-row delete (tombstone)
	srcDelete := map[string]any{"id": 30, "val": "gone"}
	if err := d.ApplyDelete("things", srcDelete); err != nil {
		t.Fatalf("ApplyDelete: %v", err)
	}

	state, err := d.TableState("things")
	if err != nil {
		t.Fatalf("TableState: %v", err)
	}

	if len(state.Inserts) != 1 {
		t.Errorf("expected 1 insert, got %d", len(state.Inserts))
	}
	if len(state.Updates) != 1 {
		t.Errorf("expected 1 update, got %d", len(state.Updates))
	}
	if len(state.Tombstones) != 1 {
		t.Errorf("expected 1 tombstone, got %d", len(state.Tombstones))
	}

	// Spot-check keys
	insertKey := RecordKey(insertedRec)
	if _, ok := state.Inserts[insertKey]; !ok {
		t.Errorf("insert key %q not found in Inserts", insertKey)
	}

	updateKey := RecordKey(srcUpdate)
	if _, ok := state.Updates[updateKey]; !ok {
		t.Errorf("stable update key %q not found in Updates", updateKey)
	}

	tombKey := RecordKey(srcDelete)
	if _, ok := state.Tombstones[tombKey]; !ok {
		t.Errorf("tombstone key %q not found in Tombstones", tombKey)
	}
}

func TestTableState_DefensiveCopy(t *testing.T) {
	d := New()

	rec := map[string]any{"id": 1, "color": "blue"}
	if err := d.ApplyInsert("widgets", rec); err != nil {
		t.Fatalf("ApplyInsert: %v", err)
	}

	// First call — mutate the returned copy.
	state1, err := d.TableState("widgets")
	if err != nil {
		t.Fatalf("TableState (first): %v", err)
	}

	// Add a spurious key to the outer map.
	state1.Inserts["__injected__"] = map[string]any{"id": 999}

	// Mutate an inner record value.
	for _, inner := range state1.Inserts {
		inner["color"] = "MUTATED"
	}

	// Second call — delta must be unaffected.
	state2, err := d.TableState("widgets")
	if err != nil {
		t.Fatalf("TableState (second): %v", err)
	}

	if len(state2.Inserts) != 1 {
		t.Errorf("expected 1 insert after mutation of copy, got %d (injected key leaked into delta)", len(state2.Inserts))
	}

	key := RecordKey(rec)
	inner, ok := state2.Inserts[key]
	if !ok {
		t.Fatalf("original key %q missing from second TableState call", key)
	}
	if inner["color"] != "blue" {
		t.Errorf("inner value was mutated through the copy: expected %q, got %v", "blue", inner["color"])
	}
}
