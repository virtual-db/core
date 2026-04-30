// Package emit_test exercises the emit.Handlers using a real framework.Pipeline
// and framework.Bus. No stubs, no export_test.go.
package emit_test

import (
	"sync"
	"testing"
	"time"

	. "github.com/virtual-db/core/internal/emit"
	"github.com/virtual-db/core/internal/framework"
	"github.com/virtual-db/core/internal/payloads"
	"github.com/virtual-db/core/internal/points"
)

// newEmitEnv builds a sealed env with all 9 emit pipelines declared and the
// emit handlers registered. Returns the live pipeline and bus.
func newEmitEnv(t *testing.T) (*framework.Pipeline, *framework.Bus) {
	t.Helper()

	var global framework.GlobalContext
	pipe := framework.NewPipeline(&global)
	bus := framework.NewBus(&global)
	global = framework.SealContext(framework.NewGlobalContextBuilder(), bus, pipe)

	// Declare all 9 pipelines that have emit points.
	pipe.DeclarePipeline(points.PipelineServerStop, []string{
		points.PointServerStopBuildContext, points.PointServerStopDrain, points.PointServerStopHalt, points.PointServerStopEmit,
	})
	pipe.DeclarePipeline(points.PipelineConnectionOpened, []string{
		points.PointConnectionOpenedBuildContext, points.PointConnectionOpenedAccept, points.PointConnectionOpenedTrack, points.PointConnectionOpenedEmit,
	})
	pipe.DeclarePipeline(points.PipelineConnectionClosed, []string{
		points.PointConnectionClosedBuildContext, points.PointConnectionClosedCleanup, points.PointConnectionClosedRelease, points.PointConnectionClosedEmit,
	})
	pipe.DeclarePipeline(points.PipelineTransactionBegin, []string{
		points.PointTransactionBeginBuildContext, points.PointTransactionBeginAuthorize, points.PointTransactionBeginEmit,
	})
	pipe.DeclarePipeline(points.PipelineTransactionCommit, []string{
		points.PointTransactionCommitBuildContext, points.PointTransactionCommitApply, points.PointTransactionCommitEmit,
	})
	pipe.DeclarePipeline(points.PipelineTransactionRollback, []string{
		points.PointTransactionRollbackBuildContext, points.PointTransactionRollbackApply, points.PointTransactionRollbackEmit,
	})
	pipe.DeclarePipeline(points.PipelineWriteInsert, []string{
		points.PointWriteInsertBuildContext, points.PointWriteInsertApply, points.PointWriteInsertEmit,
	})
	pipe.DeclarePipeline(points.PipelineWriteUpdate, []string{
		points.PointWriteUpdateBuildContext, points.PointWriteUpdateApply, points.PointWriteUpdateEmit,
	})
	pipe.DeclarePipeline(points.PipelineWriteDelete, []string{
		points.PointWriteDeleteBuildContext, points.PointWriteDeleteApply, points.PointWriteDeleteEmit,
	})
	pipe.DeclarePipeline(points.PipelineWriteTruncate, []string{
		points.PointWriteTruncateBuildContext, points.PointWriteTruncateApply, points.PointWriteTruncateEmit,
	})

	// Declare all bus events the emit handlers fire.
	bus.DeclareEvent(points.EventServerStopped)
	bus.DeclareEvent(points.EventConnectionOpened)
	bus.DeclareEvent(points.EventConnectionClosed)
	bus.DeclareEvent(points.EventTransactionStarted)
	bus.DeclareEvent(points.EventTransactionCommitted)
	bus.DeclareEvent(points.EventTransactionRolledback)
	bus.DeclareEvent(points.EventRecordInserted)
	bus.DeclareEvent(points.EventRecordUpdated)
	bus.DeclareEvent(points.EventRecordDeleted)
	bus.DeclareEvent(points.EventTableTruncated)

	h := New()
	if err := h.Register(pipe); err != nil {
		t.Fatalf("emit.Register: %v", err)
	}
	return pipe, bus
}

// waitOrTimeout waits for wg to reach zero, or fails the test after 200 ms.
// The bus delivers events asynchronously, so callers must not check results
// immediately after pipe.Process.
func waitOrTimeout(t *testing.T, wg *sync.WaitGroup) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout: expected bus event was not delivered within 200ms")
	}
}

// ---------------------------------------------------------------------------
// Server stop
// ---------------------------------------------------------------------------

