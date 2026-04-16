package lifecycle

import (
	"fmt"
	"log"

	"github.com/AnqorDX/vdb-core/internal/framework"
	"github.com/AnqorDX/vdb-core/internal/payloads"
	"github.com/AnqorDX/vdb-core/internal/points"
)

// Handlers owns all pipeline points for the framework's lifecycle pipelines.
type Handlers struct {
	app App
}

// New returns a Handlers ready for registration.
func New(app App) *Handlers {
	return &Handlers{app: app}
}

// Register attaches all lifecycle handlers to r.
// Points covered:
//
//	context.create.build_context  (10) → h.ContextCreateBuild
//	context.create.seal           (10) → h.ContextCreateSeal
//	server.start.build_context    (10) → h.ServerStartBuild
//	server.start.launch           (10) → h.ServerStartLaunch
//	server.stop.build_context     (10) → h.ServerStopBuild
//	server.stop.drain             (10) → h.ServerStopDrain
//	server.stop.halt              (10) → h.ServerStopHalt
func (h *Handlers) Register(r framework.Registrar) error {
	for _, reg := range []struct {
		point    string
		priority int
		fn       framework.PointFunc
	}{
		{points.PointContextCreateBuildContext, 10, h.ContextCreateBuild},
		{points.PointContextCreateSeal, 10, h.ContextCreateSeal},
		{points.PointServerStartBuildContext, 10, h.ServerStartBuild},
		{points.PointServerStartLaunch, 10, h.ServerStartLaunch},
		{points.PointServerStopBuildContext, 10, h.ServerStopBuild},
		{points.PointServerStopDrain, 10, h.ServerStopDrain},
		{points.PointServerStopHalt, 10, h.ServerStopHalt},
	} {
		if err := r.Attach(reg.point, reg.priority, reg.fn); err != nil {
			return fmt.Errorf("lifecycle: attach %q: %w", reg.point, err)
		}
	}
	return nil
}

// ContextCreateBuild creates the *framework.GlobalContextBuilder payload.
func (h *Handlers) ContextCreateBuild(ctx any, _ any) (any, any, error) {
	return ctx, framework.NewGlobalContextBuilder(), nil
}

// ContextCreateSeal seals the builder into the app's global context.
func (h *Handlers) ContextCreateSeal(ctx any, p any) (any, any, error) {
	hctx := ctx.(framework.HandlerContext)
	builder, ok := p.(*framework.GlobalContextBuilder)
	if !ok {
		return ctx, nil, fmt.Errorf("context.create.seal: unexpected payload type %T (expected *framework.GlobalContextBuilder)", p)
	}
	sealed := framework.SealContext(builder, h.app.Bus(), h.app.Pipe())
	h.app.SetGlobal(sealed)
	return hctx, sealed, nil
}

// ServerStartBuild creates the ServerStartPayload.
func (h *Handlers) ServerStartBuild(ctx any, _ any) (any, any, error) {
	return ctx, payloads.ServerStartPayload{}, nil
}

// ServerStartLaunch starts app.server.Run() in a goroutine.
func (h *Handlers) ServerStartLaunch(ctx any, p any) (any, any, error) {
	srv := h.app.GetServer()
	if srv == nil {
		return ctx, p, nil
	}
	go func() {
		h.app.ServerErrCh() <- srv.Run()
	}()
	return ctx, p, nil
}

// ServerStopBuild creates the ServerStopPayload.
func (h *Handlers) ServerStopBuild(ctx any, _ any) (any, any, error) {
	return ctx, payloads.ServerStopPayload{Reason: "graceful shutdown"}, nil
}

// ServerStopDrain shuts down all active plugin connections.
func (h *Handlers) ServerStopDrain(ctx any, p any) (any, any, error) {
	h.app.Plugins().Shutdown()
	return ctx, p, nil
}

// ServerStopHalt calls server.Stop() to close the listener.
func (h *Handlers) ServerStopHalt(ctx any, p any) (any, any, error) {
	srv := h.app.GetServer()
	if srv == nil {
		return ctx, p, nil
	}
	if err := srv.Stop(); err != nil {
		log.Printf("core: server.Stop: %v", err)
	}
	return ctx, p, nil
}
