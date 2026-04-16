// Package schema provides the framework's in-memory schema cache.
package schema

import "sync"

// Entry holds the schema information for a single table.
type Entry struct {
	Columns []string
	PKCol   string
}

// Cache is a concurrent-safe in-memory store of per-table schema information.
// It is populated lazily by SchemaLoaded calls and cleared on SchemaInvalidated.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]Entry
}

// NewCache allocates and returns an empty Cache ready for use.
func NewCache() *Cache {
	return &Cache{
		entries: make(map[string]Entry),
	}
}

// Load stores or replaces the schema entry for table. Makes a defensive copy
// of columns before storing.
func (c *Cache) Load(table string, columns []string, pkCol string) {
	cols := make([]string, len(columns))
	copy(cols, columns)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[table] = Entry{Columns: cols, PKCol: pkCol}
}

// Get retrieves the schema entry for table. Returns (entry, true) if found,
// (Entry{}, false) otherwise. Returns a defensive copy.
func (c *Cache) Get(table string) (Entry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[table]
	if !ok {
		return Entry{}, false
	}
	cols := make([]string, len(e.Columns))
	copy(cols, e.Columns)
	return Entry{Columns: cols, PKCol: e.PKCol}, true
}

// Invalidate removes the schema entry for table. A missing table is a no-op.
func (c *Cache) Invalidate(table string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, table)
}
