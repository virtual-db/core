// Package plugin — conn.go: pluginConn struct and low-level I/O transport.
package plugin

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"

	"github.com/AnqorDX/vdb-core/internal/framework"
)

// pluginConn wraps a raw net.Conn with JSON-RPC framing and pending-request
// tracking. It is shared between the transport layer (this file) and the
// application-protocol layer (protocol.go).
type pluginConn struct {
	conn    net.Conn
	scanner *bufio.Scanner

	wrMu sync.Mutex

	pending map[int64]chan rpcResponse
	pendMu  sync.Mutex

	nextID atomic.Int64

	inst *pluginInstance
	bus  *framework.Bus
}

// newPluginConn constructs a pluginConn around an accepted raw connection.
func newPluginConn(conn net.Conn, inst *pluginInstance, bus *framework.Bus) *pluginConn {
	return &pluginConn{
		conn:    conn,
		scanner: bufio.NewScanner(conn),
		pending: make(map[int64]chan rpcResponse),
		inst:    inst,
		bus:     bus,
	}
}

// start launches the inbound reader goroutine (defined in protocol.go).
func (c *pluginConn) start() {
	go c.readLoop()
}

// readNextLine returns the next newline-delimited byte slice from the stream.
// The returned slice is safe to use after the next call to readNextLine.
func (c *pluginConn) readNextLine() ([]byte, error) {
	if c.scanner.Scan() {
		line := c.scanner.Bytes()
		out := make([]byte, len(line))
		copy(out, line)
		return out, nil
	}
	if err := c.scanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

// writeMessage JSON-encodes v and writes it followed by a newline.
// It is safe for concurrent use.
func (c *pluginConn) writeMessage(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("plugin %q: marshal message: %w", c.inst.id, err)
	}
	c.wrMu.Lock()
	defer c.wrMu.Unlock()
	if _, err := c.conn.Write(data); err != nil {
		return fmt.Errorf("plugin %q: write: %w", c.inst.id, err)
	}
	if _, err := c.conn.Write([]byte{'\n'}); err != nil {
		return fmt.Errorf("plugin %q: write newline: %w", c.inst.id, err)
	}
	return nil
}

// sendRequest sends a JSON-RPC request with an auto-incremented ID and blocks
// until the matching response arrives on the pending channel.
func (c *pluginConn) sendRequest(method string, params any) (json.RawMessage, error) {
	id := c.nextID.Add(1)

	ch := make(chan rpcResponse, 1)

	c.pendMu.Lock()
	c.pending[id] = ch
	c.pendMu.Unlock()

	req := rpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      id,
	}
	if err := c.writeMessage(req); err != nil {
		c.pendMu.Lock()
		delete(c.pending, id)
		c.pendMu.Unlock()
		return nil, err
	}

	resp := <-ch
	if resp.Error != nil {
		return nil, fmt.Errorf("plugin %q rpc error %d: %s", c.inst.id, resp.Error.Code, resp.Error.Message)
	}
	return resp.Result, nil
}

// sendNotification sends a JSON-RPC notification (no ID, no response expected).
func (c *pluginConn) sendNotification(method string, params any) error {
	n := rpcNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	return c.writeMessage(n)
}

// Close closes the underlying network connection.
func (c *pluginConn) Close() error {
	return c.conn.Close()
}
