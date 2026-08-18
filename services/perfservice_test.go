package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"unityperfagent/internal/llm"
	"unityperfagent/internal/protocol"
	"unityperfagent/internal/unityprofiler"
)

const testPackageID = "com.company.game"

func newTestService() *PerfService {
	return NewPerfService()
}

func TestConnectFailure(t *testing.T) {
	svc := newTestService()
	svc.adbRun = func(_ context.Context, args ...string) (string, error) {
		return "List of devices attached\n", nil
	}
	if _, err := svc.ConnectUSB("", testPackageID); err == nil {
		t.Fatal("expected connect error")
	}
	if svc.Status().Connected {
		t.Fatal("should not be connected")
	}
}

func TestLLMConfigRoundTrip(t *testing.T) {
	svc := newTestService()
	cfg := llm.Config{AgentCommand: "gemini", AgentArgs: []string{"--experimental-acp"}}
	svc.SetLLMConfig(cfg)
	got := svc.LLMConfig()
	if got.AgentCommand != "gemini" || len(got.AgentArgs) != 1 {
		t.Fatalf("cfg = %+v", got)
	}
	if got.ActiveID != "custom" {
		t.Fatalf("active = %q", got.ActiveID)
	}
	if len(got.Agents) < 3 {
		t.Fatalf("agents = %+v", got.Agents)
	}
}

func TestLLMConfigDefaults(t *testing.T) {
	svc := newTestService()
	got := svc.LLMConfig()
	if got.ActiveID != "codex" || len(got.Agents) != 2 {
		t.Fatalf("defaults = %+v", got)
	}
	if got.AgentCommand != "codex" {
		t.Fatalf("codex command = %q", got.AgentCommand)
	}
}

func TestDiagnoseWithoutLLMConfig(t *testing.T) {
	svc := newTestService()
	_, err := svc.Diagnose(t.Context())
	if err == nil {
		t.Fatal("expected error without llm config")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("err = %v", err)
	}
}

func TestAgentChatSendWithoutConfig(t *testing.T) {
	svc := newTestService()
	_, err := svc.AgentChatSend(t.Context(), "hello")
	if err == nil {
		t.Fatal("expected error without llm config")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("err = %v", err)
	}
}

func TestAgentChatSendEmptyMessage(t *testing.T) {
	svc := newTestService()
	if _, err := svc.AgentChatSend(t.Context(), "  "); err == nil {
		t.Fatal("expected error for empty message")
	}
}

func TestSetLLMConfigClosesChat(t *testing.T) {
	svc := newTestService()
	svc.SetLLMConfig(llm.Config{AgentCommand: "gemini"})
	svc.AgentChatClose()
	got := svc.LLMConfig()
	if got.AgentCommand != "gemini" {
		t.Fatalf("cfg = %+v", got)
	}
}

