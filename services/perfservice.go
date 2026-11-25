// Package services 定义暴露给前端的 Wails 服务。
package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"unityperf/internal/adbforward"
	"unityperf/internal/analyzer"
	"unityperf/internal/capture"
	"unityperf/internal/collector"
	"unityperf/internal/device"
	"unityperf/internal/profileframe"
	"unityperf/internal/profilerproxy"
	"unityperf/internal/protocol"
	"unityperf/internal/screenshots"
	"unityperf/internal/unityprofiler"
	"unityperf/internal/upac"
)

// TickEventName 是采集进度事件名，前端据此增量刷新。
const TickEventName = "perf:tick"

// ImportProgressEventName 是加载存档解析进度事件名。
const ImportProgressEventName = "perf:import-progress"

const ExportProgressEventName = "perf:export-progress"

type ExportProgressEvent struct {
	Path    string `json:"path"`
	Percent int    `json:"percent"`
}

const importProgressMinInterval = 80 * time.Millisecond

const tickInterval = 250 * time.Millisecond

// TickEvent 是推送给前端的采集进度。
type TickEvent struct {
	Reconnecting bool   `json:"reconnecting"`
	LastError    string `json:"lastError"`
	TotalFrames  uint64 `json:"totalFrames"`
	Retained     int    `json:"retained"`
	Connected    bool   `json:"connected"`
	Recording    bool   `json:"recording"`
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
	// LastError 是最近一次采集异常中断的原因；正常断开、对端 EOF 自动重连或未出错时为空。
	// 解码失步这类问题必须让用户看见，否则界面只会停在"已连接但不再出帧"。
	LastError string `json:"lastError"`
	// Reconnecting 表示用户仍要保持采集，但对端已断开（典型是 Android native Activity
	// 盖住 Unity 时 PlayerConnection 直接 EOF），正在等游戏回到前台后自动重连。
	Reconnecting bool `json:"reconnecting"`
	// HasRaw 表示会话里有可导出的原始 Profiler 数据流。
	HasRaw bool `json:"hasRaw"`
}

// PerfService 是性能采集与分析的门面，供前端调用。
type PerfService struct {
	connectionMu      sync.Mutex
	mu                sync.Mutex
	client            *device.Client
	session           *collector.Session
	transport         string
	tunnel            *adbforward.Tunnel
	adbRun            adbforward.Runner
	emit              func(name string, data any)
	logger            *log.Logger
	logFile           *os.File
	recording         bool
	captureRaw        bool
	sourceFile        string
	loadedHS          protocol.Handshake
	capturePhone      *upac.PhoneInfo
	captureGeneration uint64
	deep              bool
	callStacks        bool
	lastErr           string

	// wantLive 表示用户点了连接且尚未点断开。对端 EOF 后仍为 true，会自动重连。
	wantLive        bool
	reconnecting    bool
	liveHS          protocol.Handshake
	usbSerial       string
	usbPackage      string
	liveHost        string
	livePort        int
	liveKind        string
	reconnectCancel context.CancelFunc
	incoming        *unityprofiler.AutoconnectListener
	incomingPort    int
	playerLogs      playerLogBuffer
	usbLogCancel    context.CancelFunc
	screen          screenState

	// tickReq 是"界面需要刷新"的合并信号，容量 1 且非阻塞投递。
	tickReq chan struct{}
	stopUI  chan struct{}
}

// reconnectInterval 是自动重连的间隔。native Activity 关掉后 Unity 会重建
// localabstract:Unity-{包名}，需要反复探测直到它回来。
var reconnectInterval = 400 * time.Millisecond

// NewPerfService 创建服务实例。
func NewPerfService() *PerfService {
	p := &PerfService{
		client:     device.NewClient(),
		session:    collector.NewSession(0),
		adbRun:     adbforward.DefaultRun,
		emit:       func(string, any) {},
		recording:  true,
		captureRaw: true,
		tickReq:    make(chan struct{}, 1),
		stopUI:     make(chan struct{}),
	}
	p.client.SetRawHandler(p.onRaw)
	p.client.SetLogHandler(p.onPlayerLog)
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
	p.startDefaultAutoconnect(defaultAutoconnectPort)
	return nil
}

