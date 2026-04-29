// Package write provides write interception and delta overlay handlers.
package write

import (
	"fmt"
	"log"

	"github.com/virtual-db/core/internal/connection"
	"github.com/virtual-db/core/internal/delta"
	"github.com/virtual-db/core/internal/framework"
	"github.com/virtual-db/core/internal/payloads"
	"github.com/virtual-db/core/internal/points"
	"github.com/virtual-db/core/internal/schema"
)

// Handlers owns all pipeline points for write interception and delta overlay.
type Handlers struct {
	schema *schema.Cache
	delta  *delta.Delta
	conns  *connection.State
}

// New returns a Handlers ready for registration.
func New(sc *schema.Cache, d *delta.Delta, conns *connection.State) *Handlers {
	return &Handlers{schema: sc, delta: d, conns: conns}
}

// targetDelta returns the delta that writes from connID should be applied to.
// When a transaction is open (TxDelta != nil) writes go to the private staging
// delta so they remain invisible to other connections until COMMIT. Otherwise
// writes go directly to the shared live delta.
func (h *Handlers) targetDelta(connID uint32) *delta.Delta {
	if state, found := h.conns.Get(connID); found && state.TxDelta != nil {
		return state.TxDelta
	}
	return h.delta
}

// Register attaches all write handlers to r.
// Points covered:
//
//	write.insert.build_context     (10) → framework.BuildContext
//	write.insert.apply             (10) → h.InsertApply
//	write.update.build_context     (10) → framework.BuildContext
//	write.update.apply             (10) → h.UpdateApply
//	write.delete.build_context     (10) → framework.BuildContext
//	write.delete.apply             (10) → h.DeleteApply
//	records.source.build_context   (10) → framework.BuildContext
//	records.source.transform       (10) → h.RecordsOverlay
//	records.merged.build_context   (10) → framework.BuildContext
func (h *Handlers) Register(r framework.Registrar) error {
	for _, reg := range []struct {
		point    string
		priority int
		fn       framework.PointFunc
	}{
		{points.PointWriteInsertBuildContext, 10, framework.BuildContext},
		{points.PointWriteInsertApply, 10, h.InsertApply},
		{points.PointWriteUpdateBuildContext, 10, framework.BuildContext},
		{points.PointWriteUpdateApply, 10, h.UpdateApply},
		{points.PointWriteDeleteBuildContext, 10, framework.BuildContext},
		{points.PointWriteDeleteApply, 10, h.DeleteApply},
		{points.PointRecordsSourceBuildContext, 10, framework.BuildContext},
		{points.PointRecordsSourceTransform, 10, h.RecordsOverlay},
		{points.PointRecordsMergedBuildContext, 10, framework.BuildContext},
	} {
		if err := r.Attach(reg.point, reg.priority, reg.fn); err != nil {
			return fmt.Errorf("write: attach %q: %w", reg.point, err)
		}
	}
	return nil
}

// InsertApply records a net-new row in the appropriate delta.
// When the connection has an open transaction the row goes to TxDelta and
// remains invisible to other connections until COMMIT.
func (h *Handlers) InsertApply(ctx any, p any) (any, any, error) {
	payload, err := payloads.Decode[payloads.WriteInsertPayload](p)
	if err != nil {
		return ctx, nil, fmt.Errorf("write.insert.apply: %w", err)
	}
	if _, found := h.schema.Get(payload.Table); !found {
		log.Printf("write.insert.apply: schema not loaded for table %q; delta key may be incorrect", payload.Table)
	}
	d := h.targetDelta(payload.ConnectionID)
	if err := d.ApplyInsert(payload.Table, payload.Record); err != nil {
		return ctx, nil, fmt.Errorf("write.insert.apply: backend error: %w", err)
	}
	return ctx, payload, nil
}

