// Package delta — snapshot.go
// Provides the Merge operation used by transaction COMMIT to promote a
// connection's private staging delta into the shared live delta.
//
// The old Snapshot / Restore API has been removed. The previous design took a
// full copy of the live delta at BEGIN and restored it wholesale on ROLLBACK,
// which incorrectly erased concurrent writes from other connections. The
// replacement design routes in-transaction writes to a per-connection private
// *Delta (TxDelta on the Conn). ROLLBACK simply nils TxDelta — the live delta
// was never touched. COMMIT calls Merge to replay TxDelta into the live delta
// using a last-write-wins policy.
package delta

// Merge applies every mutation stored in src into d using a last-write-wins
// policy. It is called by the transaction COMMIT handler to promote a
// connection's private staging delta into the shared live delta.
//
// For each table that src has touched:
//   - Net-new inserts are copied into d; an existing entry for the same key
//     is overwritten (last write wins).
//   - Source-row overlays (updates) are copied into d; an existing overlay for
//     the same stable key is overwritten.
//   - Tombstones are added to d; any existing update overlay for the same
//     stable key is removed — a committed delete supersedes a prior update.
//   - currentToStable key mappings are merged in.
//
// Merge is safe for concurrent use. It copies all data out of src under src's
// read lock, releases that lock, then writes into d under d's write lock.
// Releasing src before acquiring d eliminates any possibility of deadlock
// between the two mutexes.
func (d *Delta) Merge(src *Delta) {
	// Phase 1 — read everything out of src while holding its read lock.
	type tableSnapshot struct {
		inserts         map[string]map[string]any
		updates         map[string]map[string]any
		tombstones      map[string]struct{}
		currentToStable map[string]string
		truncated       bool
	}

	src.mu.RLock()
	snaps := make(map[string]tableSnapshot, len(src.tables))
	for table, tbl := range src.tables {
		ins := make(map[string]map[string]any, len(tbl.inserts))
		for k, v := range tbl.inserts {
			ins[k] = copyRecord(v)
		}
		upd := make(map[string]map[string]any, len(tbl.updates))
		for k, v := range tbl.updates {
			upd[k] = copyRecord(v)
		}
		ts := make(map[string]struct{}, len(tbl.tombstones))
		for k := range tbl.tombstones {
			ts[k] = struct{}{}
		}
		cts := make(map[string]string, len(tbl.currentToStable))
		for k, v := range tbl.currentToStable {
			cts[k] = v
		}
		snaps[table] = tableSnapshot{ins, upd, ts, cts, tbl.truncated}
	}
	src.mu.RUnlock()

	if len(snaps) == 0 {
		return
	}

	// Phase 2 — write into d under d's write lock.
	d.mu.Lock()
	defer d.mu.Unlock()

	for table, snap := range snaps {
		if snap.truncated {
			d.tables[table] = newTableState()
			d.tables[table].truncated = true
		}
		dst := d.tableFor(table)

		for k, v := range snap.inserts {
			dst.inserts[k] = v
			// A committed insert supersedes any prior tombstone for the same
			// key: a DELETE followed by an INSERT in a later transaction must
			// make the row visible again.
			delete(dst.tombstones, k)
		}

		for stableKey, v := range snap.updates {
			dst.updates[stableKey] = v
		}

		// A committed delete supersedes any prior update overlay and any
		// prior net-new insert for the same row. Remove both before recording
		// the tombstone so that the live delta never holds a key in both
		// inserts and tombstones simultaneously.
		for stableKey := range snap.tombstones {
			delete(dst.updates, stableKey)
			delete(dst.inserts, stableKey)
			dst.tombstones[stableKey] = struct{}{}
		}

		for currentKey, stableKey := range snap.currentToStable {
			dst.currentToStable[currentKey] = stableKey
		}
	}
}
