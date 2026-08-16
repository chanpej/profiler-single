// Package services 定义暴露给前端的 Wails 服务。
package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"unityperfagent/internal/adbforward"
	"unityperfagent/internal/analyzer"
	"unityperfagent/internal/capture"
	"unityperfagent/internal/collector"
	"unityperfagent/internal/device"
	"unityperfagent/internal/llm"
	"unityperfagent/internal/protocol"
	"unityperfagent/internal/unityprofiler"
)

// TickEventName 是采集进度事件名，前端据此增量刷新。
const TickEventName = "perf:tick"

// ImportProgressEventName 是加载存档解析进度事件名。
const ImportProgressEventName = "perf:import-progress"

// AgentChunkEventName 是 ACP 聊天流式文本事件名。
const AgentChunkEventName = "agent:chunk"

const importProgressMinInterval = 80 * time.Millisecond

const tickInterval = 250 * time.Millisecond

// TickEvent 是推送给前端的采集进度。
type TickEvent struct {
	TotalFrames uint64 `json:"totalFrames"`
	Retained    int    `json:"retained"`
	Connected   bool   `json:"connected"`
	Recording   bool   `json:"recording"`
}

// AgentChunkEvent 是 ACP Agent 流式回复的一段文本。
type AgentChunkEvent struct {
	Text string `json:"text"`
}

// ImportProgressEvent 是加载存档时的解析进度。
type ImportProgressEvent struct {
	Phase    string `json:"phase"`
	Done     int    `json:"done"`
	Total    int    `json:"total"`
	Percent  int    `json:"percent"`
	FileName string `json:"fileName"`
}

// Status 是连接状态快照。
type Status struct {
	Connected bool `json:"connected"`
	// Mode 固定为 "profiler"：Unity Editor Profiler 协议采集。
	Mode string `json:"mode"`
	// Transport 为 "adb"（USB adb forward）或空。
	Transport      string             `json:"transport"`
	Device         protocol.Handshake `json:"device"`
	TotalFrames    uint64             `json:"totalFrames"`
	Retained       int                `json:"retained"`
	UnitySupported bool               `json:"unitySupported"`
	// Recording 为 true 时把收到的帧写入会话；false 表示暂停抓取。
	Recording bool `json:"recording"`
	// SourceFile 是当前数据来自的存档路径；实时采集时为空。
	SourceFile string `json:"sourceFile"`
	// DeepProfiling / CallStacks 是工具栏开关；连接后会下发给 Player。
	DeepProfiling bool `json:"deepProfiling"`
	CallStacks    bool `json:"callStacks"`
	// DeepProfilerSupported / ManagedCallstacksSupported 来自 Player 能力位。
	DeepProfilerSupported      bool `json:"deepProfilerSupported"`
	ManagedCallstacksSupported bool `json:"managedCallstacksSupported"`
	// LastError 是最近一次采集异常中断的原因；正常断开或未出错时为空。
	// 解码失步这类问题必须让用户看见，否则界面只会停在"已连接但不再出帧"。
	LastError string `json:"lastError"`
	// HasRaw 表示会话里有可导出的原始 Profiler 数据流。
	HasRaw bool `json:"hasRaw"`
}

// PerfService 是性能采集与分析的门面，供前端调用。
type PerfService struct {
	mu         sync.Mutex
	client     *device.Client
	session    *collector.Session
	llmCfg     llm.Config
	transport  string
	tunnel     *adbforward.Tunnel
	adbRun     adbforward.Runner
	emit       func(name string, data any)
	logger     *log.Logger
	logFile    *os.File
	recording  bool
	captureRaw bool
	sourceFile string
	loadedHS   protocol.Handshake
	deep       bool
	callStacks bool
	lastErr    string
	chat       *llm.ChatSession

	// tickReq 是"界面需要刷新"的合并信号，容量 1 且非阻塞投递。
	tickReq chan struct{}
	stopUI  chan struct{}
}

