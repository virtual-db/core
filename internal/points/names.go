// Package points declares the canonical string names for all standard pipeline
// names, pipeline point full names, and event names in the vdb framework.
//
// This is an internal package. Consumers of vdb-core should use the string
// names directly — the pipeline and event names are the stable, language-agnostic
// extension points. These constants are used only by the framework's built-in
// internal handler registrations.
package points

// Pipeline name constants.
const (
	PipelineContextCreate       = "vdb.context.create"
	PipelineServerStart         = "vdb.server.start"
	PipelineServerStop          = "vdb.server.stop"
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
)

// Point full name constants — lifecycle pipelines.
const (
	// vdb.context.create points
	PointContextCreateBuildContext = "vdb.context.create.build_context"
	PointContextCreateContribute   = "vdb.context.create.contribute"
	PointContextCreateSeal         = "vdb.context.create.seal"
	PointContextCreateEmit         = "vdb.context.create.emit"

	// vdb.server.start points
	PointServerStartBuildContext = "vdb.server.start.build_context"
	PointServerStartConfigure    = "vdb.server.start.configure"
	PointServerStartLaunch       = "vdb.server.start.launch"
	PointServerStartEmit         = "vdb.server.start.emit"

	// vdb.server.stop points
	PointServerStopBuildContext = "vdb.server.stop.build_context"
	PointServerStopDrain        = "vdb.server.stop.drain"
	PointServerStopHalt         = "vdb.server.stop.halt"
	PointServerStopEmit         = "vdb.server.stop.emit"
)

// Point full name constants — connection, transaction, and query pipelines.
const (
	// vdb.connection.opened points
	PointConnectionOpenedBuildContext = "vdb.connection.opened.build_context"
	PointConnectionOpenedAccept       = "vdb.connection.opened.accept"
	PointConnectionOpenedTrack        = "vdb.connection.opened.track"
	PointConnectionOpenedEmit         = "vdb.connection.opened.emit"

	// vdb.connection.closed points
	PointConnectionClosedBuildContext = "vdb.connection.closed.build_context"
	PointConnectionClosedCleanup      = "vdb.connection.closed.cleanup"
	PointConnectionClosedRelease      = "vdb.connection.closed.release"
	PointConnectionClosedEmit         = "vdb.connection.closed.emit"

	// vdb.transaction.begin points
	PointTransactionBeginBuildContext = "vdb.transaction.begin.build_context"
	PointTransactionBeginAuthorize    = "vdb.transaction.begin.authorize"
	PointTransactionBeginEmit         = "vdb.transaction.begin.emit"

	// vdb.transaction.commit points
	PointTransactionCommitBuildContext = "vdb.transaction.commit.build_context"
	PointTransactionCommitApply        = "vdb.transaction.commit.apply"
	PointTransactionCommitEmit         = "vdb.transaction.commit.emit"

	// vdb.transaction.rollback points
	PointTransactionRollbackBuildContext = "vdb.transaction.rollback.build_context"
	PointTransactionRollbackApply        = "vdb.transaction.rollback.apply"
	PointTransactionRollbackEmit         = "vdb.transaction.rollback.emit"

	// vdb.query.received points
	PointQueryReceivedBuildContext = "vdb.query.received.build_context"
	PointQueryReceivedIntercept    = "vdb.query.received.intercept"
	PointQueryReceivedEmit         = "vdb.query.received.emit"
)

// Point full name constants — record pipelines.
const (
	// vdb.records.source points
	PointRecordsSourceBuildContext = "vdb.records.source.build_context"
	PointRecordsSourceTransform    = "vdb.records.source.transform"
	PointRecordsSourceEmit         = "vdb.records.source.emit"

	// vdb.records.merged points
	PointRecordsMergedBuildContext = "vdb.records.merged.build_context"
	PointRecordsMergedTransform    = "vdb.records.merged.transform"
	PointRecordsMergedEmit         = "vdb.records.merged.emit"

	// vdb.write.insert points
	PointWriteInsertBuildContext = "vdb.write.insert.build_context"
	PointWriteInsertApply        = "vdb.write.insert.apply"
	PointWriteInsertEmit         = "vdb.write.insert.emit"

	// vdb.write.update points
	PointWriteUpdateBuildContext = "vdb.write.update.build_context"
	PointWriteUpdateApply        = "vdb.write.update.apply"
	PointWriteUpdateEmit         = "vdb.write.update.emit"

	// vdb.write.delete points
	PointWriteDeleteBuildContext = "vdb.write.delete.build_context"
	PointWriteDeleteApply        = "vdb.write.delete.apply"
	PointWriteDeleteEmit         = "vdb.write.delete.emit"
)

// Event name constants.
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
)
