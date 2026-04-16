package framework_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	. "github.com/AnqorDX/vdb-core/internal/framework"
)

func newTestBus(t *testing.T) *Bus {
	t.Helper()
	var g GlobalContext
	b := NewBus(&g)
	pipe := NewPipeline(&g)
	g = SealContext(NewGlobalContextBuilder(), b, pipe)
	return b
}

// TestBus_DeclareSubscribeEmit_InvokesSubscriber verifies the basic
// declare → subscribe → emit flow actually calls the subscriber.
// Because dispatch.EventBus fires each subscriber in its own goroutine
// (fire-and-forget), the test gates on a WaitGroup before asserting.
func TestBus_DeclareSubscribeEmit_InvokesSubscriber(t *testing.T) {
	b := newTestBus(t)

	b.DeclareEvent("user.created")

	var wg sync.WaitGroup
	wg.Add(1)

	var called int32
	var receivedPayload atomic.Value

	if err := b.Subscribe("user.created", func(ctx any, payload any) error {
		defer wg.Done()
		atomic.StoreInt32(&called, 1)
		receivedPayload.Store(payload)
		return nil
	}); err != nil {
		t.Fatalf("unexpected Subscribe error: %v", err)
	}

	b.Emit("user.created", "alice")
	wg.Wait()

	if atomic.LoadInt32(&called) != 1 {
		t.Fatal("expected subscriber to be called, but it was not")
	}
	if got := receivedPayload.Load(); got != "alice" {
		t.Fatalf("expected payload %q, got %v", "alice", got)
	}
}

// TestBus_Subscribe_UndeclaredEvent_ReturnsError verifies that subscribing
// to an event that was never declared returns a non-nil error.
func TestBus_Subscribe_UndeclaredEvent_ReturnsError(t *testing.T) {
	b := newTestBus(t)

	err := b.Subscribe("ghost.event", func(ctx any, payload any) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected an error when subscribing to an undeclared event, got nil")
	}
}

// TestBus_Emit_UndeclaredEvent_NoOp verifies that emitting an undeclared event
// does not panic — the bus simply logs and returns.
func TestBus_Emit_UndeclaredEvent_NoOp(t *testing.T) {
	b := newTestBus(t)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Emit of undeclared event panicked: %v", r)
		}
	}()

	b.Emit("never.declared", "payload")
}

// TestBus_MultipleSubscribers_AllReceiveEmission verifies that every subscriber
// registered for an event receives the emission.
func TestBus_MultipleSubscribers_AllReceiveEmission(t *testing.T) {
	b := newTestBus(t)

	b.DeclareEvent("order.placed")

	const n = 5
	var counts [n]int32

	var wg sync.WaitGroup
	wg.Add(n)

	for i := range counts {
		idx := i
		if err := b.Subscribe("order.placed", func(ctx any, payload any) error {
			defer wg.Done()
			atomic.AddInt32(&counts[idx], 1)
			return nil
		}); err != nil {
			t.Fatalf("subscriber %d: unexpected Subscribe error: %v", idx, err)
		}
	}

	b.Emit("order.placed", struct{}{})
	wg.Wait()

	for i, c := range counts {
		if c != 1 {
			t.Errorf("subscriber %d: expected 1 call, got %d", i, c)
		}
	}
}

// TestBus_SubscriberError_DoesNotPreventOtherSubscribers verifies that when one
// subscriber returns an error the other subscribers still run to completion.
func TestBus_SubscriberError_DoesNotPreventOtherSubscribers(t *testing.T) {
	b := newTestBus(t)

	b.DeclareEvent("job.started")

	// All three subscribers run (even the erroring one), so we wait for all three.
	var wg sync.WaitGroup
	wg.Add(3)

	var firstCalls, thirdCalls int32

	if err := b.Subscribe("job.started", func(ctx any, payload any) error {
		defer wg.Done()
		atomic.AddInt32(&firstCalls, 1)
		return nil
	}); err != nil {
		t.Fatalf("subscriber 1 registration: %v", err)
	}

	// Intentionally returns an error to verify it does not block the others.
	if err := b.Subscribe("job.started", func(ctx any, payload any) error {
		defer wg.Done()
		return errors.New("intentional subscriber error")
	}); err != nil {
		t.Fatalf("subscriber 2 registration: %v", err)
	}

	if err := b.Subscribe("job.started", func(ctx any, payload any) error {
		defer wg.Done()
		atomic.AddInt32(&thirdCalls, 1)
		return nil
	}); err != nil {
		t.Fatalf("subscriber 3 registration: %v", err)
	}

	b.Emit("job.started", nil)
	wg.Wait()

	if firstCalls != 1 {
		t.Errorf("first subscriber: expected 1 call, got %d", firstCalls)
	}
	if thirdCalls != 1 {
		t.Errorf("third subscriber: expected 1 call, got %d", thirdCalls)
	}
}

// TestBus_Emit_ConcurrentNoRace hammers Emit from multiple goroutines to
// exercise the race detector. It pre-adds the exact expected number of
// subscriber completions to a WaitGroup and reads the aggregate counter only
// after all subscriber goroutines have finished.
func TestBus_Emit_ConcurrentNoRace(t *testing.T) {
	b := newTestBus(t)

	b.DeclareEvent("ping")

	const (
		numSubscribers    = 4
		goroutines        = 8
		emitsPerGoroutine = 50
	)

	totalExpected := goroutines * emitsPerGoroutine * numSubscribers

	// Pre-add so that every subscriber goroutine finds a matching Add already
	// in place regardless of scheduling order.
	var subWg sync.WaitGroup
	subWg.Add(totalExpected)

	var total int64
	for i := 0; i < numSubscribers; i++ {
		if err := b.Subscribe("ping", func(ctx any, payload any) error {
			defer subWg.Done()
			atomic.AddInt64(&total, 1)
			return nil
		}); err != nil {
			t.Fatalf("subscribe: %v", err)
		}
	}

	var emitWg sync.WaitGroup
	emitWg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer emitWg.Done()
			for j := 0; j < emitsPerGoroutine; j++ {
				b.Emit("ping", j)
			}
		}()
	}

	// Wait for all Emit calls to return, then for every subscriber goroutine
	// to finish before reading the counter.
	emitWg.Wait()
	subWg.Wait()

	if got := atomic.LoadInt64(&total); got != int64(totalExpected) {
		t.Errorf("expected %d total subscriber invocations, got %d", totalExpected, got)
	}
}