// NewPerfService 创建服务实例。
func NewPerfService() *PerfService {
	p := &PerfService{
		client:    device.NewClient(),
		session:   collector.NewSession(0),
		adbRun:    adbforward.DefaultRun,
		emit:      func(string, any) {},
		recording:  true,
		captureRaw: true,
		tickReq:    make(chan struct{}, 1),
		stopUI:     make(chan struct{}),
	}
	p.client.SetRawHandler(p.onRaw)
	go p.uiLoop()
	return p
}

// ServiceStartup 在 Wails 应用启动时注入事件发射器，并打开诊断日志。
func (p *PerfService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	app := application.Get()
	p.mu.Lock()
	p.emit = func(name string, data any) { app.Event.Emit(name, data) }
	p.mu.Unlock()
	p.openLog()
	return nil
}

func (p *PerfService) ServiceShutdown() error {
	p.Disconnect()
	p.AgentChatClose()
	close(p.stopUI)
	p.mu.Lock()
	f := p.logFile
	p.logFile = nil
	p.mu.Unlock()
	if f != nil {
		_ = f.Close()
	}
	return nil
}

// openLog 在可执行文件旁开一份诊断日志，记录采集链路的关键指标。
// 排障时不必重新构建就能看清"连上了但没数据"卡在哪一环。
func (p *PerfService) openLog() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(filepath.Dir(exe), "unityperfagent.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	p.mu.Lock()
	p.logFile = f
	p.logger = log.New(f, "", log.LstdFlags|log.Lmicroseconds)
	p.mu.Unlock()
	p.logf("=== UnityPerfAgent 启动 ===")
}

func (p *PerfService) logf(format string, args ...any) {
	p.mu.Lock()
	logger := p.logger
	p.mu.Unlock()
	if logger != nil {
		logger.Printf(format, args...)
	}
}

func (p *PerfService) connectedLocked() bool {
	return p.client.Connected()
}

func (p *PerfService) connected() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.connectedLocked()
}

// Connect 仅接受 USB 目标（usb / adb / adb:<serial>）。Wi‑Fi Direct Connection 已移除。
func (p *PerfService) Connect(host string, port int) (protocol.Handshake, error) {
	if serial, viaADB := adbforward.ParseHost(host); viaADB {
		return p.ConnectUSB(serial, "")
	}
	return protocol.Handshake{}, errors.New("已移除 Wi-Fi Direct Connection，请使用 USB 连接")
}

// ConnectUSB 通过 adb forward 连接 USB 手机上的 Development Player。
// packageID 为应用包名（如 com.company.game），用于 localabstract:Unity-{bundleId}。
// 只使用官方通道，不回退 tcp:55000。
func (p *PerfService) ConnectUSB(serial, packageID string) (protocol.Handshake, error) {
	p.mu.Lock()
	if p.connectedLocked() {
		p.mu.Unlock()
		return protocol.Handshake{}, errors.New("已有连接，请先断开")
	}
	run := p.adbRun
	p.mu.Unlock()

	if err := adbforward.ValidatePackageID(packageID); err != nil {
		return protocol.Handshake{}, err
	}

	dev, err := adbforward.ResolveSerial(run, serial)
	if err != nil {
		return protocol.Handshake{}, err
	}
	specs := adbforward.ProbeForwardSpecs(run, dev.Serial, packageID)
	if len(specs) == 0 {
		return protocol.Handshake{}, errors.New("adb forward 失败：未找到 localabstract:Unity-{包名}，请确认 Development Build 正在前台运行")
	}

	var lastErr error
	for _, spec := range specs {
		if !strings.HasPrefix(spec.Remote, "localabstract:Unity-") {
			continue
		}
		adbforward.RemoveStale(run, dev.Serial, spec.Remote)
		tun, err := adbforward.ForwardRemote(run, dev.Serial, spec.Remote)
		if err != nil {
			lastErr = err
			continue
		}
		hs, err := p.connectTCP("127.0.0.1", tun.LocalPort, "adb")
		if err != nil {
			_ = adbforward.Remove(run, tun)
			wrapped := fmt.Errorf("adb forward 本机:%d -> %s 已建立。%w", tun.LocalPort, spec.Remote, err)
			if unityprofiler.IsPlayerReject(err) {
				return protocol.Handshake{}, wrapped
			}
			if errors.Is(err, unityprofiler.ErrPlayerSilent) {
				// 通道是通的，问题在 Player 侧被占用，不该提示 Development Build。
				return protocol.Handshake{}, wrapped
			}
			lastErr = fmt.Errorf("adb forward 本机:%d -> %s 已建立，但 Profiler 握手失败（需 Unity 2022 Development Build）: %w", tun.LocalPort, spec.Remote, err)
			continue
		}
		p.client.PatchHandshake(func(h *protocol.Handshake) {
			if h.UnityVersion == "" {
				h.UnityVersion = adbforward.ProbeUnityVersion(run, dev.Serial)
			}
			if h.DeviceModel == "" {
				h.DeviceModel = dev.Model
			}
			if h.OS == "" {
				h.OS = "Android"
			}
		})
		hs, _ = p.client.Handshake()
		p.mu.Lock()
		p.tunnel = &tun
		p.mu.Unlock()
		return hs, nil
	}
	if lastErr == nil {
		lastErr = errors.New("adb forward 失败：未找到 localabstract:Unity-{包名}，请确认 Development Build 正在前台运行")
	}
	return protocol.Handshake{}, lastErr
}