func TestListAgentModelsSkipsUnknownAgent(t *testing.T) {
	svc := newTestService()
	got, err := svc.ListAgentModels(t.Context(), llm.Config{AgentCommand: "gemini"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("unknown agent should not probe: %+v", got)
	}
}

func TestListAgentModelsCursorMissingBinary(t *testing.T) {
	svc := newTestService()
	_, err := svc.ListAgentModels(t.Context(), llm.Config{
		ActiveID: "cursor",
		Agents:   []llm.AgentProfile{{ID: "cursor", Name: "Cursor CLI", Command: "definitely-not-a-real-command-upa", Args: []string{"acp"}}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListAgentModelsMissingBinary(t *testing.T) {
	svc := newTestService()
	_, err := svc.ListAgentModels(t.Context(), llm.Config{
		ActiveID: "codex",
		Agents:   []llm.AgentProfile{{ID: "codex", Name: "Codex", Command: "definitely-not-a-real-command-upa"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestStatusEmptySession(t *testing.T) {
	svc := newTestService()
	st := svc.Status()
	if st.Connected || st.TotalFrames != 0 || st.Retained != 0 {
		t.Fatalf("status = %+v", st)
	}
	if st.Mode != "profiler" {
		t.Fatalf("mode = %q, want profiler", st.Mode)
	}
	if !st.Recording {
		t.Fatal("recording should default to true")
	}
	if frames := svc.Frames(0, 10); frames != nil {
		t.Fatalf("frames = %v", frames)
	}
}

func TestConnectRejectsWiFi(t *testing.T) {
	svc := newTestService()
	_, err := svc.Connect("192.168.1.20", 55000)
	if err == nil || !strings.Contains(err.Error(), "Wi-Fi") {
		t.Fatalf("err=%v", err)
	}
}

func TestConnectUSBRequiresPackageID(t *testing.T) {
	svc := newTestService()
	_, err := svc.ConnectUSB("", "")
	if err == nil || !strings.Contains(err.Error(), "包名") {
		t.Fatalf("err=%v", err)
	}
}

func TestConnectUSBForwardFails(t *testing.T) {
	svc := newTestService()
	svc.adbRun = func(_ context.Context, args ...string) (string, error) {
		cmd := strings.Join(args, " ")
		switch {
		case strings.Contains(cmd, "devices"):
			return "List of devices attached\n7ddbdf55 device model:Mi_MIX_2S\n", nil
		case strings.Contains(cmd, "forward --list"):
			return "", nil
		case strings.Contains(cmd, "forward"):
			return "", fmt.Errorf("adb forward rejected")
		default:
			return "", fmt.Errorf("unexpected adb: %s", cmd)
		}
	}
	_, err := svc.ConnectUSB("", testPackageID)
	if err == nil {
		t.Fatal("expected forward error")
	}
}

func TestDiscoverPlayersIncludesUSB(t *testing.T) {
	svc := newTestService()
	svc.adbRun = func(_ context.Context, args ...string) (string, error) {
		cmd := strings.Join(args, " ")
		if strings.Contains(cmd, "devices") {
			return "List of devices attached\n7ddbdf55 device model:Mi_MIX_2S\n", nil
		}
		if strings.Contains(cmd, "/proc/net/tcp") {
			return "  sl  local_address rem_address   st\n   0: 00000000:D6D8 00000000:0000 0A 0 0 0 0 0 0 0 0 0 0 0 0 0 0\n", nil
		}
		return "", fmt.Errorf("unexpected adb: %s", cmd)
	}
	players, err := svc.DiscoverPlayers()
	if err != nil {
		t.Fatalf("DiscoverPlayers: %v", err)
	}
	var usb *unityprofiler.PlayerInfo
	for i := range players {
		if players[i].Transport == "adb" {
			usb = &players[i]
			break
		}
	}
	if usb == nil || usb.IP != "adb:7ddbdf55" {
		t.Fatalf("usb player missing: %+v", players)
	}
}

func TestConnectUSBNoDevice(t *testing.T) {
	svc := newTestService()
	svc.adbRun = func(_ context.Context, args ...string) (string, error) {
		return "List of devices attached\n", nil
	}
	_, err := svc.ConnectUSB("", testPackageID)
	if err == nil || !strings.Contains(err.Error(), "USB") {
		t.Fatalf("err=%v", err)
	}
}

func TestSetRecordingPausesCapture(t *testing.T) {
	svc := newTestService()
	svc.onFrame(protocol.FrameData{FrameIndex: 1, FPS: 60}, nil)
	if svc.FrameCount() != 1 {
		t.Fatalf("count = %d", svc.FrameCount())
	}
	svc.SetRecording(false)
	if svc.Status().Recording {
		t.Fatal("expected recording off")
	}
	svc.onFrame(protocol.FrameData{FrameIndex: 2, FPS: 60}, nil)
	if svc.FrameCount() != 1 {
		t.Fatalf("paused capture still stored frame: %d", svc.FrameCount())
	}
	svc.SetRecording(true)
	svc.onFrame(protocol.FrameData{FrameIndex: 3, FPS: 60}, nil)
	if svc.FrameCount() != 2 {
		t.Fatalf("resume count = %d", svc.FrameCount())
	}
}

func TestImportOfficialUnityData(t *testing.T) {
	path := filepath.Join("..", "test", "testdata", "unity-profiler.data")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Skip("official Unity .data fixture is not available")
	} else if err != nil {
		t.Fatal(err)
	}
	svc := newTestService()
	n, err := svc.ImportCapture(path)
	if err != nil {
		t.Fatalf("ImportCapture: %v", err)
	}
	if n != 299 {
		t.Fatalf("imported %d, want 299", n)
	}
	st := svc.Status()
	if st.SourceFile != path || st.Device.UnityVersion != "2022.3.47f1" {
		t.Fatalf("status = %+v", st)
	}
	frames := svc.Frames(0, 1)
	if len(frames) != 1 || frames[0].FrameIndex != 1 || frames[0].FPS < 29 || frames[0].FPS > 30 || frames[0].CPUScriptsMs <= 0 {
		t.Fatalf("frames = %+v", frames)
	}
	hierarchy := svc.FrameHierarchy(0)
	if len(hierarchy) == 0 || hierarchy[0].Name != "Main Thread" || !hierarchyHas(hierarchy, "PlayerLoop") {
		t.Fatalf("hierarchy = %+v", hierarchy)
	}
	if svc.Status().HasRaw {
		t.Fatal("official .data must not be treated as a PD3U raw stream")
	}
}

func hierarchyHas(nodes []protocol.SampleNode, name string) bool {
	for _, node := range nodes {
		if node.Name == name || hierarchyHas(node.Children, name) {
			return true
		}
	}
	return false
}

func TestImportPercent(t *testing.T) {
	if got := importPercent(0, 0); got != 0 {
		t.Fatalf("empty=%d", got)
	}
	if got := importPercent(1, 0); got != 100 {
		t.Fatalf("done without total=%d", got)
	}
	if got := importPercent(1, 4); got != 25 {
		t.Fatalf("quarter=%d", got)
	}
	if got := importPercent(4, 4); got != 100 {
		t.Fatalf("full=%d", got)
	}
}

func TestImportCaptureEmitsProgress(t *testing.T) {
	path := filepath.Join("..", "test", "testdata", "unity-profiler.data")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Skip("official Unity .data fixture is not available")
	} else if err != nil {
		t.Fatal(err)
	}
	svc := newTestService()
	var mu sync.Mutex
	var events []ImportProgressEvent
	svc.mu.Lock()
	svc.emit = func(name string, data any) {
		if name != ImportProgressEventName {
			return
		}
		ev, ok := data.(ImportProgressEvent)
		if !ok {
			return
		}
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
	}
	svc.mu.Unlock()
	if _, err := svc.ImportCapture(path); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(events) == 0 {
		t.Fatal("expected import progress events")
	}
	if events[0].Phase != "read" {
		t.Fatalf("first phase=%s", events[0].Phase)
	}
	last := events[len(events)-1]
	if last.Phase != "parse" || last.Percent != 100 || last.FileName != "unity-profiler.data" {
		t.Fatalf("last=%+v", last)
	}
}

func TestImportUPACFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.upac")
	if err := os.WriteFile(path, []byte("UPAC"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := newTestService()
	if _, err := svc.ImportCapture(path); err == nil || !strings.Contains(err.Error(), "格式版本") {
		t.Fatalf("err=%v", err)
	}
}

func TestExportRejectsPseudoData(t *testing.T) {
	svc := newTestService()
	if err := svc.ExportCapture(filepath.Join(t.TempDir(), "capture.data")); err == nil || !strings.Contains(err.Error(), "PD3U") {
		t.Fatalf("err=%v", err)
	}
}

func TestSetDeepProfilingAndCallStacks(t *testing.T) {
	svc := newTestService()
	if err := svc.SetDeepProfiling(true); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetCallStacks(true); err != nil {
		t.Fatal(err)
	}
	st := svc.Status()
	if !st.DeepProfiling || !st.CallStacks {
		t.Fatalf("status=%+v", st)
	}
	svc.onFrame(protocol.FrameData{FrameIndex: 1, FPS: 60}, []protocol.SampleNode{
		{Name: "Main Thread", TotalMs: 16, Children: []protocol.SampleNode{{Name: "PlayerController.Update", TotalMs: 4}}},
	})
	h := svc.FrameHierarchy(0)
	if len(h) != 1 || h[0].Name != "Main Thread" || len(h[0].Children) != 1 {
		t.Fatalf("hierarchy=%+v", h)
	}
	if svc.FrameHierarchy(9) != nil {
		t.Fatal("missing frame should return nil")
	}
}

// 采集异常中断的原因要能带到界面上：解码失步不能只留在日志里。
func TestStatusCarriesLastError(t *testing.T) {
	svc := newTestService()
	if svc.Status().LastError != "" {
		t.Fatal("初始不应有错误")
	}
	svc.onClosed(errors.New("数据流解析失步"))
	if got := svc.Status().LastError; got != "数据流解析失步" {
		t.Fatalf("lastError=%q", got)
	}
	svc.onClosed(fmt.Errorf("unityprofiler: Profiler 数据流解析失步，已停止采集：main thread"))
	if got := svc.Status().LastError; got != "Profiler 数据流解析失步，已停止采集：main thread" {
		t.Fatalf("lastError=%q", got)
	}
	svc.onClosed(nil)
	if got := svc.Status().LastError; got != "" {
		t.Fatalf("正常断开应清空，实际 %q", got)
	}
}

func TestEOFStartsReconnectWithoutErrorPopup(t *testing.T) {
	old := reconnectInterval
	reconnectInterval = 30 * time.Millisecond
	defer func() { reconnectInterval = old }()

	player := unityprofiler.NewPlayer()
	player.Interval = 8 * time.Millisecond
	port, err := player.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer player.Stop()

	svc := newTestService()
	defer svc.Disconnect()
	if _, err := svc.connectTCP("127.0.0.1", port, "tcp", true); err != nil {
		t.Fatal(err)
	}
	waitFrames := func(min int, d time.Duration) int {
		deadline := time.Now().Add(d)
		for time.Now().Before(deadline) {
			if n := svc.FrameCount(); n >= min {
				return n
			}
			time.Sleep(5 * time.Millisecond)
		}
		return svc.FrameCount()
	}
	before := waitFrames(3, time.Second)
	if before < 3 {
		t.Fatalf("重连前只收到 %d 帧", before)
	}

	player.DropClients()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		st := svc.Status()
		if st.Reconnecting || st.Connected {
			if strings.Contains(strings.ToUpper(st.LastError), "EOF") {
				t.Fatalf("EOF 自动重连不应弹错误: %q", st.LastError)
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if le := svc.Status().LastError; strings.Contains(strings.ToUpper(le), "EOF") {
		t.Fatalf("LastError=%q", le)
	}

	after := waitFrames(before+3, 2*time.Second)
	if after < before+3 {
		t.Fatalf("native 断开后应自动恢复出帧: 之前=%d 之后=%d reconnecting=%v connected=%v err=%q",
			before, after, svc.Status().Reconnecting, svc.Status().Connected, svc.Status().LastError)
	}
	if svc.Status().Reconnecting {
		t.Fatal("恢复后不应仍处于重连")
	}
}

func TestDisconnectStopsReconnect(t *testing.T) {
	old := reconnectInterval
	reconnectInterval = 20 * time.Millisecond
	defer func() { reconnectInterval = old }()

	player := unityprofiler.NewPlayer()
	player.Interval = 8 * time.Millisecond
	port, err := player.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer player.Stop()

	svc := newTestService()
	if _, err := svc.connectTCP("127.0.0.1", port, "tcp", true); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && svc.FrameCount() < 2 {
		time.Sleep(5 * time.Millisecond)
	}
	player.DropClients()
	time.Sleep(50 * time.Millisecond)
	svc.Disconnect()
	time.Sleep(80 * time.Millisecond)
	if svc.Status().Connected || svc.Status().Reconnecting {
		t.Fatalf("用户断开后不应自动重连: %+v", svc.Status())
	}
}
