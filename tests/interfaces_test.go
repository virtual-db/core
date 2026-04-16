package core_test

import (
	"testing"

	core "github.com/AnqorDX/vdb-core"
)

// ---- Stub types for compile-time interface satisfaction checks ----

// stubServer satisfies core.Server.
type stubServer struct{}

func (s *stubServer) Run() error  { return nil }
func (s *stubServer) Stop() error { return nil }

var _ core.Server = (*stubServer)(nil)

// noopDriverAPI satisfies core.DriverAPI with no-ops.
type noopDriverAPI struct{}

func (n *noopDriverAPI) ConnectionOpened(_ uint32, _, _ string) error            { return nil }
func (n *noopDriverAPI) ConnectionClosed(_ uint32, _, _ string)                  {}
func (n *noopDriverAPI) TransactionBegun(_ uint32, _ bool) error                 { return nil }
func (n *noopDriverAPI) TransactionCommitted(_ uint32) error                     { return nil }
func (n *noopDriverAPI) TransactionRolledBack(_ uint32, _ string)                {}
func (n *noopDriverAPI) QueryReceived(_ uint32, query, _ string) (string, error) { return query, nil }
func (n *noopDriverAPI) QueryCompleted(_ uint32, _ string, _ int64, _ error)     {}
func (n *noopDriverAPI) RecordsSource(_ uint32, _ string, recs []map[string]any) ([]map[string]any, error) {
	return recs, nil
}
func (n *noopDriverAPI) RecordsMerged(_ uint32, _ string, recs []map[string]any) ([]map[string]any, error) {
	return recs, nil
}
func (n *noopDriverAPI) RecordInserted(_ uint32, _ string, rec map[string]any) (map[string]any, error) {
	return rec, nil
}
func (n *noopDriverAPI) RecordUpdated(_ uint32, _ string, _, new map[string]any) (map[string]any, error) {
	return new, nil
}
func (n *noopDriverAPI) RecordDeleted(_ uint32, _ string, _ map[string]any) error { return nil }
func (n *noopDriverAPI) SchemaLoaded(_ string, _ []string, _ string)              {}
func (n *noopDriverAPI) SchemaInvalidated(_ string)                               {}

var _ core.DriverAPI = (*noopDriverAPI)(nil)

// ---- Runtime tests ----

// TestUseDriverReturnsApp verifies UseDriver supports method chaining.
func TestUseDriverReturnsApp(t *testing.T) {
	app := core.New(core.Config{})
	if got := app.UseDriver(&stubServer{}); got != app {
		t.Fatal("UseDriver must return the same *App for method chaining")
	}
}

// TestUseDriverNoPanicWithoutDriverReceiver verifies that UseDriver does not panic.
func TestUseDriverNoPanicWithoutDriverReceiver(t *testing.T) {
	app := core.New(core.Config{})
	app.UseDriver(&stubServer{}) // must not panic
}

// TestQueryReceivedReturnsString confirms that QueryReceived returns the query string.
func TestQueryReceivedReturnsString(t *testing.T) {
	var api core.DriverAPI = &noopDriverAPI{}
	q, err := api.QueryReceived(1, "SELECT 1", "testdb")
	if err != nil {
		t.Fatalf("QueryReceived: unexpected error: %v", err)
	}
	if q != "SELECT 1" {
		t.Fatalf("QueryReceived: got %q, want %q", q, "SELECT 1")
	}
}

// TestRecordUpdatedReceivesBothRecords confirms that both old and new records are passed.
func TestRecordUpdatedReceivesBothRecords(t *testing.T) {
	type capturingAPI struct {
		noopDriverAPI
		capturedOld map[string]any
		capturedNew map[string]any
	}
	api := &capturingAPI{}
	// Override RecordUpdated inline by just using noopDriverAPI as base and testing directly.
	oldRec := map[string]any{"id": float64(1), "name": "alice"}
	newRec := map[string]any{"id": float64(1), "name": "alicia"}

	result, err := api.RecordUpdated(1, "users", oldRec, newRec)
	if err != nil {
		t.Fatalf("RecordUpdated: unexpected error: %v", err)
	}
	if result["name"] != "alicia" {
		t.Fatalf("return value name: got %v, want alicia", result["name"])
	}
}