func (p *PerfService) connectTCP(host string, port int, transport string) (protocol.Handshake, error) {
	p.mu.Lock()
	if p.connectedLocked() {
		p.mu.Unlock()
		return protocol.Handshake{}, errors.New("已有连接，请先断开")
	}
	p.session.ClearAll()
	p.transport = transport
	p.recording = true
	p.captureRaw = true
	p.sourceFile = ""
	p.loadedHS = protocol.Handshake{}
	p.lastErr = ""
	p.mu.Unlock()

	hs, err := p.client.Connect(host, port, p.onFrame, p.onClosed)
	if err != nil {
		p.mu.Lock()
		p.transport = ""
		p.mu.Unlock()
		p.logf("连接 %s:%d(%s) 失败: %v", host, port, transport, err)
		return protocol.Handshake{}, err
	}
	p.logf("已连接 %s:%d(%s): 设备=%q 系统=%q 引擎=%q 流版本=%#x",
		host, port, transport, hs.DeviceName, hs.OS, hs.UnityVersion, hs.ProfilerStreamVersion)
	p.mu.Lock()
	deep, stacks := p.deep, p.callStacks
	p.mu.Unlock()
	if deep || stacks {
		if err := p.client.SetProfilerOptions(deep, stacks); err != nil {
			p.logf("下发 Deep Profile/Call Stacks 失败: %v", err)
		}
	}
	p.notifyUI()
	return hs, nil
}

// DiscoverPlayers 列出 USB 上可通过 adb forward 连接的设备（不含 Wi‑Fi 组播）。
func (p *PerfService) DiscoverPlayers() ([]unityprofiler.PlayerInfo, error) {
	p.mu.Lock()
	run := p.adbRun
	p.mu.Unlock()
	return adbforward.USBPlayers(run)
}

// Disconnect 断开 Profiler 连接（并拆除 adb forward）。已采集数据保留。
func (p *PerfService) Disconnect() {
	p.client.Close()
	p.mu.Lock()
	p.transport = ""
	tun := p.tunnel
	p.tunnel = nil
	run := p.adbRun
	p.mu.Unlock()
	if tun != nil {
		_ = adbforward.Remove(run, *tun)
	}
	p.logf("已断开")
	p.notifyUI()
}

// Status 返回连接状态快照。
func (p *PerfService) Status() Status {
	p.mu.Lock()
	transport := p.transport
	recording := p.recording
	sourceFile := p.sourceFile
	loadedHS := p.loadedHS
	deep := p.deep
	callStacks := p.callStacks
	lastErr := p.lastErr
	p.mu.Unlock()
	hs, _ := p.client.Handshake()
	if hs == (protocol.Handshake{}) {
		hs = loadedHS
	}
	connected := p.client.Connected()
	return Status{
		Connected:                  connected,
		Mode:                       "profiler",
		Transport:                  transport,
		Device:                     hs,
		TotalFrames:                p.session.Total(),
		Retained:                   p.session.Len(),
		UnitySupported:             connected && hs.Supported(),
		Recording:                  recording,
		SourceFile:                 sourceFile,
		DeepProfiling:              deep,
		CallStacks:                 callStacks,
		DeepProfilerSupported:      hs.DeepProfilerSupported,
		ManagedCallstacksSupported: hs.ManagedCallstacksSupported,
		LastError:                  lastErr,
		HasRaw:                     p.session.RawLen() > 0,
	}
}

