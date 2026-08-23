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
	"unityperfagent/internal/protocol"
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
	service  *PerfService
	mu       sync.Mutex
	server   *http.Server
	listener net.Listener
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
	case "performance_memory_analysis":
		var input mcpRangeInput
		if err := decodeArguments(args, &input); err != nil {
			return nil, err
		}
		frames, err := mcpFrames(service, input.Offset, input.Limit)
		if err != nil {
			return nil, err
		}
		return memoryAnalysis(frames), nil
	case "performance_module_analysis":
		var input struct {
			Module string `json:"module"`
			mcpRangeInput
		}
		if err := decodeArguments(args, &input); err != nil {
			return nil, err
		}
		frames, err := mcpFrames(service, input.Offset, input.Limit)
		if err != nil {
			return nil, err
		}
		return moduleAnalysis(input.Module, frames)
	case "performance_frame_compare":
		var input struct {
			BaselineIndex   int `json:"baselineIndex"`
			ComparisonIndex int `json:"comparisonIndex"`
		}
		if err := decodeArguments(args, &input); err != nil {
			return nil, err
		}
		if input.BaselineIndex < 0 || input.BaselineIndex >= service.FrameCount() || input.ComparisonIndex < 0 || input.ComparisonIndex >= service.FrameCount() {
			return nil, errors.New("baselineIndex 和 comparisonIndex 必须是当前会话中的有效帧索引")
		}
		baseline := service.FrameAt(input.BaselineIndex)
		comparison := service.FrameAt(input.ComparisonIndex)
		return map[string]any{
			"baselineIndex": input.BaselineIndex, "comparisonIndex": input.ComparisonIndex,
			"baseline": baseline, "comparison": comparison, "delta": frameDelta(baseline, comparison),
		}, nil
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
		{"name": "performance_memory_analysis", "description": "按帧窗口分析 Unity Memory 模块：Used/Committed、GC、Gfx、纹理、网格、材质、对象数量的可用性、趋势、平均值和峰值。未提供参数时分析当前会话最近的最多 200 帧。", "inputSchema": rangeInputSchema()},
		{"name": "performance_module_analysis", "description": "分析指定 Profiler 模块的指标趋势、平均值与峰值。memory 会返回完整 Memory 分析；其它模块支持 cpu、gpu、rendering、audio、video、physics、ui。", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"module": map[string]any{"type": "string", "enum": []string{"cpu", "gpu", "rendering", "memory", "audio", "video", "physics", "ui"}}, "offset": map[string]any{"type": "integer", "minimum": 0}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": mcpMaxFrames}}, "required": []string{"module"}}},
		{"name": "performance_frame_compare", "description": "比较两帧的关键 CPU、GPU、GC、内存、渲染和对象指标，delta 为 comparisonIndex 减 baselineIndex。索引从当前保留的最旧帧开始、零基。", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"baselineIndex": map[string]any{"type": "integer", "minimum": 0}, "comparisonIndex": map[string]any{"type": "integer", "minimum": 0}}, "required": []string{"baselineIndex", "comparisonIndex"}}},
	}
}

type mcpRangeInput struct {
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

func rangeInputSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"offset": map[string]any{"type": "integer", "minimum": 0},
		"limit":  map[string]any{"type": "integer", "minimum": 1, "maximum": mcpMaxFrames},
	}}
}

// mcpFrames normalizes optional analysis windows. A zero limit is intentionally
// accepted for analysis tools, but not for performance_frames' paging API.
func mcpFrames(service *PerfService, offset, limit int) ([]protocol.FrameData, error) {
	if offset < 0 || limit < 0 || limit > mcpMaxFrames {
		return nil, fmt.Errorf("offset 必须 >= 0，limit 必须在 0 到 %d 之间", mcpMaxFrames)
	}
	count := service.FrameCount()
	if offset > count {
		return nil, fmt.Errorf("offset %d 超出当前 %d 帧会话", offset, count)
	}
	if limit == 0 {
		// 默认分析最新窗口；显式提供 offset 时则从该位置向后取样。
		if offset == 0 && count > mcpMaxFrames {
			offset = count - mcpMaxFrames
		}
		limit = count - offset
		if limit > mcpMaxFrames {
			limit = mcpMaxFrames
		}
	}
	return service.Frames(offset, limit), nil
}

