package payloads

// ContextCreatePayload is the payload for the vdb.context.create pipeline.
// It is nil at the start of the pipeline; the build_context handler creates
// a *framework.GlobalContextBuilder as the payload.
type ContextCreatePayload struct{}