func (p *PerfService) Frames(offset, limit int) []protocol.FrameData {
	return p.session.Range(offset, limit)
}

func (p *PerfService) Latest(n int) []protocol.FrameData {
	return p.session.Latest(n)
}

func (p *PerfService) FrameAt(index int) protocol.FrameData {
	f, ok := p.session.At(index)
	if !ok {
		return protocol.FrameData{}
	}
	return f
}

func (p *PerfService) FrameCount() int {
	return p.session.Len()
}

func (p *PerfService) Stats() collector.Stats {
	return p.session.Stats()
}

func (p *PerfService) Clear() {
	connected := p.client.Connected()
	p.session.Clear()
	p.mu.Lock()
	p.sourceFile = ""
	p.loadedHS = protocol.Handshake{}
	p.captureRaw = connected
	p.mu.Unlock()
	p.notifyUI()
}

// SetRecording 开关抓取。连接保持不变；暂停时丢弃新到的帧，已采集数据保留。
func (p *PerfService) SetRecording(on bool) {
	p.mu.Lock()
	p.recording = on
	p.mu.Unlock()
	p.logf("采集开关: recording=%v", on)
	p.notifyUI()
}

// SetDeepProfiling 开关 Deep Profile。连接后立即下发给 Player，影响后续帧。
func (p *PerfService) SetDeepProfiling(on bool) error {
	p.mu.Lock()
	p.deep = on
	stacks := p.callStacks
	p.mu.Unlock()
	p.logf("Deep Profile: %v", on)
	if p.client.Connected() {
		if err := p.client.SetProfilerOptions(on, stacks); err != nil {
			return err
		}
	}
	p.notifyUI()
	return nil
}

// SetCallStacks 开关 Call Stacks（GC.Alloc 等采样的托管调用栈）。
func (p *PerfService) SetCallStacks(on bool) error {
	p.mu.Lock()
	p.callStacks = on
	deep := p.deep
	p.mu.Unlock()
	p.logf("Call Stacks: %v", on)
	if p.client.Connected() {
		if err := p.client.SetProfilerOptions(deep, on); err != nil {
			return err
		}
	}
	p.notifyUI()
	return nil
}

// FrameHierarchy 返回选中帧的 CPU Hierarchy。
func (p *PerfService) FrameHierarchy(index int) []protocol.SampleNode {
	h, ok := p.session.HierarchyAt(index)
	if !ok {
		return nil
	}
	return h
}

// ExportCapture 将当前会话的原始 Profiler 数据流导出为 .upac 文件。
func (p *PerfService) ExportCapture(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("导出路径为空")
	}
	hs, _ := p.client.Handshake()
	p.mu.Lock()
	if hs == (protocol.Handshake{}) {
		hs = p.loadedHS
	}
	p.mu.Unlock()
	raw := p.session.Raw()
	if len(raw) == 0 {
		return errors.New("没有可导出的原始 Profiler 数据")
	}
	if err := capture.SaveFile(path, capture.File{Handshake: hs, Raw: raw}); err != nil {
		return fmt.Errorf("无法保存存档：%w", err)
	}
	p.logf("已导出 %d 个原始数据块 -> %s", len(raw), path)
	return nil
}

