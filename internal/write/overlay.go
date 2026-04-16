// Package write provides write interception and delta overlay handlers.
package write

import (
	"fmt"
	"log"

	"github.com/AnqorDX/vdb-core/internal/delta"
	"github.com/AnqorDX/vdb-core/internal/schema"
)

// Overlay merges the delta state for table onto the source record slice.
// Extracted as a named function so it is unit-testable without the handler
// dispatch machinery. Returns a new slice; source is never modified.
func Overlay(
	d *delta.Delta,
	sc *schema.Cache,
	table string,
	source []map[string]any,
) ([]map[string]any, error) {
	if _, ok := sc.Get(table); !ok {
		log.Printf("records.source.transform: schema not loaded for table %q; "+
			"skipping delta overlay — source records returned unmerged", table)
		return source, nil
	}

	state, err := d.TableState(table)
	if err != nil {
		return nil, fmt.Errorf("TableState(%q): %w", table, err)
	}

	if len(state.Inserts) == 0 && len(state.Updates) == 0 && len(state.Tombstones) == 0 {
		return source, nil
	}

	result := make([]map[string]any, 0, len(source))
	seen := make(map[string]struct{}, len(source))

	for _, rec := range source {
		key := delta.RecordKey(rec)
		seen[key] = struct{}{}

		if _, tombstoned := state.Tombstones[key]; tombstoned {
			continue
		}
		if overlay, updated := state.Updates[key]; updated {
			result = append(result, overlay)
		} else {
			result = append(result, rec)
		}
	}

	for insertKey, insertedRec := range state.Inserts {
		if _, inSource := seen[insertKey]; !inSource {
			// A tombstone for a net-new delta key means the row was deleted in
			// a later transaction. Suppress it so the row is not re-surfaced
			// after the delete committed. (Merge guarantees that inserts and
			// tombstones are mutually exclusive after every COMMIT, but we
			// check here as defense-in-depth.)
			if _, tombstoned := state.Tombstones[insertKey]; tombstoned {
				continue
			}
			// If an ODKU update targeted this net-new row (e.g. the row was
			// inserted into a prior transaction and the current transaction
			// applied an ON DUPLICATE KEY UPDATE against it), the update
			// overlay is keyed by the original insert key and takes precedence
			// over the stale insert record.
			if overlay, updated := state.Updates[insertKey]; updated {
				result = append(result, overlay)
			} else {
				result = append(result, insertedRec)
			}
		}
	}

	return result, nil
}
