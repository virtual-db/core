package delta_test

import (
	"testing"

	. "github.com/AnqorDX/vdb-core/internal/delta"
)

// ---------------------------------------------------------------------------
// Merge — COMMIT replay semantics
// ---------------------------------------------------------------------------

// TestMerge_Inserts verifies that net-new rows in the staging delta are
// promoted into the live delta on COMMIT.
func TestMerge_Inserts(t *testing.T) {
	live := New()
	tx := New()

	rec := map[string]any{"id": 1, "name": "alice"}
	if err := tx.ApplyInsert("users", rec); err != nil {
		t.Fatalf("ApplyInsert: %v", err)
	}

	live.Merge(tx)

	state, err := live.TableState("users")
	if err != nil {
		t.Fatalf("TableState: %v", err)
	}
	if got := len(state.Inserts); got != 1 {
		t.Errorf("expected 1 insert after Merge, got %d", got)
	}
	key := RecordKey(rec)
	if _, ok := state.Inserts[key]; !ok {
		t.Errorf("merged record key %q not found in live delta", key)
	}
}

// TestMerge_Updates verifies that source-row overlays in the staging delta are
// promoted into the live delta on COMMIT.
func TestMerge_Updates(t *testing.T) {
	live := New()
	tx := New()

	old := map[string]any{"id": 1, "status": "active"}
	new := map[string]any{"id": 1, "status": "suspended"}
	if err := tx.ApplyUpdate("users", old, new); err != nil {
		t.Fatalf("ApplyUpdate: %v", err)
	}

	live.Merge(tx)

	state, err := live.TableState("users")
	if err != nil {
		t.Fatalf("TableState: %v", err)
	}
	if got := len(state.Updates); got != 1 {
		t.Errorf("expected 1 update after Merge, got %d", got)
	}
}

// TestMerge_Tombstones verifies that tombstones in the staging delta are
// promoted into the live delta on COMMIT, and that any prior update overlay
// for the same row is removed (delete supersedes update).
func TestMerge_Tombstones(t *testing.T) {
	live := New()
	tx := New()

	rec := map[string]any{"id": 1, "name": "bob"}
	if err := tx.ApplyDelete("users", rec); err != nil {
		t.Fatalf("ApplyDelete: %v", err)
	}

	live.Merge(tx)

	state, err := live.TableState("users")
	if err != nil {
		t.Fatalf("TableState: %v", err)
	}
	if got := len(state.Tombstones); got != 1 {
		t.Errorf("expected 1 tombstone after Merge, got %d", got)
	}
}

// TestMerge_TombstoneSupersedePriorUpdate verifies that when the staging delta
// contains a tombstone for a row that the live delta previously had an update
// overlay for, the tombstone wins and the update is removed.
//
// Both the live update overlay and the TxDelta tombstone must use the same
// stable key — the RecordKey of the original source row — for the supersede
// to work. The write handlers always supply the original source record when
// calling ApplyDelete, so in production this invariant is upheld.
func TestMerge_TombstoneSupersedePriorUpdate(t *testing.T) {
	live := New()

	// Live delta has an update overlay for a source row.
	// ApplyUpdate stores the overlay keyed by RecordKey(src) as the stable key.
	src := map[string]any{"id": 1, "status": "active"}
	overlay := map[string]any{"id": 1, "status": "suspended"}
	if err := live.ApplyUpdate("users", src, overlay); err != nil {
		t.Fatalf("ApplyUpdate into live: %v", err)
	}

	// Transaction deletes the same source row, using the original source record
	// as the key (as the write handler would supply from the merged read).
	// TxDelta has no currentToStable inheritance from live, so the tombstone
	// stable key must match what live stored: RecordKey(src).
	tx := New()
	if err := tx.ApplyDelete("users", src); err != nil {
		t.Fatalf("ApplyDelete into tx: %v", err)
	}

	live.Merge(tx)

	state, err := live.TableState("users")
	if err != nil {
		t.Fatalf("TableState: %v", err)
	}
	if got := len(state.Updates); got != 0 {
		t.Errorf("expected update to be removed after tombstone merge, got %d updates", got)
	}
	if got := len(state.Tombstones); got != 1 {
		t.Errorf("expected 1 tombstone after merge, got %d", got)
	}
}

