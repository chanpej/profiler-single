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
	"unityperfagent/internal/collector"
	"unityperfagent/internal/device"
	"unityperfagent/internal/llm"
	"unityperfagent/internal/protocol"
	"unityperfagent/internal/unityprofiler"
)

// TickEventName 是采集进度事件名，前端据此增量刷新。
const TickEventName = "perf:tick"

const tickInterval = 250 * time.Millisecond

// TickEvent 是推送给前端的采集进度。
type TickEvent struct {
	TotalFrames uint64 `json:"totalFrames"`
	Retained    int    `json:"retained"`
	Connected   bool   `json:"connected"`
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
}

// PerfService 是性能采集与分析的门面，供前端调用。
type PerfService struct {
	mu        sync.Mutex
	client    *device.Client
	session   *collector.Session
	llmCfg    llm.Config
	transport string
	tunnel    *adbforward.Tunnel
	adbRun    adbforward.Runner
	emit      func(name string, data any)
	logger    *log.Logger
	logFile   *os.File

	// tickReq 是"界面需要刷新"的合并信号，容量 1 且非阻塞投递。
	tickReq chan struct{}
	stopUI  chan struct{}
}

// NewPerfService 创建服务实例。
func NewPerfService() *PerfService {
	p := &PerfService{
		client:  device.NewClient(),
		session: collector.NewSession(0),
		adbRun:  adbforward.DefaultRun,
		emit:    func(string, any) {},
		tickReq: make(chan struct{}, 1),
		stopUI:  make(chan struct{}),
	}
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
	p.session.Clear()
	p.transport = transport
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
	p.mu.Unlock()
	hs, _ := p.client.Handshake()
	connected := p.client.Connected()
	return Status{
		Connected:      connected,
		Mode:           "profiler",
		Transport:      transport,
		Device:         hs,
		TotalFrames:    p.session.Total(),
		Retained:       p.session.Len(),
		UnitySupported: connected && hs.Supported(),
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
	p.session.Clear()
	p.notifyUI()
}

func (p *PerfService) Analyze() analyzer.Report {
	return analyzer.Analyze(p.session.Range(0, p.session.Len()))
}

func (p *PerfService) SetLLMConfig(cfg llm.Config) {
	p.mu.Lock()
	p.llmCfg = cfg
	p.mu.Unlock()
}

func (p *PerfService) LLMConfig() llm.Config {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.llmCfg
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
func (p *PerfService) onFrame(f protocol.FrameData) {
	p.session.Append(f)
	p.notifyUI()
}

func (p *PerfService) onClosed(err error) {
	if err != nil {
		p.logf("采集连接关闭: %v", err)
	} else {
		p.logf("采集连接关闭")
	}
	p.notifyUI()
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
		ev := TickEvent{
			TotalFrames: p.session.Total(),
			Retained:    p.session.Len(),
			Connected:   p.connected(),
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
