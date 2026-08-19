package services

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"unityperfagent/internal/llm"
	"unityperfagent/internal/mcp"
)

const (
	agentLogCapacity = 1000
	mcpMaxFrames     = 200
)

// AgentLogEntry records a complete Agent or MCP interaction. Payload is kept as
// JSON/text so the native log window can show the exact data exchanged.
type AgentLogEntry struct {
	Time      time.Time `json:"time"`
	Source    string    `json:"source"`
	Direction string    `json:"direction"`
	Method    string    `json:"method"`
	Payload   string    `json:"payload"`
}

type agentMCPBridge struct {
	service      *PerfService
	mu           sync.Mutex
	server       *http.Server
	listener     net.Listener
	endpoint string
	token    string
}

func newAgentMCPBridge(service *PerfService) *agentMCPBridge {
	return &agentMCPBridge{service: service}
}

func (b *agentMCPBridge) config() (llm.MCPServer, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.server == nil {
		if err := b.startLocked(); err != nil {
			return llm.MCPServer{}, err
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return llm.MCPServer{}, fmt.Errorf("MCP server executable: %w", err)
	}
	return llm.NewStdioMCP(exe, []string{"mcp"}, map[string]string{
		"UNITY_PERF_MCP_ENDPOINT": b.endpoint,
		"UNITY_PERF_MCP_TOKEN":    b.token,
	}), nil
}

func (b *agentMCPBridge) startLocked() error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("start MCP bridge: %w", err)
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		_ = listener.Close()
		return fmt.Errorf("generate MCP bridge token: %w", err)
	}
	b.listener = listener
	b.endpoint = "http://" + listener.Addr().String() + "/mcp"
	b.token = hex.EncodeToString(tokenBytes)
	b.server = &http.Server{Handler: http.HandlerFunc(b.serveHTTP)}
	go func() { _ = b.server.Serve(listener) }()
	return nil
}

func (b *agentMCPBridge) close() {
	b.mu.Lock()
	server := b.server
	b.server = nil
	b.listener = nil
	b.endpoint = ""
	b.token = ""
	b.mu.Unlock()
	if server != nil {
		_ = server.Close()
	}
}

func (b *agentMCPBridge) serveHTTP(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	token := b.token
	b.mu.Unlock()
	if r.Method != http.MethodPost || r.URL.Path != "/mcp" || r.Header.Get("Authorization") != "Bearer "+token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	defer r.Body.Close()
	var request mcp.Request
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8*1024*1024)).Decode(&request); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	b.service.addAgentLog("MCP", "Agent → MCP", request.Method, prettyJSON(request))
	result, err := b.handle(request)
	if err != nil {
		b.service.addAgentLog("MCP", "MCP → Agent", request.Method, prettyJSON(map[string]string{"error": err.Error()}))
		writeMCPEnvelope(w, nil, err)
		return
	}
	b.service.addAgentLog("MCP", "MCP → Agent", request.Method, prettyJSON(result))
	writeMCPEnvelope(w, result, nil)
}

func writeMCPEnvelope(w http.ResponseWriter, result any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"result": result})
}

func (b *agentMCPBridge) handle(request mcp.Request) (any, error) {
	switch request.Method {
	case "initialize", "notifications/initialized":
		return map[string]bool{"recorded": true}, nil
	case "tools/list":
		return map[string]any{"tools": mcpTools()}, nil
	case "tools/call":
		var call struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(request.Params, &call); err != nil {
			return nil, errors.New("tools/call 参数无效")
		}
		result, err := b.execute(call.Name, call.Arguments)
		if err != nil {
			return nil, err
		}
		data, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}
		return map[string]any{"content": []map[string]string{{"type": "text", "text": string(data)}}}, nil
	default:
		return nil, fmt.Errorf("unsupported MCP method %q", request.Method)
	}
}

