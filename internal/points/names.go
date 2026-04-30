// Package points declares the canonical string names for all standard pipeline
// names, pipeline point full names, and event names in the vdb framework.
//
// This is an internal package. Consumers of vdb-core should use the string
// names directly — the pipeline and event names are the stable, language-agnostic
// extension points. These constants are used only by the framework's built-in
// internal handler registrations.
package points

// ── Pipeline name constants ───────────────────────────────────────────────
//
// Pipelines are divided into two groups:
//
//   Host-only pipelines run during the startup or shutdown sequence, before or
//   after out-of-process plugins are connected. Out-of-process plugin handlers
//   declared in a `declare` notification will never be invoked on these
//   pipelines. In-process consumers that embed core as a library may still
//   attach handlers via app.Attach — that is the intended and supported use.
//
//   Plugin-accessible pipelines run during normal operation, after all plugins
//   have completed their startup handshake. Both in-process and out-of-process
//   handlers may attach to these pipelines.

// Host-only pipeline names.
//
// vdb.context.create runs synchronously in App.Run() before ConnectAll is
// called. No plugin handler can ever be registered in time to observe it.
// The contribute point is the extension point for in-process host code that
// needs to inject values into the process-wide GlobalContext at startup.
//
// vdb.server.stop runs during App.Stop(). Its drain point (priority 10) calls
// plugin.Manager.Shutdown(), which sends every connected plugin a shutdown RPC
// and waits for their processes to exit. Plugin cleanup on shutdown is handled
// by that RPC protocol, not by pipeline hooks. All points in this pipeline are
// therefore host-only.
const (
	PipelineContextCreate = "vdb.context.create"
	PipelineServerStop    = "vdb.server.stop"
)

// Plugin-accessible pipeline names.
const (
	PipelineServerStart         = "vdb.server.start"
	PipelineConnectionOpened    = "vdb.connection.opened"
	PipelineConnectionClosed    = "vdb.connection.closed"
	PipelineTransactionBegin    = "vdb.transaction.begin"
	PipelineTransactionCommit   = "vdb.transaction.commit"
	PipelineTransactionRollback = "vdb.transaction.rollback"
	PipelineQueryReceived       = "vdb.query.received"
	PipelineRecordsSource       = "vdb.records.source"
	PipelineRecordsMerged       = "vdb.records.merged"
	PipelineWriteInsert         = "vdb.write.insert"
	PipelineWriteUpdate         = "vdb.write.update"
	PipelineWriteDelete         = "vdb.write.delete"
	PipelineWriteTruncate       = "vdb.write.truncate"
)

// ── Point full name constants — host-only lifecycle pipelines ─────────────
//
// These points belong to PipelineContextCreate and PipelineServerStop.
// See the pipeline-level comments above for why these are host-only.

// vdb.context.create points.
//
// Host-only. Runs before plugins connect. The contribute point is the
// in-process extension point; seal and emit are pure framework internals.
const (
	PointContextCreateBuildContext = "vdb.context.create.build_context"
	PointContextCreateContribute   = "vdb.context.create.contribute"
	PointContextCreateSeal         = "vdb.context.create.seal"
	PointContextCreateEmit         = "vdb.context.create.emit"
)

// vdb.server.stop points.
//
// Host-only. Plugin cleanup on shutdown is handled by the shutdown RPC sent
// at the drain point, not by plugin pipeline handlers. All points in this
// pipeline are internal to the framework shutdown sequence.
const (
	PointServerStopBuildContext = "vdb.server.stop.build_context"
	PointServerStopDrain        = "vdb.server.stop.drain"
	PointServerStopHalt         = "vdb.server.stop.halt"
	PointServerStopEmit         = "vdb.server.stop.emit"
)

// ── Point full name constants — plugin-accessible lifecycle pipeline ───────

// vdb.server.start points.
const (
	PointServerStartBuildContext = "vdb.server.start.build_context"
	PointServerStartConfigure    = "vdb.server.start.configure"
	PointServerStartLaunch       = "vdb.server.start.launch"
	PointServerStartEmit         = "vdb.server.start.emit"
)

// ── Point full name constants — plugin-accessible operational pipelines ────

// vdb.connection.opened points.
const (
	PointConnectionOpenedBuildContext = "vdb.connection.opened.build_context"
	PointConnectionOpenedAccept       = "vdb.connection.opened.accept"
	PointConnectionOpenedTrack        = "vdb.connection.opened.track"
	PointConnectionOpenedEmit         = "vdb.connection.opened.emit"
)

