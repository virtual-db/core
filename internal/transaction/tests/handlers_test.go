package transaction_test

import (
	"testing"

	"github.com/AnqorDX/vdb-core/internal/connection"
	"github.com/AnqorDX/vdb-core/internal/delta"
	"github.com/AnqorDX/vdb-core/internal/framework"
	"github.com/AnqorDX/vdb-core/internal/payloads"
	"github.com/AnqorDX/vdb-core/internal/points"
	. "github.com/AnqorDX/vdb-core/internal/transaction"
)

func newTxPipe(t *testing.T) (*framework.Pipeline, *connection.State, *delta.Delta) {
	t.Helper()
	var global framework.GlobalContext
	pipe := framework.NewPipeline(&global)
	bus := framework.NewBus(&global)
	global = framework.SealContext(framework.NewGlobalContextBuilder(), bus, pipe)

	pipe.DeclarePipeline(points.PipelineTransactionBegin, []string{
		points.PointTransactionBeginBuildContext,
		points.PointTransactionBeginAuthorize,
		points.PointTransactionBeginEmit,
	})
	pipe.DeclarePipeline(points.PipelineTransactionCommit, []string{
		points.PointTransactionCommitBuildContext,
		points.PointTransactionCommitApply,
		points.PointTransactionCommitEmit,
	})
	pipe.DeclarePipeline(points.PipelineTransactionRollback, []string{
		points.PointTransactionRollbackBuildContext,
		points.PointTransactionRollbackApply,
		points.PointTransactionRollbackEmit,
	})

	conns := connection.NewState()
	d := delta.New()
	h := New(conns, d)
	if err := h.Register(pipe); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return pipe, conns, d
}

