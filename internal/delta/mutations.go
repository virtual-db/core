package delta

// ApplyInsert records a net-new row in the delta.
func (d *Delta) ApplyInsert(table string, record map[string]any) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	tbl := d.tableFor(table)
	tbl.inserts[RecordKey(record)] = copyRecord(record)
	return nil
}

// ApplyUpdate records a source-row overlay in the delta. old is the row as it
// appeared when GMS read it (possibly already overlaid by a prior delta), and
// new is the intended replacement. The stable source key — the RecordKey of
// the original un-modified source row — is resolved by consulting the delta's
// own currentToStable map. If not found there, oldKey is used as the stable
// key directly (i.e. old is already the original source row).
func (d *Delta) ApplyUpdate(table string, old, new map[string]any) error {
	return d.applyUpdate(table, old, new, nil)
}

// ApplyUpdateWithFallback is identical to ApplyUpdate but also consults
// fallback's currentToStable when the local map has no entry for oldKey.
//
// This is necessary for chained UPDATEs that span implicit transaction
// boundaries (autocommit). When GMS wraps each statement in its own implicit
// BEGIN / COMMIT, the second UPDATE's TxDelta is freshly allocated and has no
// knowledge of the stable-key mapping produced by the first UPDATE's TxDelta
// (which has already been merged into the live delta). Passing the live delta
// as fallback lets the second UPDATE resolve the correct stable source key even
// though its own TxDelta has never seen that mapping.
//
// The fallback read is performed before d's write lock is acquired so there is
// no nested lock acquisition; the only lock ordering is: fallback.mu.RLock →
// release → d.mu.Lock. Because TxDelta is private to a single connection no
// other goroutine holds TxDelta's lock, so this ordering is deadlock-free.
//
// Callers must ensure fallback != d.
func (d *Delta) ApplyUpdateWithFallback(table string, old, new map[string]any, fallback *Delta) error {
	return d.applyUpdate(table, old, new, fallback)
}

// applyUpdate is the shared implementation behind ApplyUpdate and
// ApplyUpdateWithFallback.
func (d *Delta) applyUpdate(table string, old, new map[string]any, fallback *Delta) error {
	oldKey := RecordKey(old)
	newKey := RecordKey(new)

	// Pre-resolve the stable key from the fallback (live) delta before
	// acquiring d's write lock to avoid nested lock acquisition. The returned
	// string is empty when the fallback has no mapping for oldKey, which is the
	// common case for the first UPDATE (where old is always the source row).
	var fallbackStableKey string
	if fallback != nil {
		fallback.mu.RLock()
		if fbTbl, ok := fallback.tables[table]; ok {
			fallbackStableKey = fbTbl.currentToStable[oldKey]
		}
		fallback.mu.RUnlock()
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	tbl := d.tableFor(table)

	// Case 1: net-new row (from a prior ApplyInsert in the same delta).
	// Re-key in inserts; never touch updates.
	if _, isInsert := tbl.inserts[oldKey]; isInsert {
		delete(tbl.inserts, oldKey)
		tbl.inserts[newKey] = copyRecord(new)
		return nil
	}

	// Case 2: source-row overlay (or update-of-update). Resolve the stable
	// source key using the following priority order:
	//
	//  1. Local currentToStable — handles same-transaction chained updates
	//     where the first update already recorded the mapping in this delta.
	//
	//  2. fallbackStableKey — handles chained updates across implicit
	//     transaction boundaries: the first UPDATE committed its mapping into
	//     the live delta; the second UPDATE's fresh TxDelta uses the
	//     pre-resolved fallback key instead.
	//
	//  3. oldKey itself — old is already the original source row; no
	//     translation needed.
	stableKey := oldKey
	if stable, found := tbl.currentToStable[oldKey]; found {
		stableKey = stable
		delete(tbl.currentToStable, oldKey)
	} else if fallbackStableKey != "" {
		stableKey = fallbackStableKey
	}

	tbl.updates[stableKey] = copyRecord(new)
	tbl.currentToStable[newKey] = stableKey
	return nil
}

// ApplyDelete records a tombstone for a source row and removes any prior
// update overlay for the same row.
func (d *Delta) ApplyDelete(table string, record map[string]any) error {
	return d.applyDelete(table, record, nil)
}

// ApplyDeleteWithFallback is identical to ApplyDelete but also consults
// fallback's currentToStable when the local map has no entry for the deleted
// record's key.
//
// This is necessary when a DELETE targets a row whose RecordKey changed due to
// an UPDATE committed in a prior implicit transaction (autocommit). The
// RecordKey of the row as returned by PartitionRows reflects the updated
// value, but the stable source key — needed to create the correct tombstone —
// lives in the live delta's currentToStable rather than in the fresh TxDelta.
// Without consulting the fallback, the tombstone is recorded under the wrong
// key and the stale insert/update records are never cleaned up.
//
// The fallback read is performed before d's write lock is acquired so there is
// no nested lock acquisition; the only lock ordering is: fallback.mu.RLock →
// release → d.mu.Lock. Because TxDelta is private to a single connection no
// other goroutine holds TxDelta's lock, so this ordering is deadlock-free.
//
// Callers must ensure fallback != d.
func (d *Delta) ApplyDeleteWithFallback(table string, record map[string]any, fallback *Delta) error {
	return d.applyDelete(table, record, fallback)
}

// applyDelete is the shared implementation behind ApplyDelete and
// ApplyDeleteWithFallback.
func (d *Delta) applyDelete(table string, record map[string]any, fallback *Delta) error {
	key := RecordKey(record)

	// Pre-resolve the stable key from the fallback (live) delta before
	// acquiring d's write lock to avoid nested lock acquisition. The returned
	// string is empty when the fallback has no mapping for key, which is the
	// common case (the deleted row was never updated in a prior transaction).
	var fallbackStableKey string
	if fallback != nil {
		fallback.mu.RLock()
		if fbTbl, ok := fallback.tables[table]; ok {
			fallbackStableKey = fbTbl.currentToStable[key]
		}
		fallback.mu.RUnlock()
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	tbl := d.tableFor(table)

	// Case 1: net-new row deletion in this delta — remove from inserts, no
	// tombstone needed.
	if _, isInsert := tbl.inserts[key]; isInsert {
		delete(tbl.inserts, key)
		return nil
	}

	// Case 2: source row (possibly after updates). Resolve the stable source
	// key using the following priority order:
	//
	//  1. Local currentToStable — handles same-transaction chained deletes
	//     where a prior UPDATE in this delta already recorded the mapping.
	//
	//  2. fallbackStableKey — handles the cross-boundary case: the row was
	//     updated in a prior committed transaction, its currentToStable
	//     mapping was merged into the live delta, and this fresh TxDelta has
	//     never seen it. The fallback key is used instead.
	//
	//  3. key itself — the deleted row is the original source row with no
	//     prior update; no translation needed.
	stableKey := key
	if stable, found := tbl.currentToStable[key]; found {
		stableKey = stable
		delete(tbl.currentToStable, key)
	} else if fallbackStableKey != "" {
		stableKey = fallbackStableKey
	}

	delete(tbl.updates, stableKey)
	tbl.tombstones[stableKey] = struct{}{}
	return nil
}
