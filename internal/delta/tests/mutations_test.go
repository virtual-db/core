package delta_test

import (
	"fmt"
	"sync"
	"testing"

	. "github.com/AnqorDX/vdb-core/internal/delta"
)

func mapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func TestApplyInsert_AddsRecord(t *testing.T) {
	d := New()
	rec := map[string]any{"id": 1, "name": "alice"}
	if err := d.ApplyInsert("users", rec); err != nil {
		t.Fatalf("ApplyInsert error: %v", err)
	}

	state, err := d.TableState("users")
	if err != nil {
		t.Fatalf("TableState error: %v", err)
	}

	if len(state.Inserts) != 1 {
		t.Errorf("expected 1 insert, got %d", len(state.Inserts))
	}
	if len(state.Updates) != 0 {
		t.Errorf("expected 0 updates, got %d", len(state.Updates))
	}
	if len(state.Tombstones) != 0 {
		t.Errorf("expected 0 tombstones, got %d", len(state.Tombstones))
	}

	key := RecordKey(rec)
	inserted, ok := state.Inserts[key]
	if !ok {
		t.Fatalf("expected key %q in Inserts, keys present: %v", key, mapKeys(state.Inserts))
	}
	if inserted["id"] != 1 {
		t.Errorf("expected id=1, got %v", inserted["id"])
	}
	if inserted["name"] != "alice" {
		t.Errorf("expected name=alice, got %v", inserted["name"])
	}
}

func TestApplyInsert_TwiceSameRecord_LastWriteWins(t *testing.T) {
	d := New()
	rec := map[string]any{"id": 1, "name": "alice"}
	_ = d.ApplyInsert("users", rec)
	_ = d.ApplyInsert("users", rec)

	state, err := d.TableState("users")
	if err != nil {
		t.Fatalf("TableState error: %v", err)
	}
	if len(state.Inserts) != 1 {
		t.Errorf("expected 1 insert after duplicate, got %d", len(state.Inserts))
	}
}

func TestApplyUpdate_OnNetNewInsert_ReKeysInInserts(t *testing.T) {
	d := New()
	v1 := map[string]any{"id": 1, "name": "alice"}
	v2 := map[string]any{"id": 1, "name": "alice-updated"}
	_ = d.ApplyInsert("users", v1)
	if err := d.ApplyUpdate("users", v1, v2); err != nil {
		t.Fatalf("ApplyUpdate error: %v", err)
	}

	state, err := d.TableState("users")
	if err != nil {
		t.Fatalf("TableState error: %v", err)
	}
	if len(state.Updates) != 0 {
		t.Errorf("expected 0 updates, got %d (keys: %v)", len(state.Updates), mapKeys(state.Updates))
	}
	if len(state.Inserts) != 1 {
		t.Errorf("expected 1 insert, got %d (keys: %v)", len(state.Inserts), mapKeys(state.Inserts))
	}
	oldKey := RecordKey(v1)
	if _, present := state.Inserts[oldKey]; present {
		t.Errorf("old key %q should not be in Inserts after re-key", oldKey)
	}
	newKey := RecordKey(v2)
	inserted, ok := state.Inserts[newKey]
	if !ok {
		t.Fatalf("new key %q not found in Inserts", newKey)
	}
	if inserted["name"] != "alice-updated" {
		t.Errorf("expected name=alice-updated, got %v", inserted["name"])
	}
}

func TestApplyUpdate_OnSourceRow_CreatesUpdate(t *testing.T) {
	d := New()
	srcRow := map[string]any{"id": 2, "name": "bob"}
	newRow := map[string]any{"id": 2, "name": "bob-updated"}
	if err := d.ApplyUpdate("users", srcRow, newRow); err != nil {
		t.Fatalf("ApplyUpdate error: %v", err)
	}

	state, err := d.TableState("users")
	if err != nil {
		t.Fatalf("TableState error: %v", err)
	}
	if len(state.Inserts) != 0 {
		t.Errorf("expected 0 inserts, got %d", len(state.Inserts))
	}
	if len(state.Updates) != 1 {
		t.Errorf("expected 1 update, got %d", len(state.Updates))
	}
	stableKey := RecordKey(srcRow)
	updated, ok := state.Updates[stableKey]
	if !ok {
		t.Fatalf("stable key %q not found in Updates (keys: %v)", stableKey, mapKeys(state.Updates))
	}
	if updated["name"] != "bob-updated" {
		t.Errorf("expected name=bob-updated, got %v", updated["name"])
	}
}

func TestApplyUpdate_TwiceOnSameRow_ResolvesStableKey(t *testing.T) {
	d := New()
	v1 := map[string]any{"id": 3, "name": "carol"}
	v2 := map[string]any{"id": 3, "name": "carol-v2"}
	v3 := map[string]any{"id": 3, "name": "carol-v3"}
	_ = d.ApplyUpdate("users", v1, v2)
	if err := d.ApplyUpdate("users", v2, v3); err != nil {
		t.Fatalf("second ApplyUpdate error: %v", err)
	}

	state, err := d.TableState("users")
	if err != nil {
		t.Fatalf("TableState error: %v", err)
	}
	if len(state.Updates) != 1 {
		t.Errorf("expected 1 update, got %d (keys: %v)", len(state.Updates), mapKeys(state.Updates))
	}
	stableKey := RecordKey(v1)
	updated, ok := state.Updates[stableKey]
	if !ok {
		t.Fatalf("stable key %q not found in Updates (keys: %v)", stableKey, mapKeys(state.Updates))
	}
	if updated["name"] != "carol-v3" {
		t.Errorf("expected name=carol-v3, got %v", updated["name"])
	}
}