// UpdateApply records an upsert overlay in the appropriate delta.
// When the connection has an open transaction the overlay goes to TxDelta and
// remains invisible to other connections until COMMIT.
func (h *Handlers) UpdateApply(ctx any, p any) (any, any, error) {
	payload, err := payloads.Decode[payloads.WriteUpdatePayload](p)
	if err != nil {
		return ctx, nil, fmt.Errorf("write.update.apply: %w", err)
	}
	if _, found := h.schema.Get(payload.Table); !found {
		log.Printf("write.update.apply: schema not loaded for table %q; delta key may be incorrect", payload.Table)
	}
	d := h.targetDelta(payload.ConnectionID)
	// When writing to a private TxDelta, pass the live delta as a fallback for
	// stable-key resolution. This handles chained UPDATEs that span implicit
	// transaction boundaries: the first UPDATE's currentToStable mapping was
	// merged into the live delta, so a freshly allocated TxDelta for the second
	// UPDATE cannot find it without consulting the live delta.
	var applyErr error
	if d != h.delta {
		applyErr = d.ApplyUpdateWithFallback(payload.Table, payload.OldRecord, payload.NewRecord, h.delta)
	} else {
		applyErr = d.ApplyUpdate(payload.Table, payload.OldRecord, payload.NewRecord)
	}
	if applyErr != nil {
		return ctx, nil, fmt.Errorf("write.update.apply: backend error: %w", applyErr)
	}
	return ctx, payload, nil
}

// DeleteApply records a tombstone in the appropriate delta.
// When the connection has an open transaction the tombstone goes to TxDelta
// and remains invisible to other connections until COMMIT.
func (h *Handlers) DeleteApply(ctx any, p any) (any, any, error) {
	payload, err := payloads.Decode[payloads.WriteDeletePayload](p)
	if err != nil {
		return ctx, nil, fmt.Errorf("write.delete.apply: %w", err)
	}
	if _, found := h.schema.Get(payload.Table); !found {
		log.Printf("write.delete.apply: schema not loaded for table %q; tombstone key may be incorrect", payload.Table)
	}
	d := h.targetDelta(payload.ConnectionID)
	// When writing to a private TxDelta, pass the live delta as a fallback for
	// stable-key resolution. This handles DELETE of a row whose RecordKey
	// changed due to an UPDATE committed in a prior implicit transaction: the
	// currentToStable mapping lives in the live delta, not in the fresh TxDelta.
	var applyErr error
	if d != h.delta {
		applyErr = d.ApplyDeleteWithFallback(payload.Table, payload.Record, h.delta)
	} else {
		applyErr = d.ApplyDelete(payload.Table, payload.Record)
	}
	if applyErr != nil {
		return ctx, nil, fmt.Errorf("write.delete.apply: backend error: %w", applyErr)
	}
	return ctx, payload, nil
}

// RecordsOverlay applies delta overlays to source records.
//
// When no transaction is open the source records are overlaid with the shared
// live delta — the existing behaviour.
//
// When the connection has an open transaction a second overlay pass is applied
// on top: the connection's private TxDelta is merged over the result of the
// first pass. This gives the writing connection read-your-own-writes
// visibility into its uncommitted changes while keeping those changes
// completely invisible to every other connection.
func (h *Handlers) RecordsOverlay(ctx any, p any) (any, any, error) {
	payload, err := payloads.Decode[payloads.RecordsSourcePayload](p)
	if err != nil {
		return ctx, nil, fmt.Errorf("records.source.transform: %w", err)
	}

	// Pass 1: apply the shared live delta (committed state, visible to all).
	merged, err := Overlay(h.delta, h.schema, payload.Table, payload.Records)
	if err != nil {
		return ctx, nil, fmt.Errorf("records.source.transform: live delta overlay failed for table %q: %w", payload.Table, err)
	}

	// Pass 2: if this connection has an open transaction, layer its private
	// staging delta on top so it can read its own uncommitted writes.
	if payload.ConnectionID != 0 {
		if state, found := h.conns.Get(payload.ConnectionID); found && state.TxDelta != nil {
			merged, err = Overlay(state.TxDelta, h.schema, payload.Table, merged)
			if err != nil {
				return ctx, nil, fmt.Errorf("records.source.transform: tx delta overlay failed for table %q: %w", payload.Table, err)
			}
		}
	}

	payload.Records = merged
	return ctx, payload, nil
}
