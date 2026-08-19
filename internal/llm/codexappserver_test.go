package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

func TestCodexAppServerReusesProcessAndThread(t *testing.T) {
	fake := newFakeCodexAppServer(t)
	profile := AgentProfile{
		Command: "codex-test",
		Args:    []string{"exec", "must-not-be-used"},
		Model:   "gpt-5.3-codex",
		Effort:  "high",
		mcpServers: []acp.McpServer{{Stdio: &acp.McpServerStdio{
			Name: "unity-perf-data", Command: "UnityPerfAgent.exe", Args: []string{"mcp"},
		}}},
	}
	server := newCodexAppServerWithStarter(fake.start)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Start(ctx, `D:\Project\UnityPerfAgent`, profile); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer server.Close()

	var first, second strings.Builder
	if err := server.Prompt(ctx, "首轮原始请求", profile, func(chunk string) { first.WriteString(chunk) }); err != nil {
		t.Fatalf("first Prompt: %v", err)
	}
	secondRaw := "  第二轮原始请求\n不要改写  "
	updated := profile
	updated.Model = "gpt-5.4"
	updated.Effort = "xhigh"
	if err := server.Prompt(ctx, secondRaw, updated, func(chunk string) { second.WriteString(chunk) }); err != nil {
		t.Fatalf("second Prompt: %v", err)
	}

	fake.mu.Lock()
	requests := append([]fakeCodexRequest(nil), fake.requests...)
	launches := fake.launches
	launchArgs := append([]string(nil), fake.launchArgs...)
	launchCommand := fake.launchCommand
	launchDir := fake.launchDir
	fake.mu.Unlock()

	if launches != 1 {
		t.Fatalf("process launches = %d, want 1", launches)
	}
	if launchCommand != profile.Command || launchDir != `D:\Project\UnityPerfAgent` {
		t.Fatalf("launch = %q in %q", launchCommand, launchDir)
	}
	if len(launchArgs) < 2 || launchArgs[0] != "app-server" || launchArgs[1] != "--stdio" {
		t.Fatalf("launch args = %q", launchArgs)
	}
	joinedArgs := strings.Join(launchArgs, " ")
	if strings.Contains(joinedArgs, "must-not-be-used") {
		t.Fatalf("profile exec args leaked into app-server launch: %q", launchArgs)
	}
	if got := strings.Count(joinedArgs, "mcp_servers.unity-perf-data.command"); got != 1 {
		t.Fatalf("MCP command config count = %d in %q", got, joinedArgs)
	}

	if got := countFakeCodexMethod(requests, "initialize"); got != 1 {
		t.Fatalf("initialize count = %d", got)
	}
	if got := countFakeCodexMethod(requests, "initialized"); got != 1 {
		t.Fatalf("initialized count = %d", got)
	}
	if got := countFakeCodexMethod(requests, "thread/start"); got != 1 {
		t.Fatalf("thread/start count = %d", got)
	}
	if got := countFakeCodexMethod(requests, "turn/start"); got != 2 {
		t.Fatalf("turn/start count = %d", got)
	}

	thread := fakeCodexMethod(requests, "thread/start", 0)
	if got := stringValue(thread.Params, "baseInstructions"); got != chatSystemHint {
		t.Fatalf("baseInstructions mismatch: %q", got)
	}
	if stringValue(thread.Params, "cwd") != `D:\Project\UnityPerfAgent` || stringValue(thread.Params, "model") != profile.Model {
		t.Fatalf("thread params = %#v", thread.Params)
	}
	if thread.Params["approvalPolicy"] != "never" || thread.Params["sandbox"] != "danger-full-access" || thread.Params["ephemeral"] != true {
		t.Fatalf("thread permissions = %#v", thread.Params)
	}
	config, _ := thread.Params["config"].(map[string]any)
	if config["model_reasoning_effort"] != "high" {
		t.Fatalf("thread config = %#v", config)
	}

	firstTurn := fakeCodexMethod(requests, "turn/start", 0)
	secondTurn := fakeCodexMethod(requests, "turn/start", 1)
	if got := fakePromptText(firstTurn); got != "首轮原始请求" {
		t.Fatalf("first prompt = %q", got)
	}
	if got := fakePromptText(secondTurn); got != secondRaw {
		t.Fatalf("second prompt = %q, want raw %q", got, secondRaw)
	}
	for i, turn := range []fakeCodexRequest{firstTurn, secondTurn} {
		if _, ok := turn.Params["baseInstructions"]; ok || strings.Contains(mustJSON(t, turn.Params), chatSystemHint) {
			t.Fatalf("turn %d contains baseInstructions: %#v", i+1, turn.Params)
		}
		if stringValue(turn.Params, "threadId") != "thread-1" {
			t.Fatalf("turn %d thread = %#v", i+1, turn.Params)
		}
	}
	if secondTurn.Params["model"] != "gpt-5.4" || secondTurn.Params["effort"] != "xhigh" {
		t.Fatalf("second turn overrides = %#v", secondTurn.Params)
	}

	if got := first.String(); !strings.Contains(got, "第1轮回答") || !strings.Contains(got, "正在调用 unity-perf-data.capture") || !strings.Contains(got, "已返回结果") {
		t.Fatalf("first streamed output = %q", got)
	}
	if got := second.String(); !strings.Contains(got, "第2轮回答") {
		t.Fatalf("second streamed output = %q", got)
	}
}

type fakeCodexRequest struct {
	ID     json.RawMessage
	Method string
	Params map[string]any
}

