package services

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"unityperfagent/internal/llm"
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

func TestStatusEmptySession(t *testing.T) {
	svc := newTestService()
	st := svc.Status()
	if st.Connected || st.TotalFrames != 0 || st.Retained != 0 {
		t.Fatalf("status = %+v", st)
	}
	if st.Mode != "profiler" {
		t.Fatalf("mode = %q, want profiler", st.Mode)
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