// vdb.connection.closed points.
const (
	PointConnectionClosedBuildContext = "vdb.connection.closed.build_context"
	PointConnectionClosedCleanup      = "vdb.connection.closed.cleanup"
	PointConnectionClosedRelease      = "vdb.connection.closed.release"
	PointConnectionClosedEmit         = "vdb.connection.closed.emit"
)

// vdb.transaction.begin points.
const (
	PointTransactionBeginBuildContext = "vdb.transaction.begin.build_context"
	PointTransactionBeginAuthorize    = "vdb.transaction.begin.authorize"
	PointTransactionBeginEmit         = "vdb.transaction.begin.emit"
)

// vdb.transaction.commit points.
const (
	PointTransactionCommitBuildContext = "vdb.transaction.commit.build_context"
	PointTransactionCommitApply        = "vdb.transaction.commit.apply"
	PointTransactionCommitEmit         = "vdb.transaction.commit.emit"
)

// vdb.transaction.rollback points.
const (
	PointTransactionRollbackBuildContext = "vdb.transaction.rollback.build_context"
	PointTransactionRollbackApply        = "vdb.transaction.rollback.apply"
	PointTransactionRollbackEmit         = "vdb.transaction.rollback.emit"
)

// vdb.query.received points.
const (
	PointQueryReceivedBuildContext = "vdb.query.received.build_context"
	PointQueryReceivedIntercept    = "vdb.query.received.intercept"
	PointQueryReceivedEmit         = "vdb.query.received.emit"
)

// vdb.records.source points.
const (
	PointRecordsSourceBuildContext = "vdb.records.source.build_context"
	PointRecordsSourceTransform    = "vdb.records.source.transform"
	PointRecordsSourceEmit         = "vdb.records.source.emit"
)

// vdb.records.merged points.
const (
	PointRecordsMergedBuildContext = "vdb.records.merged.build_context"
	PointRecordsMergedTransform    = "vdb.records.merged.transform"
	PointRecordsMergedEmit         = "vdb.records.merged.emit"
)

// vdb.write.insert points.
const (
	PointWriteInsertBuildContext = "vdb.write.insert.build_context"
	PointWriteInsertApply        = "vdb.write.insert.apply"
	PointWriteInsertEmit         = "vdb.write.insert.emit"
)

// vdb.write.update points.
const (
	PointWriteUpdateBuildContext = "vdb.write.update.build_context"
	PointWriteUpdateApply        = "vdb.write.update.apply"
	PointWriteUpdateEmit         = "vdb.write.update.emit"
)

// vdb.write.delete points.
const (
	PointWriteDeleteBuildContext = "vdb.write.delete.build_context"
	PointWriteDeleteApply        = "vdb.write.delete.apply"
	PointWriteDeleteEmit         = "vdb.write.delete.emit"
)

// vdb.write.truncate points.
const (
	PointWriteTruncateBuildContext = "vdb.write.truncate.build_context"
	PointWriteTruncateApply        = "vdb.write.truncate.apply"
	PointWriteTruncateEmit         = "vdb.write.truncate.emit"
)

// ── Event name constants ──────────────────────────────────────────────────
//
// All events are observable by both in-process and out-of-process subscribers.
// vdb.server.stopped is emitted at the end of the vdb.server.stop pipeline;
// because that pipeline is host-only and plugin processes are terminated before
// the emit point runs, out-of-process plugins will not receive this event in
// practice. All other events fire during normal operation and are fully
// accessible to plugins.
const (
	EventServerStopped         = "vdb.server.stopped"
	EventConnectionOpened      = "vdb.connection.opened"
	EventConnectionClosed      = "vdb.connection.closed"
	EventTransactionStarted    = "vdb.transaction.started"
	EventTransactionCommitted  = "vdb.transaction.committed"
	EventTransactionRolledback = "vdb.transaction.rolledback"
	EventQueryCompleted        = "vdb.query.completed"
	EventRecordInserted        = "vdb.record.inserted"
	EventRecordUpdated         = "vdb.record.updated"
	EventRecordDeleted         = "vdb.record.deleted"
	EventSchemaLoaded          = "vdb.schema.loaded"
	EventSchemaInvalidated     = "vdb.schema.invalidated"
	EventTableTruncated        = "vdb.table.truncated"
)
