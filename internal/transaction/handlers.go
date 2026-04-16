// Package transaction owns transaction snapshot lifecycle handlers.
package transaction

import (
	"fmt"

	"github.com/AnqorDX/vdb-core/internal/connection"
	"github.com/AnqorDX/vdb-core/internal/delta"
	"github.com/AnqorDX/vdb-core/internal/framework"
	"github.com/AnqorDX/vdb-core/internal/payloads"
	"github.com/AnqorDX/vdb-core/internal/points"
)

// Handlers owns all pipeline points that manage transaction lifecycle.
type Handlers struct {
	conns *connection.State
	delta *delta.Delta
}

// New returns a Handlers ready for registration.
func New(conns *connection.State, d *delta.Delta) *Handlers {
	return &Handlers{conns: conns, delta: d}
}

// Register attaches all transaction handlers to r.
// Points covered:
//
//	transaction.begin.build_context    (10) → framework.BuildContext
//	transaction.begin.authorize        (10) → h.Authorize
//	transaction.commit.build_context   (10) → framework.BuildContext
//	transaction.commit.apply           (10) → h.CommitApply
//	transaction.rollback.build_context (10) → framework.BuildContext
//	transaction.rollback.apply         (10) → h.RollbackApply
func (h *Handlers) Register(r framework.Registrar) error {
	for _, reg := range []struct {
		point    string
		priority int
		fn       framework.PointFunc
	}{
		{points.PointTransactionBeginBuildContext, 10, framework.BuildContext},
		{points.PointTransactionBeginAuthorize, 10, h.Authorize},
		{points.PointTransactionCommitBuildContext, 10, framework.BuildContext},
		{points.PointTransactionCommitApply, 10, h.CommitApply},
		{points.PointTransactionRollbackBuildContext, 10, framework.BuildContext},
		{points.PointTransactionRollbackApply, 10, h.RollbackApply},
	} {
		if err := r.Attach(reg.point, reg.priority, reg.fn); err != nil {
			return fmt.Errorf("transaction: attach %q: %w", reg.point, err)
		}
	}
	return nil
}

// Authorize handles BEGIN by allocating a fresh private staging delta for the
// connection. From this point forward, all writes issued by this connection
// are routed to TxDelta by the write handlers and remain invisible to every
// other connection until COMMIT promotes them into the shared live delta.
//
// The live delta is never modified by in-transaction writes, so ROLLBACK
// requires no undo work — it simply discards TxDelta.
func (h *Handlers) Authorize(ctx any, p any) (any, any, error) {
	payload, ok := p.(payloads.TransactionBeginPayload)
	if !ok {
		return ctx, nil, fmt.Errorf("transaction.begin.authorize: unexpected payload type %T", p)
	}
	if state, found := h.conns.Get(payload.ConnectionID); found {
		state.TxDelta = delta.New()
	}
	return ctx, payload, nil
}

// CommitApply promotes the connection's private staging delta into the shared
// live delta using a last-write-wins merge, then clears TxDelta. After this
// call the committed writes are visible to all connections.
//
// If the connection has no open transaction (TxDelta == nil) the call is a
// no-op, matching MySQL's behaviour for COMMIT outside a transaction.
func (h *Handlers) CommitApply(ctx any, p any) (any, any, error) {
	payload, ok := p.(payloads.TransactionCommitPayload)
	if !ok {
		return ctx, nil, fmt.Errorf("transaction.commit.apply: unexpected payload type %T", p)
	}
	if state, found := h.conns.Get(payload.ConnectionID); found && state.TxDelta != nil {
		h.delta.Merge(state.TxDelta)
		state.TxDelta = nil
	}
	return ctx, payload, nil
}

// RollbackApply discards the connection's private staging delta. Because
// in-transaction writes were never applied to the shared live delta, no
// undo work is required — dropping TxDelta is sufficient to make the
// transaction's writes permanently invisible.
//
// If the connection has no open transaction (TxDelta == nil) the call is a
// no-op, matching MySQL's behaviour for ROLLBACK outside a transaction.
func (h *Handlers) RollbackApply(ctx any, p any) (any, any, error) {
	payload, ok := p.(payloads.TransactionRollbackPayload)
	if !ok {
		return ctx, nil, fmt.Errorf("transaction.rollback.apply: unexpected payload type %T", p)
	}
	if state, found := h.conns.Get(payload.ConnectionID); found {
		state.TxDelta = nil
	}
	return ctx, payload, nil
}
