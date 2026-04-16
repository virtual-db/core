// Package plugin — declare.go
package plugin

import (
	"encoding/json"
	"fmt"
	"log"
	"net"

	"github.com/AnqorDX/vdb-core/internal/framework"
)

// readDeclare reads the opening "declare" notification from a freshly connected
// plugin, registers all of its pipeline handlers and event subscriptions into
// bus and pipe, and starts the inbound reader goroutine.  It returns the fully
// initialised *pluginConn ready for use.
func (m *Manager) readDeclare(inst *pluginInstance, rawConn net.Conn, bus *framework.Bus, pipe *framework.Pipeline) (*pluginConn, error) {
	pc := newPluginConn(rawConn, inst, bus)

	line, err := pc.readNextLine()
	if err != nil {
		return nil, fmt.Errorf("reading declare notification: %w", err)
	}

	var envelope struct {
		JSONRPC string          `json:"jsonrpc"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(line, &envelope); err != nil {
		return nil, fmt.Errorf("malformed declare message: %w", err)
	}
	if envelope.Method != "declare" {
		return nil, fmt.Errorf("expected declare notification, got method %q", envelope.Method)
	}

	var params DeclareParams
	if err := json.Unmarshal(envelope.Params, &params); err != nil {
		return nil, fmt.Errorf("malformed declare params: %w", err)
	}
	inst.declare = params

	// 1. Declare plugin-owned pipelines.
	for _, pd := range params.PipelineDeclarations {
		if len(pd.Points) == 0 {
			log.Printf("plugin %q: pipeline %q has no points; skipping declaration", inst.id, pd.Name)
			continue
		}
		pipe.DeclarePipeline(pd.Name, pd.Points)
	}

	// 2. Declare plugin-owned events.
	for _, event := range params.EventDeclarations {
		bus.DeclareEvent(event)
	}

	// 3. Register pipeline handler adapters.
	for _, h := range params.PipelineHandlers {
		h := h
		if err := pipe.Attach(h.Point, h.Priority, func(ctx any, p any) (any, any, error) {
			result, err := pc.sendHandlePipelinePoint(h.Point, p)
			return ctx, result, err
		}); err != nil {
			log.Printf("plugin %q: cannot register handler at point %q: %v", inst.id, h.Point, err)
		}
	}

	// 4. Register event subscription adapters.
	for _, event := range params.EventSubscriptions {
		event := event
		if err := bus.Subscribe(event, func(_ any, p any) error {
			if err := pc.sendHandleEvent(event, p); err != nil {
				log.Printf("plugin %q: handle_event %q failed: %v", inst.id, event, err)
			}
			return nil
		}); err != nil {
			log.Printf("plugin %q: cannot subscribe to event %q: %v", inst.id, event, err)
		}
	}

	// 5. Start the inbound reader goroutine.
	pc.start()

	return pc, nil
}
