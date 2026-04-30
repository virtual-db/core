// Package delta provides the framework's single, unconditional mutation store.
// Delta is always constructed in core.New() and is never nil after construction.
// There is no swappable backend interface.
package delta

import (
	"sync"
)

// tableState holds all delta mutations for a single table, categorised by
// mutation origin.
type tableState struct {
	// inserts holds net-new rows (created via ApplyInsert with no source
	// counterpart). Keyed by RecordKey of the row's CURRENT state.
	inserts map[string]map[string]any

	// updates holds upsert overlays for SOURCE rows. Keyed by the STABLE
	// source RecordKey.
	updates map[string]map[string]any

	// tombstones records deleted source rows. Keyed by the stable source RecordKey.
	tombstones map[string]struct{}

	// truncated is true when ApplyTruncate has been called for this table.
	truncated bool

	// currentToStable maps RecordKey(current_state) -> stable_source_key.
	currentToStable map[string]string
}

func newTableState() *tableState {
	return &tableState{
		inserts:         make(map[string]map[string]any),
		updates:         make(map[string]map[string]any),
		tombstones:      make(map[string]struct{}),
		currentToStable: make(map[string]string),
	}
}

// Delta is the mutation store for a VirtualDB session. It records writes
// that clients issue — inserts, updates, and deletes — without forwarding
// them to the source database. The framework overlays the stored state on
// top of source rows when assembling read results.
//
// Delta is safe for concurrent use by multiple goroutines.
type Delta struct {
	mu     sync.RWMutex
	tables map[string]*tableState
}

// New allocates and returns a ready-to-use Delta.
func New() *Delta {
	return &Delta{
		tables: make(map[string]*tableState),
	}
}

// tableFor returns the tableState for table, creating it if absent.
// Caller MUST hold the write lock.
func (d *Delta) tableFor(table string) *tableState {
	tbl, ok := d.tables[table]
	if !ok {
		tbl = newTableState()
		d.tables[table] = tbl
	}
	return tbl
}

func copyRecord(r map[string]any) map[string]any {
	out := make(map[string]any, len(r))
	for k, v := range r {
		out[k] = v
	}
	return out
}

func copyTableState(tbl *tableState) *tableState {
	out := newTableState()
	for k, v := range tbl.inserts {
		out.inserts[k] = copyRecord(v)
	}
	for k, v := range tbl.updates {
		out.updates[k] = copyRecord(v)
	}
	for k := range tbl.tombstones {
		out.tombstones[k] = struct{}{}
	}
	for k, v := range tbl.currentToStable {
		out.currentToStable[k] = v
	}
	out.truncated = tbl.truncated
	return out
}
