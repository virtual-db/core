package framework

// CorrelationID carries the causal-chain identifiers for a single pipeline run
// or event emission. It is stamped onto every HandlerContext and should be
// forwarded — with appropriate field updates — whenever a handler triggers a
// downstream pipeline run or event emission.
type CorrelationID struct {
	// Root is the ID of the pipeline run that started this causal chain.
	Root string

	// Parent is the ID of the immediate cause of this run.
	Parent string

	// ID is the unique identifier for this specific pipeline run or event emission.
	ID string
}

// GlobalContextBuilder is the mutable face of GlobalContext, used only during
// the context-creation pipeline at startup.
type GlobalContextBuilder struct {
	values map[string]any
}

// NewGlobalContextBuilder creates a fresh, writable builder.
func NewGlobalContextBuilder() *GlobalContextBuilder {
	return &GlobalContextBuilder{values: make(map[string]any)}
}

// Set stores val under key in the builder.
// Panics if called after the builder has been sealed.
func (b *GlobalContextBuilder) Set(key string, val any) {
	if b.values == nil {
		panic("framework: GlobalContextBuilder used after sealing")
	}
	b.values[key] = val
}

// Get retrieves the value stored under key.
// Panics if called after the builder has been sealed.
func (b *GlobalContextBuilder) Get(key string) any {
	if b.values == nil {
		panic("framework: GlobalContextBuilder used after sealing")
	}
	return b.values[key]
}

// GlobalContext is the sealed, process-wide shared context. It is immutable
// after startup and safe to read from any goroutine without synchronisation.
type GlobalContext struct {
	values map[string]any
	bus    *Bus
	pipe   *Pipeline
}

// Get retrieves the value stored under key, or nil if not set.
func (g GlobalContext) Get(key string) any {
	return g.values[key]
}

// Bus returns the EventBus for this context. Returns a no-op bus on the zero value.
func (g GlobalContext) Bus() EventBus {
	if g.bus == nil {
		return noopBus{}
	}
	return g.bus
}

// Pipeline returns the Pipeline surface for this context.
func (g GlobalContext) Pipeline() Pipeline {
	if g.pipe == nil {
		return Pipeline{}
	}
	return *g.pipe
}

// SealContext copies the builder's values into an immutable GlobalContext and
// renders the builder inert.
func SealContext(b *GlobalContextBuilder, bus *Bus, pipe *Pipeline) GlobalContext {
	snapshot := make(map[string]any, len(b.values))
	for k, v := range b.values {
		snapshot[k] = v
	}
	b.values = nil
	return GlobalContext{values: snapshot, bus: bus, pipe: pipe}
}

// HandlerContext is passed by value as the first argument to every PointFunc
// and EventFunc. It is constructed fresh for each pipeline processing run and
// must not be stored beyond the duration of a single handler call.
type HandlerContext struct {
	// Global holds the sealed, process-wide context. Immutable after startup.
	Global GlobalContext

	// CorrelationID identifies this specific run and its position in the causal chain.
	CorrelationID CorrelationID
}