type mcpMetric struct {
	Count     int     `json:"count"`
	Average   float64 `json:"average"`
	Min       float64 `json:"min"`
	Max       float64 `json:"max"`
	PeakFrame uint64  `json:"peakFrame"`
}

func metricSummary(frames []protocol.FrameData, get func(protocol.FrameData) float64, available func(protocol.FrameData) bool) mcpMetric {
	metric := mcpMetric{}
	for _, frame := range frames {
		if available != nil && !available(frame) {
			continue
		}
		value := get(frame)
		if metric.Count == 0 || value < metric.Min {
			metric.Min = value
		}
		if metric.Count == 0 || value > metric.Max {
			metric.Max = value
			metric.PeakFrame = frame.FrameIndex
		}
		metric.Average += value
		metric.Count++
	}
	if metric.Count > 0 {
		metric.Average /= float64(metric.Count)
	}
	return metric
}

func memoryAvailable(frame protocol.FrameData, bit uint32) bool {
	return frame.MemoryCounterMask == 0 || frame.MemoryCounterMask&bit != 0
}

func memoryAnalysis(frames []protocol.FrameData) map[string]any {
	result := map[string]any{"frameCount": len(frames), "metrics": map[string]mcpMetric{}}
	if len(frames) == 0 {
		result["summary"] = "没有可分析的 Memory 帧数据。"
		return result
	}
	available := func(bit uint32) func(protocol.FrameData) bool {
		return func(frame protocol.FrameData) bool { return memoryAvailable(frame, bit) }
	}
	metrics := map[string]mcpMetric{
		"usedMemMB":          metricSummary(frames, func(f protocol.FrameData) float64 { return float64(f.UsedMemMB) }, available(protocol.MemoryCounterTotalUsed)),
		"committedMemMB":     metricSummary(frames, func(f protocol.FrameData) float64 { return float64(f.CommittedMemMB) }, available(protocol.MemoryCounterCommitted)),
		"gcUsedMB":           metricSummary(frames, func(f protocol.FrameData) float64 { return float64(f.GCUsedMB) }, available(protocol.MemoryCounterGCUsed)),
		"gfxUsedMB":          metricSummary(frames, func(f protocol.FrameData) float64 { return float64(f.GfxUsedMB) }, available(protocol.MemoryCounterGfxUsed)),
		"textureMemMB":       metricSummary(frames, func(f protocol.FrameData) float64 { return float64(f.TextureMemMB) }, available(protocol.MemoryCounterTexture)),
		"meshMemMB":          metricSummary(frames, func(f protocol.FrameData) float64 { return float64(f.MeshMemMB) }, available(protocol.MemoryCounterMesh)),
		"materialMemMB":      metricSummary(frames, func(f protocol.FrameData) float64 { return float64(f.MaterialMemMB) }, available(protocol.MemoryCounterMaterial)),
		"gcAllocatedFrameKB": metricSummary(frames, func(f protocol.FrameData) float64 { return float64(f.GCAllocatedFrameKB) }, available(protocol.MemoryCounterGCAllocatedFrame)),
		"objectCount":        metricSummary(frames, func(f protocol.FrameData) float64 { return float64(f.ObjectCount) }, available(protocol.MemoryCounterObjectCount)),
	}
	first, last := frames[0], frames[len(frames)-1]
	result["metrics"] = metrics
	result["range"] = map[string]any{"firstFrame": first.FrameIndex, "lastFrame": last.FrameIndex, "firstTimestampMs": first.TimestampMs, "lastTimestampMs": last.TimestampMs}
	result["trend"] = map[string]any{
		"usedMemDeltaMB":   float64(last.UsedMemMB - first.UsedMemMB),
		"committedDeltaMB": float64(last.CommittedMemMB - first.CommittedMemMB),
		"objectDelta":      float64(last.ObjectCount) - float64(first.ObjectCount),
	}
	return result
}