type fakeCodexAppServer struct {
	t *testing.T

	mu            sync.Mutex
	launches      int
	launchCommand string
	launchArgs    []string
	launchDir     string
	requests      []fakeCodexRequest
	turns         int
}

func newFakeCodexAppServer(t *testing.T) *fakeCodexAppServer {
	return &fakeCodexAppServer{t: t}
}

func (f *fakeCodexAppServer) start(command string, args []string, dir string) (codexAppProcess, error) {
	serverInput, clientInput := io.Pipe()
	clientOutput, serverOutput := io.Pipe()
	process := &fakeCodexProcess{
		stdin: clientInput, stdout: clientOutput, serverInput: serverInput, serverOutput: serverOutput,
		done: make(chan error, 1),
	}
	f.mu.Lock()
	f.launches++
	f.launchCommand = command
	f.launchArgs = append([]string(nil), args...)
	f.launchDir = dir
	f.mu.Unlock()
	go f.serve(process)
	return process, nil
}

func (f *fakeCodexAppServer) serve(process *fakeCodexProcess) {
	defer func() {
		_ = process.serverOutput.Close()
		_ = process.serverInput.Close()
		process.done <- nil
		close(process.done)
	}()
	scanner := bufio.NewScanner(process.serverInput)
	encoder := json.NewEncoder(process.serverOutput)
	for scanner.Scan() {
		var wire struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &wire); err != nil {
			return
		}
		var params map[string]any
		if len(wire.Params) > 0 && string(wire.Params) != "null" {
			if err := json.Unmarshal(wire.Params, &params); err != nil {
				return
			}
		}
		f.mu.Lock()
		f.requests = append(f.requests, fakeCodexRequest{ID: append(json.RawMessage(nil), wire.ID...), Method: wire.Method, Params: params})
		f.mu.Unlock()

		switch wire.Method {
		case "initialized":
			continue
		case "initialize":
			_ = encoder.Encode(map[string]any{"id": rawID(wire.ID), "result": map[string]any{"serverInfo": map[string]any{"name": "fake"}}})
		case "thread/start":
			_ = encoder.Encode(map[string]any{"id": rawID(wire.ID), "result": map[string]any{"thread": map[string]any{"id": "thread-1"}}})
		case "turn/start":
			f.mu.Lock()
			f.turns++
			turnNumber := f.turns
			f.mu.Unlock()
			turnID := fmt.Sprintf("turn-%d", turnNumber)
			_ = encoder.Encode(map[string]any{"id": rawID(wire.ID), "result": map[string]any{"turn": map[string]any{"id": turnID}}})
			_ = encoder.Encode(map[string]any{"method": "item/started", "params": map[string]any{
				"threadId": "thread-1", "turnId": turnID,
				"item": map[string]any{"type": "mcpToolCall", "server": "unity-perf-data", "tool": "capture"},
			}})
			_ = encoder.Encode(map[string]any{"method": "item/agentMessage/delta", "params": map[string]any{
				"threadId": "thread-1", "turnId": turnID, "delta": fmt.Sprintf("第%d轮回答", turnNumber),
			}})
			_ = encoder.Encode(map[string]any{"method": "item/completed", "params": map[string]any{
				"threadId": "thread-1", "turnId": turnID,
				"item": map[string]any{"type": "mcpToolCall", "server": "unity-perf-data", "tool": "capture"},
			}})
			_ = encoder.Encode(map[string]any{"method": "turn/completed", "params": map[string]any{
				"threadId": "thread-1", "turn": map[string]any{"id": turnID, "status": "completed", "items": []any{}},
			}})
		default:
			_ = encoder.Encode(map[string]any{"id": rawID(wire.ID), "error": map[string]any{"code": -32601, "message": "unknown method"}})
		}
	}
	if err := scanner.Err(); err != nil {
		return
	}
}

func rawID(id json.RawMessage) any {
	var value any
	_ = json.Unmarshal(id, &value)
	return value
}

type fakeCodexProcess struct {
	stdin        *io.PipeWriter
	stdout       *io.PipeReader
	serverInput  *io.PipeReader
	serverOutput *io.PipeWriter
	done         chan error
	closeOnce    sync.Once
}

func (p *fakeCodexProcess) Stdin() io.WriteCloser { return p.stdin }
func (p *fakeCodexProcess) Stdout() io.ReadCloser { return p.stdout }
func (p *fakeCodexProcess) Stderr() string        { return "" }
func (p *fakeCodexProcess) Done() <-chan error    { return p.done }
func (p *fakeCodexProcess) Close() {
	p.closeOnce.Do(func() {
		_ = p.stdin.Close()
		_ = p.stdout.Close()
		_ = p.serverInput.Close()
		_ = p.serverOutput.Close()
	})
}

func countFakeCodexMethod(requests []fakeCodexRequest, method string) int {
	count := 0
	for _, request := range requests {
		if request.Method == method {
			count++
		}
	}
	return count
}

func fakeCodexMethod(requests []fakeCodexRequest, method string, index int) fakeCodexRequest {
	for _, request := range requests {
		if request.Method != method {
			continue
		}
		if index == 0 {
			return request
		}
		index--
	}
	return fakeCodexRequest{}
}

func fakePromptText(request fakeCodexRequest) string {
	input, _ := request.Params["input"].([]any)
	if len(input) != 1 {
		return ""
	}
	block, _ := input[0].(map[string]any)
	if block["type"] != "text" {
		return ""
	}
	text, _ := block["text"].(string)
	return text
}

func stringValue(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(encoded)
}
