package payloads

// ConnectionOpenedPayload is the payload for the vdb.connection.opened pipeline.
type ConnectionOpenedPayload struct {
	ConnectionID uint32
	User         string
	Address      string
}

// ConnectionClosedPayload is the payload for the vdb.connection.closed pipeline.
type ConnectionClosedPayload struct {
	ConnectionID uint32
	User         string
	Address      string
}