func (p *PerfService) ServiceShutdown() error {
	p.Disconnect()
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
	f, err := os.OpenFile(filepath.Join(filepath.Dir(exe), "unityperf.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	p.mu.Lock()
	p.logFile = f
	p.logger = log.New(f, "", log.LstdFlags|log.Lmicroseconds)
	p.mu.Unlock()
	p.logf("=== unityperf 启动 ===")
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

// Connect 通过 PlayerConnection 直连指定 IP；端口为 0 时自动查找 Player。
func (p *PerfService) Connect(host string, port int) (protocol.Handshake, error) {
	if serial, viaADB := adbforward.ParseHost(host); viaADB {
		return p.ConnectUSB(serial, "")
	}
	host = strings.TrimSpace(host)
	if ip := net.ParseIP(host); ip == nil || ip.IsUnspecified() || ip.IsMulticast() {
		return protocol.Handshake{}, errors.New("请输入有效的手机 IP 地址")
	}
	if port < 0 || port > 65535 {
		return protocol.Handshake{}, errors.New("端口必须在 1–65535 之间，或使用 0 自动查找")
	}
	p.connectionMu.Lock()
	defer p.connectionMu.Unlock()
	p.mu.Lock()
	if p.connectedLocked() || (p.wantLive && p.incoming == nil) {
		p.mu.Unlock()
		return protocol.Handshake{}, errors.New("已有连接或正在重连，请先断开")
	}
	incoming, listenPort := p.incoming, p.incomingPort
	if incoming != nil {
		p.incoming = nil
		p.incomingPort = 0
		p.wantLive = false
		p.reconnecting = false
		p.transport = ""
	}
	p.mu.Unlock()
	if incoming != nil {
		incoming.Close()
	}
	hs, err := p.connectTCP(host, port, "tcp", true)
	if err != nil && incoming != nil {
		p.startDefaultAutoconnect(listenPort)
	}
	return hs, err
}

// ConnectUSB 通过 adb forward 连接 USB 手机上的 Development Player。
// packageID 为应用包名（如 com.company.game），用于 localabstract:Unity-{bundleId}。
// 只使用官方通道，不回退 tcp:55000。
func (p *PerfService) ConnectUSB(serial, packageID string) (protocol.Handshake, error) {
	p.connectionMu.Lock()
	defer p.connectionMu.Unlock()
	packageID = adbforward.NormalizePackageID(packageID)
	if err := adbforward.ValidatePackageID(packageID); err != nil {
		return protocol.Handshake{}, err
	}
	p.mu.Lock()
	if p.connectedLocked() {
		p.mu.Unlock()
		return protocol.Handshake{}, errors.New("已有连接，请先断开")
	}
	incoming, port := p.incoming, p.incomingPort
	if incoming != nil {
		p.incoming = nil
		p.incomingPort = 0
		p.wantLive = false
		p.reconnecting = false
		p.transport = ""
	} else if p.wantLive && p.reconnecting {
		hs := p.liveHS
		p.mu.Unlock()
		return hs, nil
	}
	p.mu.Unlock()
	if incoming != nil {
		incoming.Close()
	}
	hs, err := p.dialUSB(serial, packageID, true)
	if err != nil && incoming != nil {
		p.startDefaultAutoconnect(port)
	}
	return hs, err
}

func (p *PerfService) dialUSB(serial, packageID string, fresh bool) (protocol.Handshake, error) {
	p.mu.Lock()
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
		hs, err := p.connectTCP("127.0.0.1", tun.LocalPort, "adb", fresh)
		if err != nil {
			_ = adbforward.Remove(run, tun)
			wrapped := fmt.Errorf("adb forward 本机:%d -> %s 已建立。%w", tun.LocalPort, spec.Remote, err)
			if unityprofiler.IsPlayerReject(err) {
				return protocol.Handshake{}, wrapped
			}
			if errors.Is(err, unityprofiler.ErrPlayerSilent) {
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
		old := p.tunnel
		p.tunnel = &tun
		p.usbSerial = dev.Serial
		p.usbPackage = packageID
		p.liveHS = hs
		p.loadedHS = hs
		p.startUSBLogsLocked(run, dev.Serial, packageID)
		p.mu.Unlock()
		p.ConnectedPhones()
		if old != nil && (old.LocalPort != tun.LocalPort || old.Remote != tun.Remote) {
			_ = adbforward.Remove(run, *old)
		}
		return hs, nil
	}
	if lastErr == nil {
		lastErr = errors.New("adb forward 失败：未找到 localabstract:Unity-{包名}，请确认 Development Build 正在前台运行")
	}
	return protocol.Handshake{}, lastErr
}

func (p *PerfService) connectTCP(host string, port int, transport string, fresh bool) (protocol.Handshake, error) {
	p.mu.Lock()
	if p.connectedLocked() {
		p.mu.Unlock()
		return protocol.Handshake{}, errors.New("已有连接，请先断开")
	}
	if fresh {
		p.session.ClearAll()
		p.captureGeneration++
		p.capturePhone = nil
		p.resetScreenLocked(true)
		p.sourceFile = ""
		p.loadedHS = protocol.Handshake{}
	}
	p.transport = transport
	p.recording = true
	p.captureRaw = true
	p.lastErr = ""
	p.mu.Unlock()

	var hs protocol.Handshake
	var err error
	if transport == "tcp" && port == 0 {
		hs, port, err = p.client.ConnectDirect(host, p.onFrame, p.onClosed)
	} else {
		hs, err = p.client.Connect(host, port, p.onFrame, p.onClosed)
	}
	if err != nil {
		if fresh {
			p.mu.Lock()
			p.transport = ""
			p.mu.Unlock()
		}
		p.logf("连接 %s:%d(%s) 失败: %v", host, port, transport, err)
		return protocol.Handshake{}, err
	}
	p.logf("已连接 %s:%d(%s): 设备=%q 系统=%q 引擎=%q 流版本=%#x",
		host, port, transport, hs.DeviceName, hs.OS, hs.UnityVersion, hs.ProfilerStreamVersion)
	p.mu.Lock()
	deep, stacks := p.deep, p.callStacks
	if fresh {
		p.wantLive = true
	} else if !p.wantLive {
		p.mu.Unlock()
		p.client.Close()
		return protocol.Handshake{}, errors.New("已取消重连")
	}
	p.reconnecting = false
	p.liveHost = host
	p.livePort = port
	p.liveKind = transport
	p.liveHS = hs
	p.loadedHS = hs
	p.mu.Unlock()
	if deep || stacks {
		if err := p.client.SetProfilerOptions(deep, stacks); err != nil {
			p.logf("下发 Deep Profile/Call Stacks 失败: %v", err)
		}
	}
	if transport == "tcp" {
		if _, err := wirelessEndpoint(host); err == nil {
			p.ConnectedPhones()
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
	p.connectionMu.Lock()
	defer p.connectionMu.Unlock()
	p.mu.Lock()
	p.stopScreenLocked()
	incoming := p.incoming
	p.incoming = nil
	p.incomingPort = 0
	if p.usbLogCancel != nil {
		p.usbLogCancel()
		p.usbLogCancel = nil
	}
	p.wantLive = false
	p.reconnecting = false
	p.stopReconnectLocked()
	p.usbSerial = ""
	p.usbPackage = ""
	p.liveHost = ""
	p.livePort = 0
	p.liveKind = ""
	p.liveHS = protocol.Handshake{}
	p.mu.Unlock()
	if incoming != nil {
		incoming.Close()
	}
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
	liveHS := p.liveHS
	deep := p.deep
	callStacks := p.callStacks
	lastErr := p.lastErr
	reconnecting := p.reconnecting
	p.mu.Unlock()
	hs, _ := p.client.Handshake()
	if hs == (protocol.Handshake{}) {
		if liveHS != (protocol.Handshake{}) {
			hs = liveHS
		} else {
			hs = loadedHS
		}
	}
	connected := p.client.Connected()
	return Status{
		Connected:                  connected,
		Mode:                       "profiler",
		Transport:                  transport,
		Device:                     hs,
		TotalFrames:                p.session.Total(),
		Retained:                   p.session.Len(),
		UnitySupported:             (connected || reconnecting) && hs.Supported(),
		Recording:                  recording,
		SourceFile:                 sourceFile,
		DeepProfiling:              deep,
		CallStacks:                 callStacks,
		DeepProfilerSupported:      hs.DeepProfilerSupported,
		ManagedCallstacksSupported: hs.ManagedCallstacksSupported,
		LastError:                  lastErr,
		Reconnecting:               reconnecting,
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
	p.mu.Lock()
	if connected && p.captureRaw {
		p.session.ClearRecordedData()
	} else {
		p.session.ClearAll()
		p.captureGeneration++
		p.capturePhone = nil
	}
	p.resetScreenLocked(false)
	if connected || p.screen.epoch > 0 {
		p.screen.epoch = 1
	}
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

func (p *PerfService) FrameTimeline(index int, frameIndex uint64) []protocol.TimelineThread {
	frame, ok := p.session.FrameAt(index)
	if !ok || frame.Summary.FrameIndex != frameIndex {
		return nil
	}
	return frame.TimelineThreads()
}

func (p *PerfService) FrameTimelineContext(index int, frameIndex uint64) []protocol.TimelineFrame {
	const neighbors = 3
	if index < 0 {
		return nil
	}
	start := max(0, index-neighbors)
	frames := p.session.RangeFrames(start, index-start+neighbors+1)
	selected := index - start
	if selected >= len(frames) || frames[selected].Summary.FrameIndex != frameIndex {
		return nil
	}
	anchor := frames[selected]
	left, right := selected, selected
	for left > 0 && timelineFramesShareClock(frames[left-1], frames[left]) {
		left--
	}
	for right+1 < len(frames) && timelineFramesShareClock(frames[right], frames[right+1]) {
		right++
	}
	result := make([]protocol.TimelineFrame, right-left+1)
	for position := left; position <= right; position++ {
		frame := frames[position]
		result[position-left] = protocol.TimelineFrame{
			FrameIndex: frame.Summary.FrameIndex, DurationMs: timelineFrameDuration(frame),
			Selected: position == selected, Threads: frame.TimelineThreads(),
		}
	}
	center := selected - left
	for position := center - 1; position >= 0; position-- {
		result[position].OffsetMs = result[position+1].OffsetMs - result[position].DurationMs
	}
	for position := center + 1; position < len(result); position++ {
		result[position].OffsetMs = result[position-1].OffsetMs + result[position-1].DurationMs
	}
	if anchor.PlayerEndNS > anchor.PlayerStartNS {
		for position := range result {
			result[position].OffsetMs = float64(frames[left+position].PlayerStartNS-anchor.PlayerStartNS) / 1e6
		}
	}
	return result
}

func timelineFramesShareClock(previous, next profileframe.Frame) bool {
	if previous.ClockEpoch != next.ClockEpoch || previous.Summary.FrameIndex >= next.Summary.FrameIndex {
		return false
	}
	previousTimed := previous.PlayerEndNS > previous.PlayerStartNS
	nextTimed := next.PlayerEndNS > next.PlayerStartNS
	if previousTimed != nextTimed {
		return false
	}
	if previousTimed {
		return previous.PlayerEndNS <= next.PlayerStartNS
	}
	return previous.Summary.FrameIndex+1 == next.Summary.FrameIndex && timelineFrameDuration(previous) > 0 && timelineFrameDuration(next) > 0
}

func timelineFrameDuration(frame profileframe.Frame) float64 {
	if frame.PlayerEndNS > frame.PlayerStartNS {
		return float64(frame.PlayerEndNS-frame.PlayerStartNS) / 1e6
	}
	return float64(max(frame.Summary.CPUTimeMs, frame.Summary.MainThreadMs))
}

// ExportCapture stores the current PD3U stream as a native .upac archive.
func (p *PerfService) ExportCapture(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("导出路径为空")
	}
	if strings.EqualFold(filepath.Ext(path), ".data") {
		return errors.New("请通过「导出 Unity .data」选择 Editor，或保存为 .upac")
	}
	hs, _ := p.client.Handshake()
	p.mu.Lock()
	raw := p.session.Raw()
	if hs == (protocol.Handshake{}) {
		hs = p.liveHS
	}
	if p.sourceFile != "" || hs == (protocol.Handshake{}) {
		hs = p.loadedHS
	}
	phone := p.capturePhone
	screen := p.screenTrackLocked()
	emit := p.emit
	p.mu.Unlock()
	if len(raw) == 0 {
		return errors.New("没有可导出的原始 PD3U 数据")
	}
	lastUpdate := time.Time{}
	lastPercent := 0
	archive := upac.File{Phone: phone, Handshake: hs, Raw: raw, Screen: screen}
	if err := upac.SaveFileWithProgress(path, archive, func(done, total int) {
		percent := int(int64(done) * 99 / int64(total))
		if emit != nil && percent > lastPercent && (done == total || time.Since(lastUpdate) >= importProgressMinInterval) {
			lastPercent = percent
			lastUpdate = time.Now()
			emit(ExportProgressEventName, ExportProgressEvent{Path: path, Percent: percent})
		}
	}); err != nil {
		return fmt.Errorf("保存 UPAC 存档失败：%w", err)
	}
	p.logf("已导出 %d 个原始 PD3U 分块 -> %s", len(raw), path)
	return nil
}

func (p *PerfService) ExportUnityData(path, editor string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	count, err := capture.ExportWithEditor(ctx, strings.TrimSpace(editor), strings.TrimSpace(path), p.session.Raw())
	if err == nil {
		p.logf("Unity Editor 已导出 %d 帧 -> %s", count, path)
	}
	return count, err
}

// ImportCapture loads either a native .upac archive or a Unity Editor .data
// file. .upac is the native lossless PD3U session format.
func (p *PerfService) ImportCapture(path string) (int, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return 0, errors.New("加载路径为空")
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".upac":
		return p.importUPAC(path)
	case ".uppc":
		return p.importUPPC(path)
	default:
		return p.importUnityData(path)
	}
}

func (p *PerfService) importUnityData(path string) (int, error) {
	fileName := filepath.Base(path)
	last := time.Time{}
	p.emitImportProgress(fileName, "read", 0, 1, true, &last)
	file, err := capture.LoadFileWithProgress(path, func(done, total int64) {
		p.emitImportProgress(fileName, "parse", int(done), int(total), false, &last)
	})
	if err != nil {
		return 0, fmt.Errorf("无法读取 Unity Editor .data：%w", err)
	}
	if len(file.Frames) == 0 {
		return 0, errors.New("Profiler .data 没有可解析的帧")
	}
	p.emitImportProgress(fileName, "parse", 1, 1, true, &last)
	p.session.ReplaceFrames(file.Frames)
	return p.finishImport(path, file.Handshake, screenshots.Track{}, nil)
}

func (p *PerfService) importUPAC(path string) (int, error) {
	fileName := filepath.Base(path)
	last := time.Time{}
	p.emitImportProgress(fileName, "read", 0, 1, true, &last)
	file, err := upac.LoadFile(path)
	if err != nil {
		return 0, fmt.Errorf("无法读取 UPAC 存档：%w", err)
	}
	return p.importRawPD3U(path, fileName, file.Raw, file.Handshake, file.Screen, file.Phone, &last)
}

// importUPPC loads a transparent Editor↔Player proxy capture. It is a lossless
// PD3U source and preserves the counters unavailable in Unity Editor .data files.
func (p *PerfService) importUPPC(path string) (int, error) {
	fileName := filepath.Base(path)
	last := time.Time{}
	p.emitImportProgress(fileName, "read", 0, 1, true, &last)
	captureFile, err := profilerproxy.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("无法读取 UPPC 原始抓包：%w", err)
	}
	return p.importRawPD3U(path, fileName, captureFile.Bodies, handshakeFromPD3U(captureFile.Bodies), screenshots.Track{}, nil, &last)
}

func (p *PerfService) importRawPD3U(path, fileName string, raw [][]byte, handshake protocol.Handshake, screen screenshots.Track, phone *upac.PhoneInfo, last *time.Time) (int, error) {
	frames, err := unityprofiler.ReplayFramesWithProgress(raw, func(done, total int) {
		p.emitImportProgress(fileName, "parse", done, total, false, last)
	})
	if err != nil {
		return 0, fmt.Errorf("解析 %s 的 PD3U 数据失败：%w", filepath.Ext(path), err)
	}
	if len(frames) == 0 {
		return 0, fmt.Errorf("%s 存档没有可解析的帧", strings.TrimPrefix(strings.ToUpper(filepath.Ext(path)), "."))
	}
	p.emitImportProgress(fileName, "parse", len(raw), len(raw), true, last)
	p.session.ReplaceFrames(frames)
	p.session.SetRaw(raw)
	return p.finishImport(path, handshake, screen, phone)
}

func handshakeFromPD3U(raw [][]byte) protocol.Handshake {
	for _, body := range raw {
		header, err := unityprofiler.ParseStreamHeader(body)
		if err == nil {
			return protocol.Handshake{
				UnityVersion: header.EngineVersion, ProfilerStreamVersion: header.FormatVersion,
				MainThreadID: header.MainThreadID,
			}
		}
	}
	return protocol.Handshake{}
}

func (p *PerfService) finishImport(path string, handshake protocol.Handshake, screen screenshots.Track, phone *upac.PhoneInfo) (int, error) {
	connected := p.client.Connected()
	p.mu.Lock()
	p.resetScreenLocked(true)
	p.screen.track = screen
	p.sourceFile = path
	p.loadedHS = handshake
	p.captureGeneration++
	p.capturePhone = phone
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

func (p *PerfService) onFrame(frame profileframe.Frame) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.recording {
		return
	}
	if frame.PlayerEndNS > frame.PlayerStartNS {
		frame.ClockEpoch = p.screen.epoch
		p.screen.track.Observe(frame.ClockEpoch, time.Now().UnixNano(), frame.PlayerEndNS)
	}
	p.session.AppendFrame(frame)
	p.notifyUI()
}

func (p *PerfService) onRaw(body []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if unityprofiler.IsStreamHeader(body) {
		p.screen.epoch++
	}
	if p.captureRaw {
		p.session.AppendRaw(body)
	}
}

func (p *PerfService) onClosed(err error) {
	p.mu.Lock()
	want := p.wantLive
	if !want {
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
		return
	}
	if err == nil {
		// 本端 Close（重连前清理）。用户点断开时 wantLive 已是 false。
		p.mu.Unlock()
		return
	}
	if !unityprofiler.IsLostConnection(err) {
		p.wantLive = false
		p.reconnecting = false
		p.stopReconnectLocked()
		p.lastErr = strings.TrimPrefix(err.Error(), "unityprofiler: ")
		p.mu.Unlock()
		p.logf("采集连接关闭: %v", err)
		p.notifyUI()
		return
	}
	if p.reconnecting {
		p.mu.Unlock()
		p.logf("采集连接中断（%v），继续等待游戏恢复", err)
		p.notifyUI()
		return
	}
	p.reconnecting = true
	p.lastErr = ""
	ctx, cancel := context.WithCancel(context.Background())
	p.reconnectCancel = cancel
	p.mu.Unlock()
	p.logf("采集连接被游戏断开（%v），等待回到前台后自动重连", err)
	p.notifyUI()
	go p.reconnectLoop(ctx)
}

func (p *PerfService) stopReconnectLocked() {
	if p.reconnectCancel != nil {
		p.reconnectCancel()
		p.reconnectCancel = nil
	}
}

func (p *PerfService) reconnectLoop(ctx context.Context) {
	defer func() {
		p.mu.Lock()
		if ctx.Err() != nil {
			p.reconnecting = false
		}
		p.mu.Unlock()
		p.notifyUI()
	}()
	logged := false
	for {
		if ctx.Err() != nil {
			return
		}
		p.mu.Lock()
		if !p.wantLive || p.client.Connected() {
			p.reconnecting = false
			p.mu.Unlock()
			return
		}
		kind, serial, pkg := p.liveKind, p.usbSerial, p.usbPackage
		host, port := p.liveHost, p.livePort
		interval := reconnectInterval
		p.mu.Unlock()

		p.client.Close()
		var err error
		if kind == "adb" {
			_, err = p.dialUSB(serial, pkg, false)
		} else {
			_, err = p.connectTCP(host, port, kind, false)
		}
		if err == nil {
			p.logf("已恢复采集连接")
			return
		}
		if unityprofiler.IsPlayerReject(err) {
			p.mu.Lock()
			p.wantLive = false
			p.reconnecting = false
			p.lastErr = strings.TrimPrefix(err.Error(), "unityprofiler: ")
			p.mu.Unlock()
			p.logf("自动重连失败（Player 拒绝）: %v", err)
			p.notifyUI()
			return
		}
		if !logged {
			p.logf("等待游戏恢复采集: %v", err)
			logged = true
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
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
		reconnecting, lastErr := p.reconnecting, p.lastErr
		p.mu.Unlock()
		ev := TickEvent{
			Reconnecting: reconnecting,
			LastError:    lastErr,
			TotalFrames:  p.session.Total(),
			Retained:     p.session.Len(),
			Connected:    p.connected(),
			Recording:    recording,
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