// TestMerge_LastWriteWins verifies that when the staging delta has an update
// overlay for the same stable key as the live delta, the staging delta's
// version wins after Merge.
//
// Note: last-write-wins is expressed at the stable-key level. Two ApplyInsert
// calls with the same PK but different field values produce different
// RecordKeys and coexist in the delta — the delta has no PK-level
// deduplication for inserts, only for source-row overlays (updates). This
// test therefore exercises last-write-wins via ApplyUpdate, which is the path
// where stable-key identity is preserved across both the live delta and the
// staging delta.
func TestMerge_LastWriteWins(t *testing.T) {
	live := New()

	// Live delta has a committed update overlay: source row → "suspended".
	src := map[string]any{"id": 1, "status": "active"}
	v1 := map[string]any{"id": 1, "status": "suspended"}
	if err := live.ApplyUpdate("users", src, v1); err != nil {
		t.Fatalf("ApplyUpdate into live: %v", err)
	}

	// Transaction updates the same source row to "banned".
	// Because TxDelta starts empty, ApplyUpdate uses RecordKey(src) as the
	// stable key — the same stable key the live delta used — so after Merge
	// the TxDelta's version overwrites live's.
	tx := New()
	v2 := map[string]any{"id": 1, "status": "banned"}
	if err := tx.ApplyUpdate("users", src, v2); err != nil {
		t.Fatalf("ApplyUpdate into tx: %v", err)
	}

	live.Merge(tx)

	state, err := live.TableState("users")
	if err != nil {
		t.Fatalf("TableState: %v", err)
	}
	if got := len(state.Updates); got != 1 {
		t.Errorf("expected exactly 1 update after last-write-wins merge, got %d", got)
	}
	stableKey := RecordKey(src)
	rec, ok := state.Updates[stableKey]
	if !ok {
		t.Fatalf("expected update at stable key %q not found after merge", stableKey)
	}
	if got := rec["status"]; got != "banned" {
		t.Errorf("expected status 'banned' to win after merge, got %q", got)
	}
}

// TestMerge_EmptyStaging verifies that Merge is a no-op when the staging delta
// is empty (e.g. COMMIT issued immediately after BEGIN with no writes).
func TestMerge_EmptyStaging(t *testing.T) {
	live := New()
	if err := live.ApplyInsert("users", map[string]any{"id": 1, "name": "alice"}); err != nil {
		t.Fatalf("ApplyInsert: %v", err)
	}

	tx := New() // empty — no writes during the transaction

	live.Merge(tx)

	state, err := live.TableState("users")
	if err != nil {
		t.Fatalf("TableState: %v", err)
	}
	if got := len(state.Inserts); got != 1 {
		t.Errorf("expected live delta unchanged after empty Merge, got %d inserts", got)
	}
}

// TestMerge_MultipleTables verifies that Merge correctly handles a staging
// delta that touched multiple tables.
func TestMerge_MultipleTables(t *testing.T) {
	live := New()
	tx := New()

	if err := tx.ApplyInsert("users", map[string]any{"id": 1, "name": "alice"}); err != nil {
		t.Fatalf("ApplyInsert(users): %v", err)
	}
	if err := tx.ApplyInsert("orders", map[string]any{"id": 10, "user_id": 1}); err != nil {
		t.Fatalf("ApplyInsert(orders): %v", err)
	}

	live.Merge(tx)

	usersState, err := live.TableState("users")
	if err != nil {
		t.Fatalf("TableState(users): %v", err)
	}
	if got := len(usersState.Inserts); got != 1 {
		t.Errorf("users: expected 1 insert after Merge, got %d", got)
	}

	ordersState, err := live.TableState("orders")
	if err != nil {
		t.Fatalf("TableState(orders): %v", err)
	}
	if got := len(ordersState.Inserts); got != 1 {
		t.Errorf("orders: expected 1 insert after Merge, got %d", got)
	}
}

// TestMerge_DoesNotMutateSrc verifies that the staging delta is not modified
// by the Merge call. This is important so that the caller can safely discard
// the staging delta after merging.
func TestMerge_DoesNotMutateSrc(t *testing.T) {
	live := New()
	tx := New()

	rec := map[string]any{"id": 1, "name": "alice"}
	if err := tx.ApplyInsert("users", rec); err != nil {
		t.Fatalf("ApplyInsert: %v", err)
	}

	live.Merge(tx)

	// tx must still contain alice after the merge.
	txState, err := tx.TableState("users")
	if err != nil {
		t.Fatalf("TableState on tx after Merge: %v", err)
	}
	if got := len(txState.Inserts); got != 1 {
		t.Errorf("expected staging delta to be unmodified after Merge, got %d inserts", got)
	}
}

// TestMerge_ConcurrentConnWritesSurvive verifies the core correctness property:
// writes committed by connection B must not be affected when connection A
// commits its own transaction. This is the parallel-suite safety guarantee.
func TestMerge_ConcurrentConnWritesSurvive(t *testing.T) {
	live := New()

	// Conn B writes directly to the live delta (no transaction open).
	if err := live.ApplyInsert("sel_001_users", map[string]any{"id": 1, "name": "bob"}); err != nil {
		t.Fatalf("ApplyInsert connB: %v", err)
	}

	// Conn A opens a transaction and writes to its private staging delta.
	txA := New()
	if err := txA.ApplyInsert("txn_001_users", map[string]any{"id": 1, "name": "alice"}); err != nil {
		t.Fatalf("ApplyInsert connA tx: %v", err)
	}

	// Conn A commits: merge txA into live.
	live.Merge(txA)

	// B's write must still be in the live delta.
	bState, err := live.TableState("sel_001_users")
	if err != nil {
		t.Fatalf("TableState(sel_001_users): %v", err)
	}
	if got := len(bState.Inserts); got != 1 {
		t.Errorf("connB's write must survive connA's commit: expected 1 insert, got %d", got)
	}

	// A's write must also be in the live delta.
	aState, err := live.TableState("txn_001_users")
	if err != nil {
		t.Fatalf("TableState(txn_001_users): %v", err)
	}
	if got := len(aState.Inserts); got != 1 {
		t.Errorf("connA's committed write must be in live delta: expected 1 insert, got %d", got)
	}
}
