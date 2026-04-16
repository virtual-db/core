package connection_test

import (
	"fmt"
	"sync"
	"testing"

	. "github.com/AnqorDX/vdb-core/internal/connection"
)

// TestState_Set_And_Get_RoundTrip verifies that a Conn stored via Set can be
// retrieved via Get with all fields intact.
func TestState_Set_And_Get_RoundTrip(t *testing.T) {
	s := NewState()
	s.Set(1, &Conn{ID: 1, User: "alice", Addr: "127.0.0.1"})

	conn, ok := s.Get(1)
	if !ok {
		t.Fatal("expected ok==true, got false")
	}
	if conn == nil {
		t.Fatal("expected non-nil conn, got nil")
	}
	if conn.ID != 1 {
		t.Errorf("ID: want %d, got %d", 1, conn.ID)
	}
	if conn.User != "alice" {
		t.Errorf("User: want %q, got %q", "alice", conn.User)
	}
	if conn.Addr != "127.0.0.1" {
		t.Errorf("Addr: want %q, got %q", "127.0.0.1", conn.Addr)
	}
}

// TestState_Get_Unknown_ReturnsFalse verifies that Get on an ID that was never
// Set returns (nil, false).
func TestState_Get_Unknown_ReturnsFalse(t *testing.T) {
	s := NewState()

	conn, ok := s.Get(42)
	if ok {
		t.Fatal("expected ok==false for unknown ID, got true")
	}
	if conn != nil {
		t.Fatalf("expected nil conn for unknown ID, got %+v", conn)
	}
}

// TestState_Delete_RemovesEntry verifies that after Delete the entry is no
// longer present in the state.
func TestState_Delete_RemovesEntry(t *testing.T) {
	s := NewState()
	s.Set(1, &Conn{ID: 1, User: "alice"})

	s.Delete(1)

	conn, ok := s.Get(1)
	if ok {
		t.Fatal("expected ok==false after Delete, got true")
	}
	if conn != nil {
		t.Fatalf("expected nil conn after Delete, got %+v", conn)
	}
}

// TestState_Delete_UnknownID_IsNoOp verifies that calling Delete on an ID that
// was never Set does not panic or produce any error.
func TestState_Delete_UnknownID_IsNoOp(t *testing.T) {
	s := NewState()
	// Must not panic.
	s.Delete(99)
}

// TestState_GetDatabase_ReturnsCurrentDatabase verifies that GetDatabase returns
// the Database field of the tracked Conn.
func TestState_GetDatabase_ReturnsCurrentDatabase(t *testing.T) {
	s := NewState()
	s.Set(1, &Conn{ID: 1, Database: "mydb"})

	db := s.GetDatabase(1)
	if db != "mydb" {
		t.Errorf("GetDatabase: want %q, got %q", "mydb", db)
	}
}

// TestState_GetDatabase_UnknownID_ReturnsEmpty verifies that GetDatabase returns
// an empty string when the ID is not tracked.
func TestState_GetDatabase_UnknownID_ReturnsEmpty(t *testing.T) {
	s := NewState()

	db := s.GetDatabase(99)
	if db != "" {
		t.Errorf("GetDatabase for unknown ID: want %q, got %q", "", db)
	}
}

// TestState_Set_Overwrites_Existing verifies that a second Set for the same ID
// replaces the previously stored Conn.
func TestState_Set_Overwrites_Existing(t *testing.T) {
	s := NewState()
	s.Set(1, &Conn{ID: 1, User: "alice"})
	s.Set(1, &Conn{ID: 1, User: "bob"})

	conn, ok := s.Get(1)
	if !ok {
		t.Fatal("expected ok==true after overwrite, got false")
	}
	if conn.User != "bob" {
		t.Errorf("User after overwrite: want %q, got %q", "bob", conn.User)
	}
}

// TestState_Concurrent_SetAndGet_NoRace exercises concurrent Set and Get calls
// across multiple goroutines. No value assertions are made; the goal is to pass
// the race detector cleanly.
func TestState_Concurrent_SetAndGet_NoRace(t *testing.T) {
	s := NewState()

	const n = 50
	var wg sync.WaitGroup

	// 50 goroutines each Set a unique ID.
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			id := uint32(i)
			s.Set(id, &Conn{
				ID:       id,
				User:     fmt.Sprintf("user_%d", i),
				Database: fmt.Sprintf("db_%d", i),
			})
		}()
	}
	wg.Wait()

	// 50 goroutines each Get their unique ID.
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			s.Get(uint32(i))
		}()
	}
	wg.Wait()
}
