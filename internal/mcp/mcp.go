// Package mcp implements the stdio side of UnityPerfAgent's embedded MCP server.
//
// The process launched by an ACP Agent only owns the JSON-RPC stdio transport. It
// forwards tool calls to the desktop process over a token-protected loopback
// connection, where the live profiler session is available.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

const protocolVersion = "2024-11-05"

// Request is a JSON-RPC 2.0 request sent by an MCP client.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Serve runs the MCP JSON-RPC loop. endpoint must point to the parent desktop
// process's loopback bridge; token is sent in the Authorization header.
func Serve(ctx context.Context, in io.Reader, out io.Writer, endpoint, token string) error {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" || strings.TrimSpace(token) == "" {
		return fmt.Errorf("mcp: missing parent endpoint or token")
	}
	forward := &forwarder{endpoint: endpoint, token: token}
	writer := &lockedWriter{w: out}
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			_ = writeResponse(writer, response{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error"}})
			continue
		}
		if req.JSONRPC != "2.0" || req.Method == "" {
			_ = writeResponse(writer, response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32600, Message: "invalid request"}})
			continue
		}
		if len(req.ID) == 0 { // MCP notifications do not have a response.
			if req.Method == "notifications/initialized" {
				// Forward for the parent-side interaction log; its result is ignored.
				_, _ = forward.call(ctx, req)
			}
			continue
		}
		resp := handle(ctx, req, forward)
		if err := writeResponse(writer, resp); err != nil {
			return err
		}
	}
	return sc.Err()
}

func handle(ctx context.Context, req Request, forward *forwarder) response {
	resp := response{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		// The parent owns the log, while this process owns the stdio protocol.
		// Logging failure must not prevent the MCP handshake from completing.
		_, _ = forward.call(ctx, req)
		resp.Result = map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]string{"name": "UnityPerfAgent", "version": "1.0.0"},
		}
	case "tools/list", "tools/call":
		result, err := forward.call(ctx, req)
		if err != nil {
			resp.Error = &rpcError{Code: -32000, Message: err.Error()}
		} else {
			resp.Result = result
		}
	default:
		resp.Error = &rpcError{Code: -32601, Message: "method not found"}
	}
	return resp
}

type forwarder struct {
	endpoint string
	token    string
	client   http.Client
}

func (f *forwarder) call(ctx context.Context, req Request) (any, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, f.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+f.token)
	httpResp, err := f.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("MCP bridge unavailable: %w", err)
	}
	defer httpResp.Body.Close()
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(httpResp.Body, 16*1024*1024)).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("invalid MCP bridge response: %w", err)
	}
	if httpResp.StatusCode != http.StatusOK || envelope.Error != "" {
		if envelope.Error != "" {
			return nil, fmt.Errorf("MCP tool failed: %s", envelope.Error)
		}
		return nil, fmt.Errorf("MCP bridge returned HTTP %d", httpResp.StatusCode)
	}
	var result any
	if len(envelope.Result) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(envelope.Result, &result); err != nil {
		return nil, fmt.Errorf("invalid MCP result: %w", err)
	}
	return result, nil
}

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(p)
}

func writeResponse(w io.Writer, resp response) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}