func moduleAnalysis(module string, frames []protocol.FrameData) (any, error) {
	if module == "memory" {
		return memoryAnalysis(frames), nil
	}
	metrics := map[string]mcpMetric{}
	add := func(name string, get func(protocol.FrameData) float64) {
		metrics[name] = metricSummary(frames, get, nil)
	}
	switch module {
	case "cpu":
		add("cpuTimeMs", func(f protocol.FrameData) float64 { return float64(f.CPUTimeMs) })
		add("mainThreadMs", func(f protocol.FrameData) float64 { return float64(f.MainThreadMs) })
		add("scriptsMs", func(f protocol.FrameData) float64 { return float64(f.CPUScriptsMs) })
		add("gcAllocKB", func(f protocol.FrameData) float64 { return float64(f.GCAllocKB) })
	case "gpu":
		add("gpuTimeMs", func(f protocol.FrameData) float64 { return float64(f.GPUTimeMs) })
		add("renderThreadMs", func(f protocol.FrameData) float64 { return float64(f.RenderMs) })
	case "rendering":
		add("drawCalls", func(f protocol.FrameData) float64 { return float64(f.DrawCalls) })
		add("batches", func(f protocol.FrameData) float64 { return float64(f.Batches) })
		add("setPassCalls", func(f protocol.FrameData) float64 { return float64(f.SetPassCalls) })
		add("triangles", func(f protocol.FrameData) float64 { return float64(f.Triangles) })
	case "audio":
		add("playingSources", func(f protocol.FrameData) float64 { return float64(f.PlayingSources) })
		add("audioVoices", func(f protocol.FrameData) float64 { return float64(f.AudioVoices) })
		add("audioUsedMB", func(f protocol.FrameData) float64 { return float64(f.AudioUsedMB) })
	case "video":
		add("videoUsedMB", func(f protocol.FrameData) float64 { return float64(f.VideoUsedMB) })
	case "physics":
		add("activeRigidbodies", func(f protocol.FrameData) float64 { return float64(f.ActiveRigidbodies) })
		add("colliders", func(f protocol.FrameData) float64 { return float64(f.Colliders) })
		add("physicsMs", func(f protocol.FrameData) float64 { return float64(f.CPUPhysicsMs) })
	case "ui":
		add("uiBatches", func(f protocol.FrameData) float64 { return float64(f.UIBatches) })
		add("uiVertices", func(f protocol.FrameData) float64 { return float64(f.UIVertices) })
		add("uiMs", func(f protocol.FrameData) float64 { return float64(f.CPUUIMs) })
	default:
		return nil, fmt.Errorf("unknown profiler module %q", module)
	}
	return map[string]any{"module": module, "frameCount": len(frames), "metrics": metrics}, nil
}

func frameDelta(baseline, comparison protocol.FrameData) map[string]float64 {
	return map[string]float64{
		"cpuTimeMs": float64(comparison.CPUTimeMs - baseline.CPUTimeMs), "gpuTimeMs": float64(comparison.GPUTimeMs - baseline.GPUTimeMs), "gcAllocKB": float64(comparison.GCAllocKB - baseline.GCAllocKB),
		"usedMemMB": float64(comparison.UsedMemMB - baseline.UsedMemMB), "committedMemMB": float64(comparison.CommittedMemMB - baseline.CommittedMemMB),
		"drawCalls": float64(comparison.DrawCalls) - float64(baseline.DrawCalls), "batches": float64(comparison.Batches) - float64(baseline.Batches), "objectCount": float64(comparison.ObjectCount) - float64(baseline.ObjectCount),
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
