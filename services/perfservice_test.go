package services

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"unityperfagent/internal/capture"
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

func TestExportImportCapture(t *testing.T) {
	svc := newTestService()
	svc.session.Append(protocol.FrameData{FrameIndex: 10, FPS: 60, CPUTimeMs: 16, DrawCalls: 100})
	svc.session.Append(protocol.FrameData{FrameIndex: 11, FPS: 55, CPUTimeMs: 18, DrawCalls: 120})
	dir := t.TempDir()
	path := filepath.Join(dir, "cap.upac")
	if err := svc.ExportCapture(""); err == nil {
		t.Fatal("empty path should fail")
	}
	if err := svc.ExportCapture(path); err != nil {
		t.Fatalf("ExportCapture: %v", err)
	}
	svc.Clear()
	if svc.FrameCount() != 0 {
		t.Fatal("clear failed")
	}
	n, err := svc.ImportCapture(path)
	if err != nil {
		t.Fatalf("ImportCapture: %v", err)
	}
	if n != 2 {
		t.Fatalf("imported %d, want 2", n)
	}
	st := svc.Status()
	if st.SourceFile != path {
		t.Fatalf("sourceFile = %q", st.SourceFile)
	}
	got := svc.Frames(0, 10)
	if len(got) != 2 || got[1].FrameIndex != 11 || got[1].DrawCalls != 120 {
		t.Fatalf("frames = %+v", got)
	}
}

func TestExportEmptyFails(t *testing.T) {
	svc := newTestService()
	if err := svc.ExportCapture(filepath.Join(t.TempDir(), "x.upac")); err == nil {
		t.Fatal("expected empty export error")
	}
}

func TestImportCaptureRestoresDevice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dev.upac")
	if err := capture.SaveFile(path, capture.File{
		Handshake: protocol.Handshake{DeviceName: "Pixel 7", UnityVersion: "2022.3.47f1"},
		Frames:    []protocol.FrameData{{FrameIndex: 9, FPS: 60}},
	}); err != nil {
		t.Fatal(err)
	}
	svc := newTestService()
	if _, err := svc.ImportCapture(path); err != nil {
		t.Fatal(err)
	}
	st := svc.Status()
	if st.Device.DeviceName != "Pixel 7" || st.Device.UnityVersion != "2022.3.47f1" {
		t.Fatalf("device = %+v", st.Device)
	}
}

func TestImportCapturePausesWhenConnectedFlag(t *testing.T) {
	svc := newTestService()
	svc.session.Append(protocol.FrameData{FrameIndex: 1, FPS: 60})
	path := filepath.Join(t.TempDir(), "cap.upac")
	if err := svc.ExportCapture(path); err != nil {
		t.Fatal(err)
	}
	svc.SetRecording(true)
	if _, err := svc.ImportCapture(path); err != nil {
		t.Fatal(err)
	}
	if !svc.Status().Recording {
		t.Fatal("offline import should keep recording flag when not connected")
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
	svc.onClosed(nil)
	if got := svc.Status().LastError; got != "" {
		t.Fatalf("正常断开应清空，实际 %q", got)
	}
}
