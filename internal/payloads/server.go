package payloads

// ServerStartPayload is the payload for the vdb.server.start pipeline.
type ServerStartPayload struct {
	ListenAddr string
	DBName     string
	TLSConfig  any
	MaxConns   int
}

// ServerStopPayload is the payload for the vdb.server.stop pipeline.
type ServerStopPayload struct {
	Reason string
}
