package framework_test

import (
	"errors"
	"testing"

	. "github.com/AnqorDX/vdb-core/internal/framework"
)

func newTestPipe(t *testing.T) *Pipeline {
	t.Helper()
	var g GlobalContext
	pipe := NewPipeline(&g)
	bus := NewBus(&g)
	g = SealContext(NewGlobalContextBuilder(), bus, pipe)
	return pipe
}

// TestPipeline_ZeroValue_Process_NoOp verifies that the zero-value Pipeline is
// safe to call and returns (nil, nil) without panicking.
func TestPipeline_ZeroValue_Process_NoOp(t *testing.T) {
	var p Pipeline
	result, err := p.Process("anything", "payload")
	if result != nil {
		t.Fatalf("expected nil result, got %v", result)
	}
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

// TestPipeline_DeclareAndAttach_ProcessInvokesHandler verifies that a declared
// pipeline with one attached handler actually calls the handler on Process.
func TestPipeline_DeclareAndAttach_ProcessInvokesHandler(t *testing.T) {
	pipe := newTestPipe(t)
	pipe.DeclarePipeline("myPipeline", []string{"myPipeline.step"})

	called := false
	pipe.MustAttach("myPipeline.step", 10, func(ctx any, payload any) (any, any, error) {
		called = true
		return ctx, payload, nil
	})

	_, err := pipe.Process("myPipeline", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("handler was not invoked")
	}
}

// TestPipeline_Attach_UndeclaredPoint_ReturnsError verifies that Attach returns a
// non-nil error when the point has not been declared via DeclarePipeline.
func TestPipeline_Attach_UndeclaredPoint_ReturnsError(t *testing.T) {
	pipe := newTestPipe(t)

	err := pipe.Attach("totally.unknown.point", 10, func(ctx any, payload any) (any, any, error) {
		return ctx, payload, nil
	})
	if err == nil {
		t.Fatal("expected an error for an undeclared point, got nil")
	}
}

// TestPipeline_MustAttach_UndeclaredPoint_Panics verifies that MustAttach panics
// when the target point was never declared.
func TestPipeline_MustAttach_UndeclaredPoint_Panics(t *testing.T) {
	pipe := newTestPipe(t)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected a panic for undeclared point, but none occurred")
		}
	}()

	pipe.MustAttach("totally.unknown.point", 10, func(ctx any, payload any) (any, any, error) {
		return ctx, payload, nil
	})
}

// TestPipeline_MultipleHandlers_AscendingPriorityOrder attaches three handlers at
// priorities 30, 10, and 20 (out of order) and asserts they are invoked in
// ascending priority order: 10 → 20 → 30.
func TestPipeline_MultipleHandlers_AscendingPriorityOrder(t *testing.T) {
	pipe := newTestPipe(t)
	pipe.DeclarePipeline("ordered", []string{"ordered.step"})

	var callOrder []int

	// Deliberately registered out of order to exercise priority sorting.
	pipe.MustAttach("ordered.step", 30, func(ctx any, payload any) (any, any, error) {
		callOrder = append(callOrder, 30)
		return ctx, payload, nil
	})
	pipe.MustAttach("ordered.step", 10, func(ctx any, payload any) (any, any, error) {
		callOrder = append(callOrder, 10)
		return ctx, payload, nil
	})
	pipe.MustAttach("ordered.step", 20, func(ctx any, payload any) (any, any, error) {
		callOrder = append(callOrder, 20)
		return ctx, payload, nil
	})

	_, err := pipe.Process("ordered", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []int{10, 20, 30}
	if len(callOrder) != len(want) {
		t.Fatalf("expected %d handler invocations, got %d", len(want), len(callOrder))
	}
	for i, priority := range want {
		if callOrder[i] != priority {
			t.Errorf("call[%d]: expected priority %d, got %d", i, priority, callOrder[i])
		}
	}
}

// TestPipeline_HandlerError_AbortsChain verifies that when the first handler
// returns an error, the second handler is never called and the error is surfaced.
func TestPipeline_HandlerError_AbortsChain(t *testing.T) {
	pipe := newTestPipe(t)
	pipe.DeclarePipeline("aborting", []string{"aborting.step"})

	sentinel := errors.New("abort sentinel")
	secondCalled := false

	pipe.MustAttach("aborting.step", 10, func(ctx any, payload any) (any, any, error) {
		return ctx, payload, sentinel
	})
	pipe.MustAttach("aborting.step", 20, func(ctx any, payload any) (any, any, error) {
		secondCalled = true
		return ctx, payload, nil
	})

	_, err := pipe.Process("aborting", nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if secondCalled {
		t.Fatal("second handler must not be invoked after the first handler returns an error")
	}
}

// TestBuildContext_StampsNonZeroCorrelationID verifies that BuildContext sets a
// non-empty CorrelationID on the returned HandlerContext. When the input
// CorrelationID is zero-value, Root must equal ID (new causal root) and Parent
// must be empty.
func TestBuildContext_StampsNonZeroCorrelationID(t *testing.T) {
	input := HandlerContext{} // zero-value CorrelationID

	outCtxAny, outPayloadAny, err := BuildContext(input, "the-payload")
	if err != nil {
		t.Fatalf("unexpected error from BuildContext: %v", err)
	}

	outCtx, ok := outCtxAny.(HandlerContext)
	if !ok {
		t.Fatalf("expected HandlerContext from BuildContext, got %T", outCtxAny)
	}

	// Payload must be passed through unchanged.
	if outPayloadAny != "the-payload" {
		t.Errorf("payload changed: want %q, got %v", "the-payload", outPayloadAny)
	}

	cid := outCtx.CorrelationID
	if cid.ID == "" {
		t.Error("CorrelationID.ID must not be empty after BuildContext")
	}
	if cid.Root == "" {
		t.Error("CorrelationID.Root must not be empty after BuildContext")
	}
	// Zero parent → this run becomes the root of a new causal chain.
	if cid.Root != cid.ID {
		t.Errorf("expected Root == ID for zero-value parent, got Root=%q ID=%q", cid.Root, cid.ID)
	}
	if cid.Parent != "" {
		t.Errorf("expected empty Parent for zero-value parent, got %q", cid.Parent)
	}
}
