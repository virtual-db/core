package framework_test

import (
	"fmt"
	"sync"
	"testing"

	. "github.com/AnqorDX/vdb-core/internal/framework"
)

// newSealedCtx builds a minimal GlobalContext for tests that only need a valid
// sealed context to exercise Bus/Pipeline wiring.
func newSealedCtx(t *testing.T) GlobalContext {
	t.Helper()
	var g GlobalContext
	b := NewBus(&g)
	pipe := NewPipeline(&g)
	g = SealContext(NewGlobalContextBuilder(), b, pipe)
	return g
}

// TestGlobalContextBuilder_SetGet_RoundTrip -----------------------------------

func TestGlobalContextBuilder_SetGet_RoundTrip(t *testing.T) {
	b := NewGlobalContextBuilder()

	cases := []struct {
		key string
		val any
	}{
		{"string-key", "hello"},
		{"int-key", 42},
		{"nil-key", nil},
		{"bool-key", true},
	}

	for _, c := range cases {
		b.Set(c.key, c.val)
	}

	for _, c := range cases {
		got := b.Get(c.key)
		if got != c.val {
			t.Errorf("key %q: expected %v (%T), got %v (%T)", c.key, c.val, c.val, got, got)
		}
	}
}

// TestGlobalContextBuilder_PanicsAfterSeal -----------------------------------

func TestGlobalContextBuilder_PanicsAfterSeal(t *testing.T) {
	var g GlobalContext
	bus := NewBus(&g)
	pipe := NewPipeline(&g)
	b := NewGlobalContextBuilder()
	g = SealContext(b, bus, pipe)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic after seal when calling Set, but got none")
		}
	}()

	b.Set("post-seal", "value")
}

// TestGlobalContextBuilder_GetPanicsAfterSeal --------------------------------

func TestGlobalContextBuilder_GetPanicsAfterSeal(t *testing.T) {
	var g GlobalContext
	bus := NewBus(&g)
	pipe := NewPipeline(&g)
	b := NewGlobalContextBuilder()
	g = SealContext(b, bus, pipe)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic after seal when calling Get, but got none")
		}
	}()

	b.Get("any-key")
}

// TestSealContext_ValuesPresent -----------------------------------------------

func TestSealContext_ValuesPresent(t *testing.T) {
	var g GlobalContext
	bus := NewBus(&g)
	pipe := NewPipeline(&g)

	b := NewGlobalContextBuilder()
	b.Set("db", "postgres://localhost/vdb")
	b.Set("version", 3)

	g = SealContext(b, bus, pipe)

	if got := g.Get("db"); got != "postgres://localhost/vdb" {
		t.Errorf("db: expected %q, got %v", "postgres://localhost/vdb", got)
	}
	if got := g.Get("version"); got != 3 {
		t.Errorf("version: expected 3, got %v", got)
	}
}

// TestGlobalContext_Get_MissingKey_ReturnsNil ---------------------------------

func TestGlobalContext_Get_MissingKey_ReturnsNil(t *testing.T) {
	g := newSealedCtx(t)

	if got := g.Get("totally-absent"); got != nil {
		t.Errorf("expected nil for missing key, got %v (%T)", got, got)
	}
}

// TestGlobalContext_Zero_Bus_ReturnsNoopBus -----------------------------------

func TestGlobalContext_Zero_Bus_ReturnsNoopBus(t *testing.T) {
	// Zero value — bus field is nil; Bus() must return noopBus, not nil.
	var g GlobalContext

	bus := g.Bus()
	if bus == nil {
		t.Fatal("Bus() returned nil on zero-value GlobalContext; expected a non-nil noopBus")
	}

	// Calling Emit on the noopBus must not panic.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Bus().Emit panicked on zero-value GlobalContext: %v", r)
		}
	}()
	bus.Emit("anything", nil)
}

// TestGlobalContextBuilder_SetGetConcurrentNoRace ----------------------------

func TestGlobalContextBuilder_SetGetConcurrentNoRace(t *testing.T) {
	const n = 20
	b := NewGlobalContextBuilder()

	// mu serialises access to the builder, mirroring how a startup orchestrator
	// would sequence registrations even when running concurrent workers.
	var mu sync.Mutex
	var wg sync.WaitGroup
	errs := make([]string, 0, n)
	var errMu sync.Mutex

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("concurrent-key-%d", idx)
			val := fmt.Sprintf("concurrent-val-%d", idx)

			mu.Lock()
			b.Set(key, val)
			got := b.Get(key)
			mu.Unlock()

			if got != val {
				errMu.Lock()
				errs = append(errs, fmt.Sprintf("key %q: want %q got %v", key, val, got))
				errMu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	for _, e := range errs {
		t.Error(e)
	}
}
