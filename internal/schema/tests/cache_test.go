package schema_test

import (
	"fmt"
	"sync"
	"testing"

	. "github.com/AnqorDX/vdb-core/internal/schema"
)

// TestCache_Load_And_Get_RoundTrip verifies that a table loaded into the cache
// can be retrieved with its Columns and PKCol intact.
func TestCache_Load_And_Get_RoundTrip(t *testing.T) {
	c := NewCache()
	c.Load("users", []string{"id", "name", "email"}, "id")

	entry, ok := c.Get("users")
	if !ok {
		t.Fatal("expected ok==true, got false")
	}
	if entry.PKCol != "id" {
		t.Errorf("PKCol: want %q, got %q", "id", entry.PKCol)
	}
	want := []string{"id", "name", "email"}
	if len(entry.Columns) != len(want) {
		t.Fatalf("Columns length: want %d, got %d", len(want), len(entry.Columns))
	}
	for i, col := range want {
		if entry.Columns[i] != col {
			t.Errorf("Columns[%d]: want %q, got %q", i, col, entry.Columns[i])
		}
	}
}

// TestCache_Get_UnknownTable_ReturnsFalse verifies that getting a table that was
// never loaded returns ok==false.
func TestCache_Get_UnknownTable_ReturnsFalse(t *testing.T) {
	c := NewCache()

	_, ok := c.Get("nonexistent")
	if ok {
		t.Fatal("expected ok==false for unknown table, got true")
	}
}

// TestCache_Invalidate_RemovesEntry verifies that after Invalidate the entry is
// no longer present in the cache.
func TestCache_Invalidate_RemovesEntry(t *testing.T) {
	c := NewCache()
	c.Load("orders", []string{"id", "total"}, "id")

	c.Invalidate("orders")

	_, ok := c.Get("orders")
	if ok {
		t.Fatal("expected ok==false after Invalidate, got true")
	}
}

// TestCache_Invalidate_MissingTable_IsNoOp verifies that calling Invalidate on a
// table that was never loaded does not panic or produce any error.
func TestCache_Invalidate_MissingTable_IsNoOp(t *testing.T) {
	c := NewCache()
	// Must not panic.
	c.Invalidate("ghost_table")
}

// TestCache_Load_ReplacesExisting verifies that loading a table a second time
// replaces the first entry completely.
func TestCache_Load_ReplacesExisting(t *testing.T) {
	c := NewCache()
	c.Load("products", []string{"a", "b"}, "a")
	c.Load("products", []string{"c", "d"}, "c")

	entry, ok := c.Get("products")
	if !ok {
		t.Fatal("expected ok==true after second Load, got false")
	}
	want := []string{"c", "d"}
	if len(entry.Columns) != len(want) {
		t.Fatalf("Columns length: want %d, got %d", len(want), len(entry.Columns))
	}
	for i, col := range want {
		if entry.Columns[i] != col {
			t.Errorf("Columns[%d]: want %q, got %q", i, col, entry.Columns[i])
		}
	}
	if entry.PKCol != "c" {
		t.Errorf("PKCol: want %q, got %q", "c", entry.PKCol)
	}
}

// TestCache_Load_DefensiveCopy_OnStore verifies that mutating the slice passed to
// Load after the call does not affect the stored entry.
func TestCache_Load_DefensiveCopy_OnStore(t *testing.T) {
	c := NewCache()
	columns := []string{"x", "y"}
	c.Load("items", columns, "x")

	// Mutate the original slice after Load.
	columns[0] = "MUTATED"

	entry, ok := c.Get("items")
	if !ok {
		t.Fatal("expected ok==true, got false")
	}
	if entry.Columns[0] != "x" {
		t.Errorf("defensive copy on store failed: want %q, got %q", "x", entry.Columns[0])
	}
}

// TestCache_Get_DefensiveCopy_OnRead verifies that mutating the Columns slice
// returned by Get does not affect subsequent Get calls.
func TestCache_Get_DefensiveCopy_OnRead(t *testing.T) {
	c := NewCache()
	c.Load("accounts", []string{"id", "balance"}, "id")

	first, ok := c.Get("accounts")
	if !ok {
		t.Fatal("expected ok==true, got false")
	}

	// Mutate the returned slice.
	first.Columns[0] = "MUTATED"

	second, ok := c.Get("accounts")
	if !ok {
		t.Fatal("expected ok==true on second Get, got false")
	}
	if second.Columns[0] != "id" {
		t.Errorf("defensive copy on read failed: want %q, got %q", "id", second.Columns[0])
	}
}

// TestCache_Concurrent_LoadAndGet_NoRace exercises concurrent Load and Get calls
// across multiple goroutines. No value assertions are made; the goal is to pass
// the race detector cleanly.
func TestCache_Concurrent_LoadAndGet_NoRace(t *testing.T) {
	c := NewCache()

	const n = 20
	var wg sync.WaitGroup

	// 20 goroutines each Load a unique table.
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			table := fmt.Sprintf("table_%d", i)
			cols := []string{fmt.Sprintf("col_%d_a", i), fmt.Sprintf("col_%d_b", i)}
			c.Load(table, cols, cols[0])
		}()
	}
	wg.Wait()

	// 20 goroutines each Get their unique table.
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			table := fmt.Sprintf("table_%d", i)
			c.Get(table)
		}()
	}
	wg.Wait()
}
