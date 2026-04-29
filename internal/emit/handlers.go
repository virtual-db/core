// Package emit provides the framework's emit-point handlers. These handlers
// fire the corresponding standard vdb.* events via the bus. No mutable state.
package emit

import (
	"fmt"

	"github.com/virtual-db/core/internal/framework"
	"github.com/virtual-db/core/internal/payloads"
	"github.com/virtual-db/core/internal/points"
)

// Handlers fires the standard vdb.* events from pipeline emit points.
// No mutable state.
type Handlers struct{}

// New returns a Handlers ready for registration.
func New() *Handlers {
	return &Handlers{}
}

// Register attaches all emit handlers to r.
// Points covered:
//
//	server.stop.emit             (10) → h.ServerStopped
//	connection.opened.emit       (10) → h.ConnectionOpened
//	connection.closed.emit       (10) → h.ConnectionClosed
//	transaction.begin.emit       (10) → h.TransactionStarted
//	transaction.commit.emit      (10) → h.TransactionCommitted
//	transaction.rollback.emit    (10) → h.TransactionRolledBack
//	write.insert.emit            (10) → h.RecordInserted
//	write.update.emit            (10) → h.RecordUpdated
//	write.delete.emit            (10) → h.RecordDeleted
func (h *Handlers) Register(r framework.Registrar) error {
	for _, reg := range []struct {
		point    string
		priority int
		fn       framework.PointFunc
	}{
		{points.PointServerStopEmit, 10, h.ServerStopped},
		{points.PointConnectionOpenedEmit, 10, h.ConnectionOpened},
		{points.PointConnectionClosedEmit, 10, h.ConnectionClosed},
		{points.PointTransactionBeginEmit, 10, h.TransactionStarted},
		{points.PointTransactionCommitEmit, 10, h.TransactionCommitted},
		{points.PointTransactionRollbackEmit, 10, h.TransactionRolledBack},
		{points.PointWriteInsertEmit, 10, h.RecordInserted},
		{points.PointWriteUpdateEmit, 10, h.RecordUpdated},
		{points.PointWriteDeleteEmit, 10, h.RecordDeleted},
	} {
		if err := r.Attach(reg.point, reg.priority, reg.fn); err != nil {
			return fmt.Errorf("emit: attach %q: %w", reg.point, err)
		}
	}
	return nil
}

// ServerStopped fires vdb.server.stopped from vdb.server.stop.emit.
func (h *Handlers) ServerStopped(ctx any, p any) (any, any, error) {
	hctx := ctx.(framework.HandlerContext)
	hctx.Global.Bus().Emit(points.EventServerStopped, p, hctx)
	return ctx, p, nil
}

// ConnectionOpened fires vdb.connection.opened from vdb.connection.opened.emit.
func (h *Handlers) ConnectionOpened(ctx any, p any) (any, any, error) {
	hctx := ctx.(framework.HandlerContext)
	payload, err := payloads.Decode[payloads.ConnectionOpenedPayload](p)
	if err != nil {
		return ctx, nil, fmt.Errorf("connection.opened.emit: %w", err)
	}
	hctx.Global.Bus().Emit(points.EventConnectionOpened, payload, hctx)
	return ctx, payload, nil
}

// ConnectionClosed fires vdb.connection.closed from vdb.connection.closed.emit.
func (h *Handlers) ConnectionClosed(ctx any, p any) (any, any, error) {
	hctx := ctx.(framework.HandlerContext)
	payload, err := payloads.Decode[payloads.ConnectionClosedPayload](p)
	if err != nil {
		return ctx, nil, fmt.Errorf("connection.closed.emit: %w", err)
	}
	hctx.Global.Bus().Emit(points.EventConnectionClosed, payload, hctx)
	return ctx, payload, nil
}

// TransactionStarted fires vdb.transaction.started from vdb.transaction.begin.emit.
func (h *Handlers) TransactionStarted(ctx any, p any) (any, any, error) {
	hctx := ctx.(framework.HandlerContext)
	payload, err := payloads.Decode[payloads.TransactionBeginPayload](p)
	if err != nil {
		return ctx, nil, fmt.Errorf("transaction.begin.emit: %w", err)
	}
	hctx.Global.Bus().Emit(points.EventTransactionStarted, payload, hctx)
	return ctx, payload, nil
}

// TransactionCommitted fires vdb.transaction.committed from vdb.transaction.commit.emit.
func (h *Handlers) TransactionCommitted(ctx any, p any) (any, any, error) {
	hctx := ctx.(framework.HandlerContext)
	payload, err := payloads.Decode[payloads.TransactionCommitPayload](p)
	if err != nil {
		return ctx, nil, fmt.Errorf("transaction.commit.emit: %w", err)
	}
	hctx.Global.Bus().Emit(points.EventTransactionCommitted, payload, hctx)
	return ctx, payload, nil
}

// TransactionRolledBack fires vdb.transaction.rolledback from vdb.transaction.rollback.emit.
func (h *Handlers) TransactionRolledBack(ctx any, p any) (any, any, error) {
	hctx := ctx.(framework.HandlerContext)
	payload, err := payloads.Decode[payloads.TransactionRollbackPayload](p)
	if err != nil {
		return ctx, nil, fmt.Errorf("transaction.rollback.emit: %w", err)
	}
	hctx.Global.Bus().Emit(points.EventTransactionRolledback, payload, hctx)
	return ctx, payload, nil
}

// RecordInserted fires vdb.record.inserted from vdb.write.insert.emit.
func (h *Handlers) RecordInserted(ctx any, p any) (any, any, error) {
	hctx := ctx.(framework.HandlerContext)
	payload, err := payloads.Decode[payloads.WriteInsertPayload](p)
	if err != nil {
		return ctx, nil, fmt.Errorf("write.insert.emit: %w", err)
	}
	hctx.Global.Bus().Emit(points.EventRecordInserted, payload, hctx)
	return ctx, payload, nil
}

// RecordUpdated fires vdb.record.updated from vdb.write.update.emit.
func (h *Handlers) RecordUpdated(ctx any, p any) (any, any, error) {
	hctx := ctx.(framework.HandlerContext)
	payload, err := payloads.Decode[payloads.WriteUpdatePayload](p)
	if err != nil {
		return ctx, nil, fmt.Errorf("write.update.emit: %w", err)
	}
	hctx.Global.Bus().Emit(points.EventRecordUpdated, payload, hctx)
	return ctx, payload, nil
}

// RecordDeleted fires vdb.record.deleted from vdb.write.delete.emit.
func (h *Handlers) RecordDeleted(ctx any, p any) (any, any, error) {
	hctx := ctx.(framework.HandlerContext)
	payload, err := payloads.Decode[payloads.WriteDeletePayload](p)
	if err != nil {
		return ctx, nil, fmt.Errorf("write.delete.emit: %w", err)
	}
	hctx.Global.Bus().Emit(points.EventRecordDeleted, payload, hctx)
	return ctx, payload, nil
}
