package connection

import (
	"fmt"

	"github.com/AnqorDX/vdb-core/internal/framework"
	"github.com/AnqorDX/vdb-core/internal/payloads"
	"github.com/AnqorDX/vdb-core/internal/points"
)

// Handlers owns all pipeline points that interact with connection state.
type Handlers struct {
	state *State
}

// New returns a Handlers ready for registration.
func New(state *State) *Handlers {
	return &Handlers{state: state}
}

// Register attaches all connection handlers to r.
func (h *Handlers) Register(r framework.Registrar) error {
	for _, reg := range []struct {
		point    string
		priority int
		fn       framework.PointFunc
	}{
		{points.PointConnectionOpenedBuildContext, 10, framework.BuildContext},
		{points.PointConnectionOpenedTrack, 10, h.TrackOpened},
		{points.PointConnectionClosedBuildContext, 10, framework.BuildContext},
		{points.PointConnectionClosedRelease, 10, h.ReleaseOnClose},
		{points.PointQueryReceivedBuildContext, 10, framework.BuildContext},
		{points.PointQueryReceivedIntercept, 10, h.UpdateDatabase},
	} {
		if err := r.Attach(reg.point, reg.priority, reg.fn); err != nil {
			return fmt.Errorf("connection: attach %q: %w", reg.point, err)
		}
	}
	return nil
}

// TrackOpened stores a new Conn entry when a connection is opened.
func (h *Handlers) TrackOpened(ctx any, p any) (any, any, error) {
	payload, ok := p.(payloads.ConnectionOpenedPayload)
	if !ok {
		return ctx, nil, fmt.Errorf("connection.opened.track: unexpected payload type %T", p)
	}
	h.state.Set(payload.ConnectionID, &Conn{
		ID:   payload.ConnectionID,
		User: payload.User,
		Addr: payload.Address,
	})
	return ctx, payload, nil
}

// ReleaseOnClose removes the Conn entry when a connection is closed.
func (h *Handlers) ReleaseOnClose(ctx any, p any) (any, any, error) {
	payload, ok := p.(payloads.ConnectionClosedPayload)
	if !ok {
		return ctx, nil, fmt.Errorf("connection.closed.release: unexpected payload type %T", p)
	}
	h.state.Delete(payload.ConnectionID)
	return ctx, payload, nil
}

// UpdateDatabase updates the tracked database name for a connection.
func (h *Handlers) UpdateDatabase(ctx any, p any) (any, any, error) {
	payload, ok := p.(payloads.QueryReceivedPayload)
	if !ok {
		return ctx, nil, fmt.Errorf("query.received.intercept: unexpected payload type %T", p)
	}
	if payload.Database != "" {
		if c, found := h.state.Get(payload.ConnectionID); found {
			c.Database = payload.Database
		}
	}
	return ctx, payload, nil
}
