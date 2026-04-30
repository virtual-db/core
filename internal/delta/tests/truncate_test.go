package delta_test

import (
	"testing"

	. "github.com/virtual-db/core/internal/delta"
)

// ---------------------------------------------------------------------------
// ApplyTruncate — mutation store behaviour
// ---------------------------------------------------------------------------

func TestApplyTruncate_SetsTruncatedFlag(t *testing.T) {
	d := New()
	d.ApplyTruncate("orders")

	state, err := d.TableState("orders")
	if err != nil {
		t.Fatalf("TableState: %v", err)
	}
	if !state.Truncated {
		t.Error("expected Truncated=true after ApplyTruncate, got false")
	}
}

func TestApplyTruncate_ClearsExistingInserts(t *testing.T) {
	d := New()
	_ = d.ApplyInsert("orders", map[string]any{"id": 1})
	_ = d.ApplyInsert("orders", map[string]any{"id": 2})

	d.ApplyTruncate("orders")

	state, _ := d.TableState("orders")
	if len(state.Inserts) != 0 {
		t.Errorf("expected 0 inserts after truncate, got %d", len(state.Inserts))
	}
}

func TestApplyTruncate_ClearsExistingUpdatesAndTombstones(t *testing.T) {
	d := New()
	src := map[string]any{"id": 1, "name": "alice"}
	_ = d.ApplyUpdate("orders", src, map[string]any{"id": 1, "name": "alice-v2"})
	_ = d.ApplyDelete("orders", map[string]any{"id": 2, "name": "bob"})

	d.ApplyTruncate("orders")

	state, _ := d.TableState("orders")
	if len(state.Updates) != 0 {
		t.Errorf("expected 0 updates after truncate, got %d", len(state.Updates))
	}
	if len(state.Tombstones) != 0 {
		t.Errorf("expected 0 tombstones after truncate, got %d", len(state.Tombstones))
	}
}

func TestApplyTruncate_ThenInsert_InsertsAreVisible(t *testing.T) {
	d := New()
	_ = d.ApplyInsert("orders", map[string]any{"id": 1})

	d.ApplyTruncate("orders")

	// Row inserted AFTER the truncate must be visible.
	_ = d.ApplyInsert("orders", map[string]any{"id": 99})

	state, _ := d.TableState("orders")
	if !state.Truncated {
		t.Error("expected Truncated=true after truncate+insert")
	}
	if len(state.Inserts) != 1 {
		t.Errorf("expected 1 post-truncate insert, got %d", len(state.Inserts))
	}
	if _, ok := state.Inserts[RecordKey(map[string]any{"id": 99})]; !ok {
		t.Error("expected post-truncate insert with id=99 to be present")
	}
}

func TestApplyTruncate_DoesNotAffectOtherTables(t *testing.T) {
	d := New()
	_ = d.ApplyInsert("orders", map[string]any{"id": 1})
	_ = d.ApplyInsert("customers", map[string]any{"id": 10})

	d.ApplyTruncate("orders")

	custState, _ := d.TableState("customers")
	if custState.Truncated {
		t.Error("ApplyTruncate(orders) must not mark customers as truncated")
	}
	if len(custState.Inserts) != 1 {
		t.Errorf("customers inserts: expected 1, got %d", len(custState.Inserts))
	}
}

// ---------------------------------------------------------------------------
// Merge — truncated TxDelta promotes correctly into live delta
// ---------------------------------------------------------------------------

func TestMerge_TruncatedTxDelta_ResetsLiveTableState(t *testing.T) {
	live := New()
	// Pre-populate the live delta with a source-row overlay and a tombstone.
	_ = live.ApplyUpdate("orders", map[string]any{"id": 1}, map[string]any{"id": 1, "status": "shipped"})
	_ = live.ApplyDelete("orders", map[string]any{"id": 2, "status": "pending"})

	// Transaction truncates the table, then inserts one new row.
	tx := New()
	tx.ApplyTruncate("orders")
	_ = tx.ApplyInsert("orders", map[string]any{"id": 99, "status": "new"})

	live.Merge(tx)

	state, _ := live.TableState("orders")
	if !state.Truncated {
		t.Error("expected live delta to be truncated after merging truncated TxDelta")
	}
	if len(state.Updates) != 0 {
		t.Errorf("expected 0 updates after merge of truncated tx, got %d", len(state.Updates))
	}
	if len(state.Tombstones) != 0 {
		t.Errorf("expected 0 tombstones after merge of truncated tx, got %d", len(state.Tombstones))
	}
	if len(state.Inserts) != 1 {
		t.Errorf("expected 1 post-truncate insert after merge, got %d", len(state.Inserts))
	}
}

func TestMerge_NonTruncatedTxDelta_DoesNotClearLiveState(t *testing.T) {
	live := New()
	_ = live.ApplyInsert("orders", map[string]any{"id": 1})

	// Transaction does not truncate — normal insert.
	tx := New()
	_ = tx.ApplyInsert("orders", map[string]any{"id": 2})

	live.Merge(tx)

	state, _ := live.TableState("orders")
	if state.Truncated {
		t.Error("expected Truncated=false after merging non-truncated tx")
	}
	if len(state.Inserts) != 2 {
		t.Errorf("expected 2 inserts after normal merge, got %d", len(state.Inserts))
	}
}
