package driverapi_test

import (
	"testing"

	. "github.com/AnqorDX/vdb-core/internal/driverapi"
	"github.com/AnqorDX/vdb-core/internal/points"
)

// compile-time check: Impl is reachable via the dot import.
var _ *Impl

// ---------------------------------------------------------------------------
// SchemaLoaded
// ---------------------------------------------------------------------------

// TestSchemaLoaded_PopulatesCacheBeforeEvent verifies that SchemaLoaded
// populates the schema cache BEFORE the EventSchemaLoaded event fires, so
// any subscriber can already read the fresh entry.
func TestSchemaLoaded_PopulatesCacheBeforeEvent(t *testing.T) {
	impl, _, bus, _, sch := newTestImpl(t)

	done := make(chan struct{})
	var cacheWasLoaded bool

	bus.DeclareEvent(points.EventSchemaLoaded)
	if err := bus.Subscribe(points.EventSchemaLoaded, func(_ any, _ any) error {
		_, ok := sch.Get("users")
		cacheWasLoaded = ok
		close(done)
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	impl.SchemaLoaded("users", []string{"id", "name"}, "id")

	waitDone(t, done, "EventSchemaLoaded to fire")

	if !cacheWasLoaded {
		t.Fatal("expected cache to be populated before EventSchemaLoaded fired, but it was not")
	}
}

// TestSchemaLoaded_CacheHasCorrectValues verifies that after SchemaLoaded the
// cache entry has the right columns and primary-key column.
func TestSchemaLoaded_CacheHasCorrectValues(t *testing.T) {
	impl, _, _, _, sch := newTestImpl(t)

	impl.SchemaLoaded("products", []string{"sku", "price"}, "sku")

	entry, ok := sch.Get("products")
	if !ok {
		t.Fatal("expected cache entry for 'products', got nothing")
	}
	if entry.PKCol != "sku" {
		t.Fatalf("expected PKCol %q, got %q", "sku", entry.PKCol)
	}
	if len(entry.Columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(entry.Columns))
	}
}

// ---------------------------------------------------------------------------
// SchemaInvalidated
// ---------------------------------------------------------------------------

// TestSchemaInvalidated_ClearsCacheBeforeEvent verifies that SchemaInvalidated
// removes the cache entry BEFORE the EventSchemaInvalidated event fires, so
// any subscriber sees the cache already cleared.
func TestSchemaInvalidated_ClearsCacheBeforeEvent(t *testing.T) {
	impl, _, bus, _, sch := newTestImpl(t)

	// Pre-populate the cache so there is something to invalidate.
	sch.Load("users", []string{"id"}, "id")

	done := make(chan struct{})
	var cacheWasCleared bool

	bus.DeclareEvent(points.EventSchemaInvalidated)
	if err := bus.Subscribe(points.EventSchemaInvalidated, func(_ any, _ any) error {
		_, ok := sch.Get("users")
		cacheWasCleared = !ok
		close(done)
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	impl.SchemaInvalidated("users")

	waitDone(t, done, "EventSchemaInvalidated to fire")

	if !cacheWasCleared {
		t.Fatal("expected cache to be cleared before EventSchemaInvalidated fired, but it was not")
	}
}

// TestSchemaInvalidated_CacheIsEmpty verifies that after SchemaInvalidated the
// cache entry for the table is gone.
func TestSchemaInvalidated_CacheIsEmpty(t *testing.T) {
	impl, _, _, _, sch := newTestImpl(t)

	impl.SchemaLoaded("t", []string{"a"}, "a")

	// Sanity-check: entry is present before invalidation.
	if _, ok := sch.Get("t"); !ok {
		t.Fatal("precondition failed: expected cache entry after SchemaLoaded")
	}

	impl.SchemaInvalidated("t")

	if _, ok := sch.Get("t"); ok {
		t.Fatal("expected cache entry to be removed after SchemaInvalidated, but it is still present")
	}
}
