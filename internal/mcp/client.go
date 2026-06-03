// Package mcp is a minimal Model-Context-Protocol stdio client.
//
// It spawns an MCP server as a subprocess (e.g. askdba/mysql-mcp-server in
// its default stdio mode), speaks JSON-RPC 2.0 over the subprocess's
// stdin/stdout, and exposes the high-level tool-calling surface mysqlweb's
// chat orchestrator needs: Initialize, CallTool, AddConnection,
// RemoveConnection.
//
// The MCP spec calls for newline-delimited JSON-RPC messages over stdio.
// askdba follows this convention: each request is a single JSON object
// on its own line; responses come back the same way.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
)

// Client owns a long-running MCP subprocess and serialises JSON-RPC traffic
// over its stdio. Safe for concurrent CallTool / AddConnection / Remove from
// multiple chat sessions.
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser

	writeMu sync.Mutex
	nextID  atomic.Int64

	pendMu  sync.Mutex
	pending map[int64]chan *response

	closed atomic.Bool
}

type request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Spawn launches the MCP server as a subprocess and completes the
// initialize → notifications/initialized handshake before returning.
//
// command + args: how to invoke the MCP server, e.g. command="npx",
// args=["-y", "@askdba/mcp-server-mysql"], or command="/usr/local/bin/mcp-server-mysql"
// args=[].
//
// env: full env for the child. Caller is responsible for forwarding parent
// env where appropriate (e.g. PATH) plus any MCP-specific vars
// (MYSQL_MCP_EXTENDED=1 for askdba's add_connection support).
func Spawn(ctx context.Context, command string, args []string, env []string) (*Client, error) {
	if command == "" {
		return nil, errors.New("mcp: empty command")
	}
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Env = env
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("mcp stdout pipe: %w", err)
	}
	// stderr is left attached to the parent so the operator sees crashes.
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mcp start: %w", err)
	}
	c := &Client{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		pending: map[int64]chan *response{},
	}
	go c.readLoop()

	// Per MCP spec: initialize then send notifications/initialized.
	initCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if _, err := c.call(initCtx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "mysqlweb", "version": "1.0"},
	}); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("mcp initialize: %w", err)
	}
	if err := c.notify("notifications/initialized", nil); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("mcp initialized notification: %w", err)
	}
	return c, nil
}

func (c *Client) readLoop() {
	sc := bufio.NewScanner(c.stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var resp response
		if err := json.Unmarshal(line, &resp); err != nil {
			// Probably a server-initiated notification or non-JSON debug line.
			continue
		}
		if resp.ID == 0 {
			continue // notifications: nothing waiting for them
		}
		c.pendMu.Lock()
		ch, ok := c.pending[resp.ID]
		if ok {
			delete(c.pending, resp.ID)
		}
		c.pendMu.Unlock()
		if ok {
			ch <- &resp
			close(ch)
		}
	}
	// Reader exited. Wake every pending caller with an error.
	c.closed.Store(true)
	c.pendMu.Lock()
	for id, ch := range c.pending {
		delete(c.pending, id)
		close(ch)
	}
	c.pendMu.Unlock()
}

func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if c.closed.Load() {
		return nil, errors.New("mcp: client closed")
	}
	id := c.nextID.Add(1)
	ch := make(chan *response, 1)
	c.pendMu.Lock()
	c.pending[id] = ch
	c.pendMu.Unlock()

	req := request{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	data, err := json.Marshal(req)
	if err != nil {
		c.pendMu.Lock()
		delete(c.pending, id)
		c.pendMu.Unlock()
		return nil, err
	}
	data = append(data, '\n')

	c.writeMu.Lock()
	_, err = c.stdin.Write(data)
	c.writeMu.Unlock()
	if err != nil {
		c.pendMu.Lock()
		delete(c.pending, id)
		c.pendMu.Unlock()
		return nil, fmt.Errorf("mcp write: %w", err)
	}

	select {
	case resp, ok := <-ch:
		if !ok || resp == nil {
			return nil, errors.New("mcp: subprocess closed before reply")
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("mcp rpc %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	case <-ctx.Done():
		c.pendMu.Lock()
		delete(c.pending, id)
		c.pendMu.Unlock()
		return nil, ctx.Err()
	}
}

func (c *Client) notify(method string, params any) error {
	if c.closed.Load() {
		return errors.New("mcp: client closed")
	}
	n := notification{JSONRPC: "2.0", Method: method, Params: params}
	data, err := json.Marshal(n)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	c.writeMu.Lock()
	_, err = c.stdin.Write(data)
	c.writeMu.Unlock()
	return err
}

// Close terminates the MCP subprocess.
func (c *Client) Close() error {
	if c.closed.Swap(true) {
		return nil
	}
	_ = c.stdin.Close()
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		_ = c.cmd.Wait()
	}
	return nil
}

// ToolContent is one fragment of a tools/call response (MCP spec).
type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// CallTool invokes an MCP tool. Returns the concatenated text content of all
// "text"-typed content items. If isError is true the returned text is
// returned in the error as well.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	raw, err := c.call(ctx, "tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return "", err
	}
	var wrap struct {
		Content []ToolContent `json:"content"`
		IsError bool          `json:"isError,omitempty"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		// Some servers return the result body directly without the wrapper.
		return string(raw), nil
	}
	var sb []byte
	for _, c := range wrap.Content {
		if c.Type == "text" {
			sb = append(sb, c.Text...)
		}
	}
	if wrap.IsError {
		return string(sb), fmt.Errorf("mcp tool %q reported error: %s", name, string(sb))
	}
	return string(sb), nil
}

// AddConnection registers a named DSN with the MCP server. askdba's
// extended mode (MYSQL_MCP_EXTENDED=1) exposes this as a tool called
// "add_connection" taking dsn_name + connection fields.
func (c *Client) AddConnection(ctx context.Context, dsnName, host string, port int, user, password, database string) error {
	_, err := c.CallTool(ctx, "add_connection", map[string]any{
		"dsn_name": dsnName,
		"host":     host,
		"port":     port,
		"user":     user,
		"password": password,
		"database": database,
	})
	return err
}

// RemoveConnection unregisters a previously-added DSN.
func (c *Client) RemoveConnection(ctx context.Context, dsnName string) error {
	_, err := c.CallTool(ctx, "remove_connection", map[string]any{
		"dsn_name": dsnName,
	})
	return err
}
