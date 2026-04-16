package payloads

// QueryReceivedPayload is the payload for the vdb.query.received pipeline.
type QueryReceivedPayload struct {
	ConnectionID uint32
	Query        string
	Database     string
}

// QueryCompletedPayload is the payload for the vdb.query.completed event.
type QueryCompletedPayload struct {
	ConnectionID uint32
	Query        string
	Database     string
	DurationMs   float64
	RowsAffected int64
	Error        string
}