func TestApplyDelete_OnNetNewInsert_RemovesFromInserts(t *testing.T) {
	d := New()
	rec := map[string]any{"id": 4, "name": "dave"}
	_ = d.ApplyInsert("users", rec)
	if err := d.ApplyDelete("users", rec); err != nil {
		t.Fatalf("ApplyDelete error: %v", err)
	}

	state, err := d.TableState("users")
	if err != nil {
		t.Fatalf("TableState error: %v", err)
	}
	if len(state.Inserts) != 0 {
		t.Errorf("expected 0 inserts, got %d", len(state.Inserts))
	}
	if len(state.Updates) != 0 {
		t.Errorf("expected 0 updates, got %d", len(state.Updates))
	}
	if len(state.Tombstones) != 0 {
		t.Errorf("expected 0 tombstones, got %d", len(state.Tombstones))
	}
}

func TestApplyDelete_OnSourceRow_CreatesTombstone(t *testing.T) {
	d := New()
	srcRow := map[string]any{"id": 5, "name": "eve"}
	if err := d.ApplyDelete("users", srcRow); err != nil {
		t.Fatalf("ApplyDelete error: %v", err)
	}

	state, err := d.TableState("users")
	if err != nil {
		t.Fatalf("TableState error: %v", err)
	}
	if len(state.Inserts) != 0 {
		t.Errorf("expected 0 inserts, got %d", len(state.Inserts))
	}
	if len(state.Updates) != 0 {
		t.Errorf("expected 0 updates, got %d", len(state.Updates))
	}
	if len(state.Tombstones) != 1 {
		t.Errorf("expected 1 tombstone, got %d", len(state.Tombstones))
	}
	stableKey := RecordKey(srcRow)
	if _, ok := state.Tombstones[stableKey]; !ok {
		t.Errorf("expected tombstone keyed by %q, keys present: %v", stableKey, mapKeys(state.Tombstones))
	}
}

func TestApplyDelete_OnUpdatedSourceRow_ResolvesStableKey(t *testing.T) {
	d := New()
	v1 := map[string]any{"id": 6, "name": "frank"}
	v2 := map[string]any{"id": 6, "name": "frank-updated"}
	_ = d.ApplyUpdate("users", v1, v2)
	if err := d.ApplyDelete("users", v2); err != nil {
		t.Fatalf("ApplyDelete error: %v", err)
	}

	state, err := d.TableState("users")
	if err != nil {
		t.Fatalf("TableState error: %v", err)
	}
	if len(state.Updates) != 0 {
		t.Errorf("expected 0 updates, got %d (keys: %v)", len(state.Updates), mapKeys(state.Updates))
	}
	if len(state.Tombstones) != 1 {
		t.Errorf("expected 1 tombstone, got %d", len(state.Tombstones))
	}
	stableKey := RecordKey(v1)
	if _, ok := state.Tombstones[stableKey]; !ok {
		t.Errorf("expected tombstone keyed by stable key %q, keys present: %v", stableKey, mapKeys(state.Tombstones))
	}
}