func TestHandlers_ServerStop_EmitsServerStopped(t *testing.T) {
	pipe, bus := newEmitEnv(t)

	var wg sync.WaitGroup
	var received bool

	if err := bus.Subscribe(points.EventServerStopped, func(_ any, p any) error {
		if _, ok := p.(payloads.ServerStopPayload); !ok {
			t.Errorf("EventServerStopped: expected payloads.ServerStopPayload, got %T", p)
		}
		received = true
		wg.Done()
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	wg.Add(1)
	if _, err := pipe.Process(points.PipelineServerStop, payloads.ServerStopPayload{Reason: "test"}); err != nil {
		t.Fatalf("Process: %v", err)
	}

	waitOrTimeout(t, &wg)
	if !received {
		t.Error("EventServerStopped subscriber was not called")
	}
}

// ---------------------------------------------------------------------------
// Connection opened
// ---------------------------------------------------------------------------

func TestHandlers_ConnectionOpened_EmitsEvent(t *testing.T) {
	pipe, bus := newEmitEnv(t)

	var wg sync.WaitGroup
	var received bool

	if err := bus.Subscribe(points.EventConnectionOpened, func(_ any, p any) error {
		if _, ok := p.(payloads.ConnectionOpenedPayload); !ok {
			t.Errorf("EventConnectionOpened: expected payloads.ConnectionOpenedPayload, got %T", p)
		}
		received = true
		wg.Done()
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	wg.Add(1)
	if _, err := pipe.Process(points.PipelineConnectionOpened, payloads.ConnectionOpenedPayload{
		ConnectionID: 1,
		User:         "alice",
		Address:      "127.0.0.1",
	}); err != nil {
		t.Fatalf("Process: %v", err)
	}

	waitOrTimeout(t, &wg)
	if !received {
		t.Error("EventConnectionOpened subscriber was not called")
	}
}

// ---------------------------------------------------------------------------
// Connection closed
// ---------------------------------------------------------------------------

func TestHandlers_ConnectionClosed_EmitsEvent(t *testing.T) {
	pipe, bus := newEmitEnv(t)

	var wg sync.WaitGroup
	var received bool

	if err := bus.Subscribe(points.EventConnectionClosed, func(_ any, p any) error {
		if _, ok := p.(payloads.ConnectionClosedPayload); !ok {
			t.Errorf("EventConnectionClosed: expected payloads.ConnectionClosedPayload, got %T", p)
		}
		received = true
		wg.Done()
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	wg.Add(1)
	if _, err := pipe.Process(points.PipelineConnectionClosed, payloads.ConnectionClosedPayload{ConnectionID: 1}); err != nil {
		t.Fatalf("Process: %v", err)
	}

	waitOrTimeout(t, &wg)
	if !received {
		t.Error("EventConnectionClosed subscriber was not called")
	}
}

// ---------------------------------------------------------------------------
// Transaction begin
// ---------------------------------------------------------------------------

func TestHandlers_TransactionBegin_EmitsEvent(t *testing.T) {
	pipe, bus := newEmitEnv(t)

	var wg sync.WaitGroup
	var received bool

	if err := bus.Subscribe(points.EventTransactionStarted, func(_ any, p any) error {
		if _, ok := p.(payloads.TransactionBeginPayload); !ok {
			t.Errorf("EventTransactionStarted: expected payloads.TransactionBeginPayload, got %T", p)
		}
		received = true
		wg.Done()
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	wg.Add(1)
	if _, err := pipe.Process(points.PipelineTransactionBegin, payloads.TransactionBeginPayload{ConnectionID: 1}); err != nil {
		t.Fatalf("Process: %v", err)
	}

	waitOrTimeout(t, &wg)
	if !received {
		t.Error("EventTransactionStarted subscriber was not called")
	}
}

// ---------------------------------------------------------------------------
// Transaction commit
// ---------------------------------------------------------------------------

func TestHandlers_TransactionCommit_EmitsEvent(t *testing.T) {
	pipe, bus := newEmitEnv(t)

	var wg sync.WaitGroup
	var received bool

	if err := bus.Subscribe(points.EventTransactionCommitted, func(_ any, p any) error {
		if _, ok := p.(payloads.TransactionCommitPayload); !ok {
			t.Errorf("EventTransactionCommitted: expected payloads.TransactionCommitPayload, got %T", p)
		}
		received = true
		wg.Done()
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	wg.Add(1)
	if _, err := pipe.Process(points.PipelineTransactionCommit, payloads.TransactionCommitPayload{ConnectionID: 1}); err != nil {
		t.Fatalf("Process: %v", err)
	}

	waitOrTimeout(t, &wg)
	if !received {
		t.Error("EventTransactionCommitted subscriber was not called")
	}
}

// ---------------------------------------------------------------------------
// Transaction rollback
// ---------------------------------------------------------------------------

func TestHandlers_TransactionRollback_EmitsEvent(t *testing.T) {
	pipe, bus := newEmitEnv(t)

	var wg sync.WaitGroup
	var received bool

	if err := bus.Subscribe(points.EventTransactionRolledback, func(_ any, p any) error {
		if _, ok := p.(payloads.TransactionRollbackPayload); !ok {
			t.Errorf("EventTransactionRolledback: expected payloads.TransactionRollbackPayload, got %T", p)
		}
		received = true
		wg.Done()
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	wg.Add(1)
	if _, err := pipe.Process(points.PipelineTransactionRollback, payloads.TransactionRollbackPayload{ConnectionID: 1}); err != nil {
		t.Fatalf("Process: %v", err)
	}

	waitOrTimeout(t, &wg)
	if !received {
		t.Error("EventTransactionRolledback subscriber was not called")
	}
}

// ---------------------------------------------------------------------------
// Write insert
// ---------------------------------------------------------------------------

func TestHandlers_WriteInsert_EmitsEvent(t *testing.T) {
	pipe, bus := newEmitEnv(t)

	var wg sync.WaitGroup
	var received bool

	if err := bus.Subscribe(points.EventRecordInserted, func(_ any, p any) error {
		if _, ok := p.(payloads.WriteInsertPayload); !ok {
			t.Errorf("EventRecordInserted: expected payloads.WriteInsertPayload, got %T", p)
		}
		received = true
		wg.Done()
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	wg.Add(1)
	if _, err := pipe.Process(points.PipelineWriteInsert, payloads.WriteInsertPayload{
		Table:  "users",
		Record: map[string]any{"id": 1},
	}); err != nil {
		t.Fatalf("Process: %v", err)
	}

	waitOrTimeout(t, &wg)
	if !received {
		t.Error("EventRecordInserted subscriber was not called")
	}
}

// ---------------------------------------------------------------------------
// Write update
// ---------------------------------------------------------------------------

func TestHandlers_WriteUpdate_EmitsEvent(t *testing.T) {
	pipe, bus := newEmitEnv(t)

	var wg sync.WaitGroup
	var received bool

	if err := bus.Subscribe(points.EventRecordUpdated, func(_ any, p any) error {
		if _, ok := p.(payloads.WriteUpdatePayload); !ok {
			t.Errorf("EventRecordUpdated: expected payloads.WriteUpdatePayload, got %T", p)
		}
		received = true
		wg.Done()
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	wg.Add(1)
	if _, err := pipe.Process(points.PipelineWriteUpdate, payloads.WriteUpdatePayload{
		Table:     "users",
		OldRecord: map[string]any{"id": 1},
		NewRecord: map[string]any{"id": 1, "name": "b"},
	}); err != nil {
		t.Fatalf("Process: %v", err)
	}

	waitOrTimeout(t, &wg)
	if !received {
		t.Error("EventRecordUpdated subscriber was not called")
	}
}

// ---------------------------------------------------------------------------
// Write delete
// ---------------------------------------------------------------------------

func TestHandlers_WriteDelete_EmitsEvent(t *testing.T) {
	pipe, bus := newEmitEnv(t)

	var wg sync.WaitGroup
	var received bool

	if err := bus.Subscribe(points.EventRecordDeleted, func(_ any, p any) error {
		if _, ok := p.(payloads.WriteDeletePayload); !ok {
			t.Errorf("EventRecordDeleted: expected payloads.WriteDeletePayload, got %T", p)
		}
		received = true
		wg.Done()
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	wg.Add(1)
	if _, err := pipe.Process(points.PipelineWriteDelete, payloads.WriteDeletePayload{
		Table:  "users",
		Record: map[string]any{"id": 1},
	}); err != nil {
		t.Fatalf("Process: %v", err)
	}

	waitOrTimeout(t, &wg)
	if !received {
		t.Error("EventRecordDeleted subscriber was not called")
	}
}