// ImportCapture 加载存档并按当前解码器重新分析。
// v2 文件重放原始 Profiler 流；v1 旧文件仅还原已解码帧（无 Hierarchy）。
// 若仍连接真机，会自动暂停抓取，避免实时帧冲掉刚加载的数据。
func (p *PerfService) ImportCapture(path string) (int, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return 0, errors.New("加载路径为空")
	}
	fileName := filepath.Base(path)
	last := time.Time{}
	p.emitImportProgress(fileName, "read", 0, 1, true, &last)
	file, err := capture.LoadFile(path)
	if err != nil {
		return 0, fmt.Errorf("无法读取存档：%w", err)
	}
	if len(file.Raw) > 0 {
		frames, hiers, err := unityprofiler.ReplayWithProgress(file.Raw, func(done, total int) {
			p.emitImportProgress(fileName, "parse", done, total, false, &last)
		})
		if err != nil {
			return 0, fmt.Errorf("解析存档失败：%w", err)
		}
		p.emitImportProgress(fileName, "parse", len(file.Raw), len(file.Raw), true, &last)
		p.session.ReplaceDetail(frames, hiers)
		p.session.SetRaw(file.Raw)
	} else if len(file.Frames) > 0 {
		p.emitImportProgress(fileName, "parse", 1, 1, true, &last)
		p.session.Replace(file.Frames)
	} else {
		return 0, errors.New("存档没有可解析的 Profiler 数据")
	}
	connected := p.client.Connected()
	p.mu.Lock()
	p.sourceFile = path
	p.loadedHS = file.Handshake
	p.captureRaw = false
	if connected {
		p.recording = false
	}
	p.mu.Unlock()
	p.logf("已加载 %d 帧 <- %s", p.session.Len(), path)
	p.notifyUI()
	return p.session.Len(), nil
}

func (p *PerfService) Analyze() analyzer.Report {
	return analyzer.Analyze(p.session.Range(0, p.session.Len()))
}

func (p *PerfService) SetLLMConfig(cfg llm.Config) {
	cfg = cfg.Normalize()
	p.mu.Lock()
	old := p.llmCfg
	p.llmCfg = cfg
	chat := p.chat
	restart := chat != nil && llm.ShouldRestartChat(old, cfg)
	if restart {
		p.chat = nil
	} else if chat != nil {
		chat.UpdateConfig(cfg)
	}
	p.mu.Unlock()
	if restart {
		chat.Close()
	}
}

// ProbeAgent 检测给定配置能否完成 ACP 握手。不打断当前聊天会话。
func (p *PerfService) ProbeAgent(ctx context.Context, cfg llm.Config) llm.ProbeResult {
	return llm.Probe(ctx, cfg)
}

// ListAgentModels 探测当前 Agent 支持的模型列表。
// Codex：`codex debug models`；Cursor：`agent --list-models`，失败则回退 ACP configOptions。
func (p *PerfService) ListAgentModels(ctx context.Context, cfg llm.Config) ([]llm.AgentModel, error) {
	cfg = cfg.Normalize()
	launch := cfg.Launch()
	if llm.IsCodexCLI(launch) {
		return llm.ListCodexModels(ctx, launch)
	}
	if llm.IsCursorCLI(launch) {
		return llm.ListCursorModels(ctx, launch)
	}
	return nil, nil
}

// AgentChatSend 向持久 ACP 会话发送一轮用户消息，流式片段通过 agent:chunk 事件推送。
func (p *PerfService) AgentChatSend(ctx context.Context, message string) (string, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return "", errors.New("消息为空")
	}
	sess, err := p.ensureChat()
	if err != nil {
		return "", err
	}
	p.mu.Lock()
	emit := p.emit
	p.mu.Unlock()
	return sess.Prompt(ctx, message, func(chunk string) {
		emit(AgentChunkEventName, AgentChunkEvent{Text: chunk})
	})
}

// AgentChatClose 结束当前 ACP 聊天会话。
func (p *PerfService) AgentChatClose() {
	p.mu.Lock()
	chat := p.chat
	p.chat = nil
	p.mu.Unlock()
	if chat != nil {
		chat.Close()
	}
}

