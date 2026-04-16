package driverapi

import (
	"log"

	"github.com/AnqorDX/vdb-core/internal/connection"
	"github.com/AnqorDX/vdb-core/internal/framework"
	"github.com/AnqorDX/vdb-core/internal/payloads"
	"github.com/AnqorDX/vdb-core/internal/points"
	"github.com/AnqorDX/vdb-core/internal/schema"
)

// Impl is the framework's concrete implementation of core.DriverAPI.
// It dispatches each driver callback to the appropriate pipeline or event.
type Impl struct {
	pipe   *framework.Pipeline
	bus    *framework.Bus
	conns  *connection.State
	schema *schema.Cache
}

// New constructs an Impl with the four framework subsystems it needs.
func New(
	pipe *framework.Pipeline,
	bus *framework.Bus,
	conns *connection.State,
	schema *schema.Cache,
) *Impl {
	return &Impl{pipe: pipe, bus: bus, conns: conns, schema: schema}
}

func (d *Impl) ConnectionOpened(id uint32, user, addr string) error {
	payload := payloads.ConnectionOpenedPayload{
		ConnectionID: id,
		User:         user,
		Address:      addr,
	}
	_, err := d.pipe.Process(points.PipelineConnectionOpened, payload)
	return err
}

func (d *Impl) ConnectionClosed(id uint32, user, addr string) {
	payload := payloads.ConnectionClosedPayload{
		ConnectionID: id,
		User:         user,
		Address:      addr,
	}
	if _, err := d.pipe.Process(points.PipelineConnectionClosed, payload); err != nil {
		log.Printf("driverapi: ConnectionClosed pipeline error for conn %d: %v", id, err)
	}
}

func (d *Impl) TransactionBegun(connID uint32, readOnly bool) error {
	payload := payloads.TransactionBeginPayload{
		ConnectionID: connID,
		ReadOnly:     readOnly,
	}
	_, err := d.pipe.Process(points.PipelineTransactionBegin, payload)
	return err
}

func (d *Impl) TransactionCommitted(connID uint32) error {
	payload := payloads.TransactionCommitPayload{
		ConnectionID: connID,
	}
	_, err := d.pipe.Process(points.PipelineTransactionCommit, payload)
	return err
}

func (d *Impl) TransactionRolledBack(connID uint32, savepoint string) {
	payload := payloads.TransactionRollbackPayload{
		ConnectionID: connID,
		Savepoint:    savepoint,
	}
	if _, err := d.pipe.Process(points.PipelineTransactionRollback, payload); err != nil {
		log.Printf("driverapi: TransactionRolledBack pipeline error for conn %d: %v", connID, err)
	}
}

func (d *Impl) QueryReceived(connID uint32, query, database string) (string, error) {
	payload := payloads.QueryReceivedPayload{
		ConnectionID: connID,
		Query:        query,
		Database:     database,
	}
	result, err := d.pipe.Process(points.PipelineQueryReceived, payload)
	if err != nil {
		return "", err
	}
	return validateQueryResult(result)
}

func (d *Impl) QueryCompleted(connID uint32, query string, rowsAffected int64, execErr error) {
	errStr := ""
	if execErr != nil {
		errStr = execErr.Error()
	}

	database := d.conns.GetDatabase(connID)

	payload := payloads.QueryCompletedPayload{
		ConnectionID: connID,
		Query:        query,
		Database:     database,
		DurationMs:   0,
		RowsAffected: rowsAffected,
		Error:        errStr,
	}
	d.bus.Emit(points.EventQueryCompleted, payload)
}

func (d *Impl) RecordsSource(connID uint32, table string, records []map[string]any) ([]map[string]any, error) {
	payload := payloads.RecordsSourcePayload{
		ConnectionID: connID,
		Table:        table,
		Records:      records,
	}
	result, err := d.pipe.Process(points.PipelineRecordsSource, payload)
	if err != nil {
		return nil, err
	}
	return validateRecordsSourceResult(result)
}

func (d *Impl) RecordsMerged(connID uint32, table string, records []map[string]any) ([]map[string]any, error) {
	payload := payloads.RecordsMergedPayload{
		ConnectionID: connID,
		Table:        table,
		Records:      records,
	}
	result, err := d.pipe.Process(points.PipelineRecordsMerged, payload)
	if err != nil {
		log.Printf("driverapi: RecordsMerged pipeline error for conn %d table %q: %v", connID, table, err)
		return records, nil
	}
	out, err := validateRecordsMergedResult(result)
	if err != nil {
		log.Printf("driverapi: RecordsMerged result validation error for conn %d table %q: %v", connID, table, err)
		return records, nil
	}
	return out, nil
}

func (d *Impl) RecordInserted(connID uint32, table string, record map[string]any) (map[string]any, error) {
	payload := payloads.WriteInsertPayload{
		ConnectionID: connID,
		Table:        table,
		Record:       record,
	}
	result, err := d.pipe.Process(points.PipelineWriteInsert, payload)
	if err != nil {
		return nil, err
	}
	return validateWriteInsertResult(result)
}

func (d *Impl) RecordUpdated(connID uint32, table string, old, new map[string]any) (map[string]any, error) {
	payload := payloads.WriteUpdatePayload{
		ConnectionID: connID,
		Table:        table,
		OldRecord:    old,
		NewRecord:    new,
	}
	result, err := d.pipe.Process(points.PipelineWriteUpdate, payload)
	if err != nil {
		return nil, err
	}
	return validateWriteUpdateResult(result)
}

func (d *Impl) RecordDeleted(connID uint32, table string, record map[string]any) error {
	payload := payloads.WriteDeletePayload{
		ConnectionID: connID,
		Table:        table,
		Record:       record,
	}
	_, err := d.pipe.Process(points.PipelineWriteDelete, payload)
	return err
}

func (d *Impl) SchemaLoaded(table string, columns []string, pkCol string) {
	d.schema.Load(table, columns, pkCol)
	d.bus.Emit(points.EventSchemaLoaded, payloads.SchemaLoadedPayload{
		Table:   table,
		Columns: columns,
		PKCol:   pkCol,
	})
}

func (d *Impl) SchemaInvalidated(table string) {
	d.schema.Invalidate(table)
	d.bus.Emit(points.EventSchemaInvalidated, payloads.SchemaInvalidatedPayload{
		Table: table,
	})
}
