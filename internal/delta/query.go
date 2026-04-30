package delta

import (
	"fmt"
	"sort"
	"strings"
)

func (d *Delta) Records(table string) ([]map[string]any, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	tbl, ok := d.tables[table]
	if !ok {
		return nil, nil
	}
	var result []map[string]any
	for _, r := range tbl.inserts {
		result = append(result, copyRecord(r))
	}
	for _, r := range tbl.updates {
		result = append(result, copyRecord(r))
	}
	return result, nil
}

// DeltaTableState is a point-in-time snapshot of the delta for a single
// table, categorised by mutation type. All maps are keyed by RecordKey.
// The returned value is a copy; callers may read it without holding any lock.
type DeltaTableState struct {
	// Inserts holds net-new rows: rows recorded via ApplyInsert that have
	// no counterpart in the source database.
	Inserts map[string]map[string]any

	// Updates holds upsert overlays: rows recorded via ApplyUpdate whose
	// PK exists in the source database.
	Updates map[string]map[string]any

	// Tombstones is the set of deleted source rows.
	Tombstones map[string]struct{}

	// Truncated is true when ApplyTruncate has been called for this table.
	Truncated bool
}

func (d *Delta) TableState(table string) (DeltaTableState, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	tbl, ok := d.tables[table]
	if !ok {
		return DeltaTableState{
			Inserts:    make(map[string]map[string]any),
			Updates:    make(map[string]map[string]any),
			Tombstones: make(map[string]struct{}),
			Truncated:  false,
		}, nil
	}

	inserts := make(map[string]map[string]any, len(tbl.inserts))
	for k, v := range tbl.inserts {
		inserts[k] = copyRecord(v)
	}
	updates := make(map[string]map[string]any, len(tbl.updates))
	for k, v := range tbl.updates {
		updates[k] = copyRecord(v)
	}
	tombstones := make(map[string]struct{}, len(tbl.tombstones))
	for k := range tbl.tombstones {
		tombstones[k] = struct{}{}
	}
	truncated := tbl.truncated

	return DeltaTableState{
		Inserts:    inserts,
		Updates:    updates,
		Tombstones: tombstones,
		Truncated:  truncated,
	}, nil
}

// RecordKey produces a canonical string key for a record based on all its
// fields, sorted lexicographically by field name.
func RecordKey(r map[string]any) string {
	keys := make([]string, 0, len(r))
	for k := range r {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('|')
		}
		fmt.Fprintf(&b, "%s=%v", k, r[k])
	}
	return b.String()
}