func (p *PerfService) ensureChat() (*llm.ChatSession, error) {
	p.mu.Lock()
	if p.chat != nil {
		sess := p.chat
		p.mu.Unlock()
		return sess, nil
	}
	cfg := p.llmCfg
	p.mu.Unlock()

	sess := llm.NewChatSession(cfg)
	if err := sess.Start(context.Background()); err != nil {
		return nil, err
	}

	p.mu.Lock()
	if p.chat != nil {
		existing := p.chat
		p.mu.Unlock()
		sess.Close()
		return existing, nil
	}
	p.chat = sess
	p.mu.Unlock()
	return sess, nil
}

func (p *PerfService) LLMConfig() llm.Config {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.llmCfg.Normalize()
}

func (p *PerfService) Diagnose(ctx context.Context) (string, error) {
	report := p.Analyze()
	data, err := json.Marshal(report)
	if err != nil {
		return "", err
	}
	p.mu.Lock()
	client := llm.NewClient(p.llmCfg)
	p.mu.Unlock()
	return client.Diagnose(ctx, string(data))
}

// onFrame 只做入库与一次非阻塞通知。
//
// 绝不能在这里直接推送事件：事件推送要经过 webview，一旦卡住就会把读取循环拖死，
// 表现为界面显示"已连接"却永远 0 帧，同时连接还占着 Player 唯一的 Profiler 槽位。
func (p *PerfService) onFrame(f protocol.FrameData, hierarchy []protocol.SampleNode) {
	p.mu.Lock()
	recording := p.recording
	p.mu.Unlock()
	if !recording {
		return
	}
	p.session.AppendDetail(f, hierarchy)
	p.notifyUI()
}

func (p *PerfService) onRaw(body []byte) {
	p.mu.Lock()
	capture := p.captureRaw
	p.mu.Unlock()
	if !capture {
		return
	}
	p.session.AppendRaw(body)
}

func (p *PerfService) onClosed(err error) {
	p.mu.Lock()
	p.lastErr = ""
	if err != nil {
		p.lastErr = strings.TrimPrefix(err.Error(), "unityprofiler: ")
	}
	p.mu.Unlock()
	if err != nil {
		p.logf("采集连接关闭: %v", err)
	} else {
		p.logf("采集连接关闭")
	}
	p.notifyUI()
}

func importPercent(done, total int) int {
	if total <= 0 {
		if done > 0 {
			return 100
		}
		return 0
	}
	p := done * 100 / total
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

func (p *PerfService) emitImportProgress(fileName, phase string, done, total int, force bool, last *time.Time) {
	now := time.Now()
	if !force && last != nil && !last.IsZero() && now.Sub(*last) < importProgressMinInterval {
		return
	}
	if last != nil {
		*last = now
	}
	percent := 0
	if phase != "read" {
		percent = importPercent(done, total)
	}
	ev := ImportProgressEvent{
		Phase:    phase,
		Done:     done,
		Total:    total,
		Percent:  percent,
		FileName: fileName,
	}
	p.mu.Lock()
	emit := p.emit
	p.mu.Unlock()
	if emit != nil {
		emit(ImportProgressEventName, ev)
	}
}

// notifyUI 请求刷新界面；通道满即丢弃（合并为一次刷新），永不阻塞调用方。
func (p *PerfService) notifyUI() {
	select {
	case p.tickReq <- struct{}{}:
	default:
	}
}

// uiLoop 独占事件推送，是唯一允许被 webview 拖慢的 goroutine。
func (p *PerfService) uiLoop() {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	var last TickEvent
	var logAt time.Time
	for {
		select {
		case <-p.stopUI:
			return
		case <-p.tickReq:
		case <-ticker.C:
		}
		p.mu.Lock()
		recording := p.recording
		p.mu.Unlock()
		ev := TickEvent{
			TotalFrames: p.session.Total(),
			Retained:    p.session.Len(),
			Connected:   p.connected(),
			Recording:   recording,
		}
		if ev != last {
			last = ev
			p.mu.Lock()
			emit := p.emit
			p.mu.Unlock()
			emit(TickEventName, ev)
		}
		if ev.Connected && time.Since(logAt) >= time.Second {
			logAt = time.Now()
			p.logf("采集中: 帧=%d 保留=%d 解码=%+v", ev.TotalFrames, ev.Retained, p.client.DecodeStats())
		}
	}
}
