// Package plugin — protocol.go
// Application-level protocol methods: the read loop and all outbound
// request/notification helpers that speak the higher-level plugin protocol.
package plugin

import (
	"encoding/json"
	"fmt"
	"log"
)

// readLoop drains the scanner until EOF or error, dispatching every inbound
// message to the appropriate handler.
func (c *pluginConn) readLoop() {
	for c.scanner.Scan() {
		line := c.scanner.Bytes()

		var msg rpcResponse
		if err := json.Unmarshal(line, &msg); err != nil {
			log.Printf("plugin %q: malformed inbound message: %v", c.inst.id, err)
			continue
		}

		switch {
		case msg.Method == "emit_event":
			c.handleEmitEvent(msg)

		case msg.ID != nil && msg.Method == "":
			id := *msg.ID
			c.pendMu.Lock()
			ch, ok := c.pending[id]
			if ok {
				delete(c.pending, id)
			}
			c.pendMu.Unlock()
			if ok {
				ch <- msg
			} else {
				log.Printf("plugin %q: response for unknown id %d; discarding", c.inst.id, id)
			}

		default:
			log.Printf("plugin %q: unrecognised inbound message: method=%q hasID=%v",
				c.inst.id, msg.Method, msg.ID != nil)
		}
	}

	if err := c.scanner.Err(); err != nil {
		log.Printf("plugin %q: read loop exited with error: %v", c.inst.id, err)
	}
}

// handleEmitEvent processes an emit_event request from the plugin: acks it,
// validates the event is declared, then forwards it onto the bus.
func (c *pluginConn) handleEmitEvent(msg rpcResponse) {
	ack := struct {
		JSONRPC string   `json:"jsonrpc"`
		Result  struct{} `json:"result"`
		ID      int64    `json:"id"`
	}{
		JSONRPC: "2.0",
		ID:      *msg.ID,
	}
	if err := c.writeMessage(ack); err != nil {
		log.Printf("plugin %q: failed to ack emit_event: %v", c.inst.id, err)
	}

	var p emitEventParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		log.Printf("plugin %q: malformed emit_event params: %v", c.inst.id, err)
		return
	}

	allowed := false
	for _, name := range c.inst.declare.EventDeclarations {
		if name == p.Event {
			allowed = true
			break
		}
	}
	if !allowed {
		log.Printf("plugin %q: emit_event for undeclared event %q; ignoring", c.inst.id, p.Event)
		return
	}

	c.bus.Emit(p.Event, p.Payload)
}

// sendHandlePipelinePoint sends a handle_pipeline_point request to the plugin
// and returns the decoded result payload.
func (c *pluginConn) sendHandlePipelinePoint(point string, payload any) (any, error) {
	raw, err := c.sendRequest("handle_pipeline_point", pipelineParams{
		Point:   point,
		Payload: payload,
	})
	if err != nil {
		return nil, err
	}
	var r pipelineResult
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("plugin %q: unmarshal pipeline result: %w", c.inst.id, err)
	}
	var out any
	if err := json.Unmarshal(r.Payload, &out); err != nil {
		return nil, fmt.Errorf("plugin %q: unmarshal payload: %w", c.inst.id, err)
	}
	return out, nil
}

// sendHandleEvent delivers an event notification to the plugin (fire-and-forget).
func (c *pluginConn) sendHandleEvent(event string, payload any) error {
	return c.sendNotification("handle_event", eventParams{
		Event:   event,
		Payload: payload,
	})
}

// sendShutdown sends a shutdown request and waits for the plugin's ack.
func (c *pluginConn) sendShutdown() error {
	_, err := c.sendRequest("shutdown", struct{}{})
	return err
}
