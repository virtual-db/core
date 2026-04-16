package payloads

// WriteInsertPayload is the payload for the vdb.write.insert pipeline.
type WriteInsertPayload struct {
	ConnectionID uint32
	Table        string
	Record       map[string]any
}

// WriteUpdatePayload is the payload for the vdb.write.update pipeline.
type WriteUpdatePayload struct {
	ConnectionID uint32
	Table        string
	OldRecord    map[string]any
	NewRecord    map[string]any
}

// WriteDeletePayload is the payload for the vdb.write.delete pipeline.
type WriteDeletePayload struct {
	ConnectionID uint32
	Table        string
	Record       map[string]any
}
