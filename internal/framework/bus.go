package framework

import "github.com/AnqorDX/dispatch"

// EventFunc is the signature for all event subscribers.
// Returning a non-nil error is logged but does not affect other subscribers
// or the publisher (fire-and-forget delivery semantics).
type EventFunc func(ctx any, payload any) error

// EventBus is the event-dispatch surface exposed to handlers via ctx.Global.Bus().
type EventBus interface {
	Emit(name string, payload any, ctx ...HandlerContext)
}

// noopBus is a no-op EventBus returned by GlobalContext.Bus() on the zero value.
type noopBus struct{}

func (noopBus) Emit(_ string, _ any, _ ...HandlerContext) {}

// Bus is the VDB Core event bus abstraction. It is the sole owner of the
// underlying dispatch.EventBus and the only type in the internal framework that
// interacts with the dispatch library directly.
type Bus struct {
	eb     *dispatch.EventBus
	global *GlobalContext
}

// NewBus constructs a Bus. global must be a pointer to the App's global field so
// that Bus.Emit always reads the post-seal GlobalContext value.
func NewBus(global *GlobalContext) *Bus {
	return &Bus{eb: dispatch.NewEventBus(), global: global}
}

// DeclareEvent registers name as a known event on the underlying bus.
func (b *Bus) DeclareEvent(name string) {
	b.eb.DeclareEvent(name)
}

// Subscribe attaches fn to name. Returns an error if name was not declared.
func (b *Bus) Subscribe(name string, fn EventFunc) error {
	return b.eb.Subscribe(name, dispatch.EventFunc(fn))
}

// Emit dispatches name to all subscribers. ctx is optional; when omitted a
// HandlerContext carrying the current sealed GlobalContext is injected.
func (b *Bus) Emit(name string, payload any, ctx ...HandlerContext) {
	emitCtx := HandlerContext{Global: *b.global}
	if len(ctx) > 0 {
		emitCtx = ctx[0]
	}
	b.eb.Emit(name, emitCtx, payload)
}
