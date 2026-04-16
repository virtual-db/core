// Package plugin — wire message types for the JSON-RPC plugin protocol.
package plugin

import "encoding/json"

// ── Outbound (host → plugin) ──────────────────────────────────────────────

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
	ID      int64  `json:"id"`
}

type rpcNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

// ── Inbound (plugin → host) ───────────────────────────────────────────────

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method,omitempty"`
	ID      *int64          `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ── Typed parameter / result payloads ────────────────────────────────────

type pipelineParams struct {
	Point   string `json:"point"`
	Payload any    `json:"payload"`
}

type pipelineResult struct {
	Payload json.RawMessage `json:"payload"`
}

type eventParams struct {
	Event   string `json:"event"`
	Payload any    `json:"payload"`
}

type emitEventParams struct {
	Event   string          `json:"event"`
	Payload json.RawMessage `json:"payload"`
}