func (b *agentMCPBridge) execute(name string, args json.RawMessage) (any, error) {
	service := b.service
	switch name {
	case "performance_session_summary":
		return struct {
			Status Status `json:"status"`
			Stats  any    `json:"stats"`
			Report any    `json:"report"`
		}{service.Status(), service.Stats(), service.Analyze()}, nil
	case "performance_frames":
		var input struct {
			Offset int `json:"offset"`
			Limit  int `json:"limit"`
		}
		if err := decodeArguments(args, &input); err != nil {
			return nil, err
		}
		if input.Offset < 0 || input.Limit < 1 || input.Limit > mcpMaxFrames {
			return nil, fmt.Errorf("offset 必须 >= 0，limit 必须在 1 到 %d 之间", mcpMaxFrames)
		}
		return service.Frames(input.Offset, input.Limit), nil
	case "performance_frame":
		var input struct {
			Index            int  `json:"index"`
			IncludeHierarchy bool `json:"includeHierarchy"`
		}
		if err := decodeArguments(args, &input); err != nil {
			return nil, err
		}
		if input.Index < 0 || input.Index >= service.FrameCount() {
			return nil, fmt.Errorf("frame index %d 不存在", input.Index)
		}
		result := map[string]any{"frame": service.FrameAt(input.Index)}
		if input.IncludeHierarchy {
			result["hierarchy"] = service.FrameHierarchy(input.Index)
		}
		return result, nil
	case "performance_cpu_hierarchy":
		var input struct {
			Index int `json:"index"`
		}
		if err := decodeArguments(args, &input); err != nil {
			return nil, err
		}
		if input.Index < 0 || input.Index >= service.FrameCount() {
			return nil, fmt.Errorf("frame index %d 不存在", input.Index)
		}
		return service.FrameHierarchy(input.Index), nil
	case "performance_analysis":
		return service.Analyze(), nil
	default:
		return nil, fmt.Errorf("unknown performance tool %q", name)
	}
}

func decodeArguments(raw json.RawMessage, dst any) error {
	if len(raw) == 0 || string(raw) == "null" {
		raw = []byte("{}")
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return errors.New("工具参数必须是 JSON 对象")
	}
	return nil
}

func mcpTools() []map[string]any {
	return []map[string]any{
		{"name": "performance_session_summary", "description": "获取当前 Unity 2022 性能采集会话的连接状态、聚合指标和规则分析报告。", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{}}},
		{"name": "performance_frames", "description": "按采集时间顺序查询连续帧摘要。offset 从当前保留的最旧帧起算；每次最多 200 帧。", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"offset": map[string]any{"type": "integer", "minimum": 0}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": mcpMaxFrames}}, "required": []string{"offset", "limit"}}},
		{"name": "performance_frame", "description": "查询单个帧的全部性能计数器，可选附带 Main Thread CPU Hierarchy。", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"index": map[string]any{"type": "integer", "minimum": 0}, "includeHierarchy": map[string]any{"type": "boolean"}}, "required": []string{"index"}}},
		{"name": "performance_cpu_hierarchy", "description": "查询指定帧的 Main Thread CPU Hierarchy（含调用耗时、GC 分配与子节点）。", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"index": map[string]any{"type": "integer", "minimum": 0}}, "required": []string{"index"}}},
		{"name": "performance_analysis", "description": "运行内置规则分析，返回卡顿、GC、FPS、Draw Call 与内存增长问题。", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{}}},
	}
}

func prettyJSON(value any) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}

func (p *PerfService) addAgentLog(source, direction, method, payload string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.agentLogs = append(p.agentLogs, AgentLogEntry{Time: time.Now(), Source: source, Direction: direction, Method: method, Payload: payload})
	if len(p.agentLogs) > agentLogCapacity {
		copy(p.agentLogs, p.agentLogs[len(p.agentLogs)-agentLogCapacity:])
		p.agentLogs = p.agentLogs[:agentLogCapacity]
	}
}

// AgentLogs returns the recent Agent and MCP protocol interactions.
func (p *PerfService) AgentLogs() []AgentLogEntry {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]AgentLogEntry(nil), p.agentLogs...)
}

// ClearAgentLogs removes the interaction history displayed in the Agent Log window.
func (p *PerfService) ClearAgentLogs() {
	p.mu.Lock()
	p.agentLogs = nil
	p.mu.Unlock()
}
