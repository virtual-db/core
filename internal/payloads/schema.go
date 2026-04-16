package payloads

// SchemaLoadedPayload is the payload emitted with the vdb.schema.loaded event.
type SchemaLoadedPayload struct {
	Table   string
	Columns []string
	PKCol   string
}

// SchemaInvalidatedPayload is the payload emitted with the vdb.schema.invalidated event.
type SchemaInvalidatedPayload struct {
	Table string
}