// TestApplyUpdateWithFallback_ChainedUpdate simulates the UPD-003 scenario:
// two consecutive UPDATE statements issued in autocommit mode (each wrapped in
// its own implicit BEGIN / COMMIT by GMS).
//
// After the first UPDATE commits, the live delta holds:
//
//	updates[key(active)]          = {status: "suspended"}
//	currentToStable[key(suspended)] = key(active)
//
// The second UPDATE is issued inside a freshly allocated TxDelta that knows
// nothing about the first UPDATE's mapping. Without the fallback, ApplyUpdate
// would use key(suspended) as the stable key, producing a spurious second entry
// in updates and leaving the key(active) entry pointing at "suspended" forever.
// With the fallback, the live delta's currentToStable is consulted and the
// correct stable key (key(active)) is used, so the committed Merge overwrites
// the first UPDATE's entry with "banned".
func TestApplyUpdateWithFallback_ChainedUpdate(t *testing.T) {
	// --- First implicit transaction ---
	src := map[string]any{"id": 1, "status": "active"}
	v1 := map[string]any{"id": 1, "status": "suspended"}
	v2 := map[string]any{"id": 1, "status": "banned"}

	live := New()
	tx1 := New()

	// First UPDATE goes into tx1 (the first implicit transaction's TxDelta).
	if err := tx1.ApplyUpdate("users", src, v1); err != nil {
		t.Fatalf("tx1.ApplyUpdate(active→suspended): %v", err)
	}

	// COMMIT: merge tx1 into live.
	live.Merge(tx1)

	// Verify live has the expected mapping after first COMMIT.
	liveState, err := live.TableState("users")
	if err != nil {
		t.Fatalf("live.TableState after first commit: %v", err)
	}
	srcKey := RecordKey(src)
	if got, ok := liveState.Updates[srcKey]; !ok {
		t.Fatalf("expected live.updates[%q] after first commit, keys: %v", srcKey, mapKeys(liveState.Updates))
	} else if got["status"] != "suspended" {
		t.Errorf("live.updates[%q]: got status=%v, want suspended", srcKey, got["status"])
	}

	// --- Second implicit transaction ---
	// GMS allocates a fresh TxDelta — it has no knowledge of tx1's mapping.
	tx2 := New()

	// The second UPDATE's "old" record is the overlay-merged row (suspended),
	// not the original source row (active). Without fallback, ApplyUpdate would
	// use key(suspended) as the stable key.
	if err := tx2.ApplyUpdateWithFallback("users", v1, v2, live); err != nil {
		t.Fatalf("tx2.ApplyUpdateWithFallback(suspended→banned): %v", err)
	}

	// Verify tx2 used the correct stable key (key(active)), not key(suspended).
	tx2State, err := tx2.TableState("users")
	if err != nil {
		t.Fatalf("tx2.TableState: %v", err)
	}
	if len(tx2State.Updates) != 1 {
		t.Errorf("expected exactly 1 update in tx2, got %d (keys: %v)", len(tx2State.Updates), mapKeys(tx2State.Updates))
	}
	if _, hasWrongKey := tx2State.Updates[RecordKey(v1)]; hasWrongKey {
		t.Error("tx2 used key(suspended) as stable key — fallback did not resolve to key(active)")
	}
	if got, ok := tx2State.Updates[srcKey]; !ok {
		t.Fatalf("tx2.updates[%q] not found; keys present: %v", srcKey, mapKeys(tx2State.Updates))
	} else if got["status"] != "banned" {
		t.Errorf("tx2.updates[%q]: got status=%v, want banned", srcKey, got["status"])
	}

	// COMMIT: merge tx2 into live.
	live.Merge(tx2)

	// After second COMMIT, live must show key(active) → banned (one entry only).
	finalState, err := live.TableState("users")
	if err != nil {
		t.Fatalf("live.TableState after second commit: %v", err)
	}
	if len(finalState.Updates) != 1 {
		t.Errorf("expected exactly 1 update in live after both commits, got %d (keys: %v)",
			len(finalState.Updates), mapKeys(finalState.Updates))
	}
	final, ok := finalState.Updates[srcKey]
	if !ok {
		t.Fatalf("live.updates[%q] not found after second commit; keys: %v", srcKey, mapKeys(finalState.Updates))
	}
	if final["status"] != "banned" {
		t.Errorf("final status: got %v, want banned", final["status"])
	}
}

func TestApplyUpdateWithFallback_NoFallbackNeeded_LocalMappingSuffices(t *testing.T) {
	// When both updates are in the same TxDelta (explicit transaction), the
	// fallback is not consulted — the local currentToStable mapping from the
	// first ApplyUpdate is used directly.
	src := map[string]any{"id": 1, "status": "active"}
	v1 := map[string]any{"id": 1, "status": "suspended"}
	v2 := map[string]any{"id": 1, "status": "banned"}

	live := New() // live delta is empty — fallback should not be needed
	tx := New()

	if err := tx.ApplyUpdate("users", src, v1); err != nil {
		t.Fatalf("ApplyUpdate(active→suspended): %v", err)
	}
	// Second update in the same TxDelta: local currentToStable has the mapping.
	if err := tx.ApplyUpdateWithFallback("users", v1, v2, live); err != nil {
		t.Fatalf("ApplyUpdateWithFallback(suspended→banned): %v", err)
	}

	state, err := tx.TableState("users")
	if err != nil {
		t.Fatalf("TableState: %v", err)
	}
	if len(state.Updates) != 1 {
		t.Errorf("expected 1 update, got %d (keys: %v)", len(state.Updates), mapKeys(state.Updates))
	}
	srcKey := RecordKey(src)
	got, ok := state.Updates[srcKey]
	if !ok {
		t.Fatalf("updates[%q] not found; keys: %v", srcKey, mapKeys(state.Updates))
	}
	if got["status"] != "banned" {
		t.Errorf("status: got %v, want banned", got["status"])
	}
}

func TestApplyInsert_Concurrent_NoRace(t *testing.T) {
	d := New()
	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			rec := map[string]any{"id": i, "name": fmt.Sprintf("user-%d", i)}
			if err := d.ApplyInsert("users", rec); err != nil {
				t.Errorf("goroutine %d: ApplyInsert error: %v", i, err)
			}
		}()
	}
	wg.Wait()

	recs, err := d.Records("users")
	if err != nil {
		t.Fatalf("Records error: %v", err)
	}
	if len(recs) != n {
		t.Errorf("expected %d records, got %d", n, len(recs))
	}
}
