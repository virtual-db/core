package core

import (
	"github.com/virtual-db/core/internal/framework"
	"github.com/virtual-db/core/internal/points"
)

// declarePipelines registers all 15 standard vdb.* pipelines with their ordered
// point sequences. (The delta.provide pipeline has been eliminated.)
func declarePipelines(p *framework.Pipeline) {
	p.DeclarePipeline(points.PipelineContextCreate, []string{
		points.PointContextCreateBuildContext,
		points.PointContextCreateContribute,
		points.PointContextCreateSeal,
		points.PointContextCreateEmit,
	})
	p.DeclarePipeline(points.PipelineServerStart, []string{
		points.PointServerStartBuildContext,
		points.PointServerStartConfigure,
		points.PointServerStartLaunch,
		points.PointServerStartEmit,
	})
	p.DeclarePipeline(points.PipelineServerStop, []string{
		points.PointServerStopBuildContext,
		points.PointServerStopDrain,
		points.PointServerStopHalt,
		points.PointServerStopEmit,
	})
	p.DeclarePipeline(points.PipelineConnectionOpened, []string{
		points.PointConnectionOpenedBuildContext,
		points.PointConnectionOpenedAccept,
		points.PointConnectionOpenedTrack,
		points.PointConnectionOpenedEmit,
	})
	p.DeclarePipeline(points.PipelineConnectionClosed, []string{
		points.PointConnectionClosedBuildContext,
		points.PointConnectionClosedCleanup,
		points.PointConnectionClosedRelease,
		points.PointConnectionClosedEmit,
	})
	p.DeclarePipeline(points.PipelineTransactionBegin, []string{
		points.PointTransactionBeginBuildContext,
		points.PointTransactionBeginAuthorize,
		points.PointTransactionBeginEmit,
	})
	p.DeclarePipeline(points.PipelineTransactionCommit, []string{
		points.PointTransactionCommitBuildContext,
		points.PointTransactionCommitApply,
		points.PointTransactionCommitEmit,
	})
	p.DeclarePipeline(points.PipelineTransactionRollback, []string{
		points.PointTransactionRollbackBuildContext,
		points.PointTransactionRollbackApply,
		points.PointTransactionRollbackEmit,
	})
	p.DeclarePipeline(points.PipelineQueryReceived, []string{
		points.PointQueryReceivedBuildContext,
		points.PointQueryReceivedIntercept,
		points.PointQueryReceivedEmit,
	})
	p.DeclarePipeline(points.PipelineRecordsSource, []string{
		points.PointRecordsSourceBuildContext,
		points.PointRecordsSourceTransform,
		points.PointRecordsSourceEmit,
	})
	p.DeclarePipeline(points.PipelineRecordsMerged, []string{
		points.PointRecordsMergedBuildContext,
		points.PointRecordsMergedTransform,
		points.PointRecordsMergedEmit,
	})
	p.DeclarePipeline(points.PipelineWriteInsert, []string{
		points.PointWriteInsertBuildContext,
		points.PointWriteInsertApply,
		points.PointWriteInsertEmit,
	})
	p.DeclarePipeline(points.PipelineWriteUpdate, []string{
		points.PointWriteUpdateBuildContext,
		points.PointWriteUpdateApply,
		points.PointWriteUpdateEmit,
	})
	p.DeclarePipeline(points.PipelineWriteDelete, []string{
		points.PointWriteDeleteBuildContext,
		points.PointWriteDeleteApply,
		points.PointWriteDeleteEmit,
	})
	p.DeclarePipeline(points.PipelineWriteTruncate, []string{
		points.PointWriteTruncateBuildContext,
		points.PointWriteTruncateApply,
		points.PointWriteTruncateEmit,
	})
}

// declareEvents registers all 12 standard vdb.* event names on the bus.
func declareEvents(bus *framework.Bus) {
	bus.DeclareEvent(points.EventServerStopped)
	bus.DeclareEvent(points.EventConnectionOpened)
	bus.DeclareEvent(points.EventConnectionClosed)
	bus.DeclareEvent(points.EventTransactionStarted)
	bus.DeclareEvent(points.EventTransactionCommitted)
	bus.DeclareEvent(points.EventTransactionRolledback)
	bus.DeclareEvent(points.EventQueryCompleted)
	bus.DeclareEvent(points.EventRecordInserted)
	bus.DeclareEvent(points.EventRecordUpdated)
	bus.DeclareEvent(points.EventRecordDeleted)
	bus.DeclareEvent(points.EventSchemaLoaded)
	bus.DeclareEvent(points.EventSchemaInvalidated)
	bus.DeclareEvent(points.EventTableTruncated)
}