// TestHandlers_Register_ReturnsNilError confirms that handler registration
// does not error under normal conditions.
func TestHandlers_Register_ReturnsNilError(t *testing.T) {
	var global framework.GlobalContext
	pipe := framework.NewPipeline(&global)
	bus := framework.NewBus(&global)
	global = framework.SealContext(framework.NewGlobalContextBuilder(), bus, pipe)

	pipe.DeclarePipeline(points.PipelineTransactionBegin, []string{
		points.PointTransactionBeginBuildContext,
		points.PointTransactionBeginAuthorize,
		points.PointTransactionBeginEmit,
	})
	pipe.DeclarePipeline(points.PipelineTransactionCommit, []string{
		points.PointTransactionCommitBuildContext,
		points.PointTransactionCommitApply,
		points.PointTransactionCommitEmit,
	})
	pipe.DeclarePipeline(points.PipelineTransactionRollback, []string{
		points.PointTransactionRollbackBuildContext,
		points.PointTransactionRollbackApply,
		points.PointTransactionRollbackEmit,
	})

	conns := connection.NewState()
	d := delta.New()
	h := New(conns, d)

	if err := h.Register(pipe); err != nil {
		t.Errorf("expected Register to return nil error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// BEGIN
// ---------------------------------------------------------------------------

// TestHandlers_Begin_AllocatesTxDelta verifies that BEGIN allocates a fresh
// private staging delta on the connection. Before BEGIN the field is nil;
// after BEGIN it must be a non-nil *delta.Delta ready to accept writes.
func TestHandlers_Begin_AllocatesTxDelta(t *testing.T) {
	pipe, conns, _ := newTxPipe(t)

	conns.Set(1, &connection.Conn{ID: 1})

	_, err := pipe.Process(points.PipelineTransactionBegin,
		payloads.TransactionBeginPayload{ConnectionID: 1})
	if err != nil {
		t.Fatalf("Process(begin): %v", err)
	}

	conn, ok := conns.Get(1)
	if !ok {
		t.Fatal("connection 1 not found in state")
	}
	if conn.TxDelta == nil {
		t.Error("expected conn.TxDelta to be non-nil after BEGIN")
	}
}

// TestHandlers_Begin_TxDeltaIsEmpty verifies that the TxDelta allocated at
// BEGIN starts with no mutations. Writes made before BEGIN must not bleed
// into the fresh staging delta.
func TestHandlers_Begin_TxDeltaIsEmpty(t *testing.T) {
	pipe, conns, live := newTxPipe(t)

	// Write to the live delta before BEGIN — this must not appear in TxDelta.
	if err := live.ApplyInsert("users", map[string]any{"id": 1, "name": "pre-begin"}); err != nil {
		t.Fatalf("ApplyInsert to live: %v", err)
	}

	conns.Set(1, &connection.Conn{ID: 1})
	_, err := pipe.Process(points.PipelineTransactionBegin,
		payloads.TransactionBeginPayload{ConnectionID: 1})
	if err != nil {
		t.Fatalf("Process(begin): %v", err)
	}

	conn, _ := conns.Get(1)
	state, err := conn.TxDelta.TableState("users")
	if err != nil {
		t.Fatalf("TxDelta.TableState: %v", err)
	}
	if got := len(state.Inserts); got != 0 {
		t.Errorf("expected TxDelta to be empty after BEGIN, got %d inserts", got)
	}
}

// ---------------------------------------------------------------------------
// COMMIT
// ---------------------------------------------------------------------------

// TestHandlers_Commit_MergesIntoLiveDelta verifies that COMMIT promotes every
// mutation in the connection's TxDelta into the shared live delta.
func TestHandlers_Commit_MergesIntoLiveDelta(t *testing.T) {
	pipe, conns, live := newTxPipe(t)

	conns.Set(1, &connection.Conn{ID: 1})
	_, err := pipe.Process(points.PipelineTransactionBegin,
		payloads.TransactionBeginPayload{ConnectionID: 1})
	if err != nil {
		t.Fatalf("Process(begin): %v", err)
	}

	// Simulate a write handler routing an in-transaction write to TxDelta.
	conn, _ := conns.Get(1)
	if err := conn.TxDelta.ApplyInsert("users",
		map[string]any{"id": 2, "name": "bob"}); err != nil {
		t.Fatalf("TxDelta.ApplyInsert: %v", err)
	}

	_, err = pipe.Process(points.PipelineTransactionCommit,
		payloads.TransactionCommitPayload{ConnectionID: 1})
	if err != nil {
		t.Fatalf("Process(commit): %v", err)
	}

	// Bob must now be in the live delta.
	state, err := live.TableState("users")
	if err != nil {
		t.Fatalf("live.TableState: %v", err)
	}
	if got := len(state.Inserts); got != 1 {
		t.Errorf("expected 1 insert in live delta after COMMIT, got %d", got)
	}
}

// TestHandlers_Commit_ClearsTxDelta verifies that TxDelta is nil after COMMIT,
// so the connection reverts to writing directly to the live delta.
func TestHandlers_Commit_ClearsTxDelta(t *testing.T) {
	pipe, conns, _ := newTxPipe(t)

	conns.Set(1, &connection.Conn{ID: 1})
	_, err := pipe.Process(points.PipelineTransactionBegin,
		payloads.TransactionBeginPayload{ConnectionID: 1})
	if err != nil {
		t.Fatalf("Process(begin): %v", err)
	}

	_, err = pipe.Process(points.PipelineTransactionCommit,
		payloads.TransactionCommitPayload{ConnectionID: 1})
	if err != nil {
		t.Fatalf("Process(commit): %v", err)
	}

	conn, ok := conns.Get(1)
	if !ok {
		t.Fatal("connection 1 not found after COMMIT")
	}
	if conn.TxDelta != nil {
		t.Error("expected conn.TxDelta to be nil after COMMIT")
	}
}

// TestHandlers_Commit_OutsideTransaction_IsNoOp verifies that COMMIT when no
// transaction is open does not error and leaves the live delta unchanged.
func TestHandlers_Commit_OutsideTransaction_IsNoOp(t *testing.T) {
	pipe, conns, live := newTxPipe(t)

	if err := live.ApplyInsert("users",
		map[string]any{"id": 1, "name": "alice"}); err != nil {
		t.Fatalf("ApplyInsert to live: %v", err)
	}

	// Connection with no open transaction (TxDelta is nil).
	conns.Set(1, &connection.Conn{ID: 1})

	_, err := pipe.Process(points.PipelineTransactionCommit,
		payloads.TransactionCommitPayload{ConnectionID: 1})
	if err != nil {
		t.Fatalf("Process(commit outside tx): %v", err)
	}

	state, err := live.TableState("users")
	if err != nil {
		t.Fatalf("live.TableState: %v", err)
	}
	if got := len(state.Inserts); got != 1 {
		t.Errorf("expected live delta unchanged after COMMIT outside tx, got %d inserts", got)
	}
}

// ---------------------------------------------------------------------------
// ROLLBACK
// ---------------------------------------------------------------------------

// TestHandlers_Rollback_NilsTxDelta verifies that ROLLBACK sets TxDelta to nil,
// so the connection reverts to writing directly to the live delta.
func TestHandlers_Rollback_NilsTxDelta(t *testing.T) {
	pipe, conns, _ := newTxPipe(t)

	conns.Set(1, &connection.Conn{ID: 1})
	_, err := pipe.Process(points.PipelineTransactionBegin,
		payloads.TransactionBeginPayload{ConnectionID: 1})
	if err != nil {
		t.Fatalf("Process(begin): %v", err)
	}

	_, err = pipe.Process(points.PipelineTransactionRollback,
		payloads.TransactionRollbackPayload{ConnectionID: 1})
	if err != nil {
		t.Fatalf("Process(rollback): %v", err)
	}

	conn, ok := conns.Get(1)
	if !ok {
		t.Fatal("connection 1 not found after ROLLBACK")
	}
	if conn.TxDelta != nil {
		t.Error("expected conn.TxDelta to be nil after ROLLBACK")
	}
}

// TestHandlers_Rollback_LiveDeltaUntouched verifies that in-transaction writes
// (which went to TxDelta) are never visible in the live delta after ROLLBACK.
// Because in-transaction writes never touched the live delta, no undo work is
// needed — dropping TxDelta is sufficient.
func TestHandlers_Rollback_LiveDeltaUntouched(t *testing.T) {
	pipe, conns, live := newTxPipe(t)

	conns.Set(1, &connection.Conn{ID: 1})
	_, err := pipe.Process(points.PipelineTransactionBegin,
		payloads.TransactionBeginPayload{ConnectionID: 1})
	if err != nil {
		t.Fatalf("Process(begin): %v", err)
	}

	// Simulate in-transaction writes going to TxDelta (as the write handler
	// would route them in production).
	conn, _ := conns.Get(1)
	if err := conn.TxDelta.ApplyInsert("users",
		map[string]any{"id": 1, "name": "should-vanish"}); err != nil {
		t.Fatalf("TxDelta.ApplyInsert: %v", err)
	}

	_, err = pipe.Process(points.PipelineTransactionRollback,
		payloads.TransactionRollbackPayload{ConnectionID: 1})
	if err != nil {
		t.Fatalf("Process(rollback): %v", err)
	}

	// The in-transaction write must not appear in the live delta.
	state, err := live.TableState("users")
	if err != nil {
		t.Fatalf("live.TableState: %v", err)
	}
	if got := len(state.Inserts); got != 0 {
		t.Errorf("expected 0 inserts in live delta after ROLLBACK, got %d", got)
	}
}

// TestHandlers_Rollback_OutsideTransaction_IsNoOp verifies that ROLLBACK when
// no transaction is open does not error and leaves the live delta unchanged.
func TestHandlers_Rollback_OutsideTransaction_IsNoOp(t *testing.T) {
	pipe, conns, live := newTxPipe(t)

	if err := live.ApplyInsert("users",
		map[string]any{"id": 1, "name": "alice"}); err != nil {
		t.Fatalf("ApplyInsert to live: %v", err)
	}

	// Connection with no open transaction (TxDelta is nil).
	conns.Set(1, &connection.Conn{ID: 1})

	_, err := pipe.Process(points.PipelineTransactionRollback,
		payloads.TransactionRollbackPayload{ConnectionID: 1})
	if err != nil {
		t.Fatalf("Process(rollback outside tx): %v", err)
	}

	state, err := live.TableState("users")
	if err != nil {
		t.Fatalf("live.TableState: %v", err)
	}
	if got := len(state.Inserts); got != 1 {
		t.Errorf("expected live delta unchanged after ROLLBACK outside tx, got %d inserts", got)
	}
}

// TestHandlers_Rollback_DoesNotAffectOtherConnWrites verifies the core
// parallel-safety guarantee: ROLLBACK on connection A must not affect writes
// made by connection B to any table, because B's writes never entered A's
// TxDelta and were applied directly to the live delta.
func TestHandlers_Rollback_DoesNotAffectOtherConnWrites(t *testing.T) {
	pipe, conns, live := newTxPipe(t)

	// Conn A opens a transaction.
	conns.Set(1, &connection.Conn{ID: 1})
	_, err := pipe.Process(points.PipelineTransactionBegin,
		payloads.TransactionBeginPayload{ConnectionID: 1})
	if err != nil {
		t.Fatalf("Process(begin connA): %v", err)
	}

	// Conn A writes to its private TxDelta.
	connA, _ := conns.Get(1)
	if err := connA.TxDelta.ApplyInsert("txn_001_users",
		map[string]any{"id": 1, "name": "alice"}); err != nil {
		t.Fatalf("TxDelta.ApplyInsert connA: %v", err)
	}

	// Conn B (no transaction) writes directly to the live delta.
	if err := live.ApplyInsert("sel_001_users",
		map[string]any{"id": 1, "name": "bob"}); err != nil {
		t.Fatalf("ApplyInsert connB to live: %v", err)
	}

	// Conn A rolls back.
	_, err = pipe.Process(points.PipelineTransactionRollback,
		payloads.TransactionRollbackPayload{ConnectionID: 1})
	if err != nil {
		t.Fatalf("Process(rollback connA): %v", err)
	}

	// A's write must not appear in the live delta.
	stateA, err := live.TableState("txn_001_users")
	if err != nil {
		t.Fatalf("live.TableState(txn_001_users): %v", err)
	}
	if got := len(stateA.Inserts); got != 0 {
		t.Errorf("txn_001_users: expected 0 inserts after connA rollback, got %d", got)
	}

	// B's write must be completely unaffected.
	stateB, err := live.TableState("sel_001_users")
	if err != nil {
		t.Fatalf("live.TableState(sel_001_users): %v", err)
	}
	if got := len(stateB.Inserts); got != 1 {
		t.Errorf("sel_001_users: connB's write must survive connA's rollback: expected 1 insert, got %d", got)
	}
}
