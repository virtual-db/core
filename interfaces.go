// Package core is the public API of the vdb-core module.
//
// Consumers of vdb-core should import only this package:
//
//	github.com/virtual-db/core
//
// All sub-packages under internal/ are implementation details.
// The pipeline and event bus engines are provided by github.com/AnqorDX/pipeline
// and github.com/AnqorDX/dispatch, imported only by internal/framework.
package core

// PointFunc is the handler type for all pipeline point handlers.
//
//	func(ctx any, payload any) (any, any, error)
type PointFunc func(ctx any, payload any) (any, any, error)

// EventFunc is the signature for all event subscribers.
// Returning a non-nil error is logged but does not affect other subscribers.
type EventFunc func(ctx any, payload any) error

// Config holds process-level configuration for core.New().
type Config struct {
	// PluginDir is the directory scanned one level deep for plugin manifests at
	// startup. An empty string means no plugins are loaded; this is not an error.
	PluginDir string
}

// Server is the interface the framework calls to manage the database listener.
// A value satisfying Server must be passed to App.UseDriver before App.Run is called.
type Server interface {
	// Run binds the network port and blocks until Stop is called or a fatal error occurs.
	// Port binding MUST happen inside Run, never in the adapter constructor.
	Run() error

	// Stop signals Run to return and releases the network port.
	Stop() error
}

// DriverAPI is the bridge between the database engine adapter and the framework's
// pipeline-and-event system. Implemented by the framework, not by the application
// developer or the adapter author.
type DriverAPI interface {
	ConnectionOpened(id uint32, user, addr string) error
	ConnectionClosed(id uint32, user, addr string)
	TransactionBegun(connID uint32, readOnly bool) error
	TransactionCommitted(connID uint32) error
	TransactionRolledBack(connID uint32, savepoint string)
	QueryReceived(connID uint32, query, database string) (string, error)
	QueryCompleted(connID uint32, query string, rowsAffected int64, err error)
	RecordsSource(connID uint32, table string, records []map[string]any) ([]map[string]any, error)
	RecordsMerged(connID uint32, table string, records []map[string]any) ([]map[string]any, error)
	RecordInserted(connID uint32, table string, record map[string]any) (map[string]any, error)
	RecordUpdated(connID uint32, table string, old, new map[string]any) (map[string]any, error)
	RecordDeleted(connID uint32, table string, record map[string]any) error
	SchemaLoaded(table string, columns []string, pkCol string)
	SchemaInvalidated(table string)
}
