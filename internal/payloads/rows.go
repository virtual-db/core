package payloads

// RecordsSourcePayload is the payload for the vdb.records.source pipeline.
type RecordsSourcePayload struct {
	ConnectionID uint32
	Table        string
	Records      []map[string]any
}

// RecordsMergedPayload is the payload for the vdb.records.merged pipeline.
type RecordsMergedPayload struct {
	ConnectionID uint32
	Table        string
	Records      []map[string]any
}
