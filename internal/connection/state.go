package connection

import (
	"sync"

	"github.com/AnqorDX/vdb-core/internal/delta"
)

// Conn holds per-connection framework bookkeeping for the lifetime of a single
// client session.
type Conn struct {
	ID       uint32
	User     string
	Addr     string
	Database string // current database; updated by vdb.query.received.intercept

	// TxDelta is the connection's private staging delta for the duration of an
	// open transaction. It is non-nil between BEGIN and COMMIT/ROLLBACK.
	//
	// Writes issued while TxDelta is non-nil are applied here instead of the
	// shared live delta, so they are invisible to other connections until
	// COMMIT promotes them. ROLLBACK simply nils TxDelta — the live delta is
	// never touched by in-transaction writes, so no undo work is required.
	TxDelta *delta.Delta
}

// State is the concurrency-safe store of per-connection bookkeeping keyed by
// connection ID.
type State struct {
	mu    sync.RWMutex
	store map[uint32]*Conn
}

// NewState allocates and returns an empty State ready for use.
func NewState() *State {
	return &State{store: make(map[uint32]*Conn)}
}

// Set stores or replaces the Conn for id.
func (s *State) Set(id uint32, c *Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store[id] = c
}

// Get returns the Conn for id and true if present, or nil and false if not.
func (s *State) Get(id uint32) (*Conn, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.store[id]
	return c, ok
}

// Delete removes the Conn for id. A missing id is a no-op.
func (s *State) Delete(id uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.store, id)
}

// GetDatabase returns the current database name for id, or "" if not tracked.
func (s *State) GetDatabase(id uint32) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if c, ok := s.store[id]; ok {
		return c.Database
	}
	return ""
}
