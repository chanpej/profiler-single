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

	"unityperf/internal/profileframe"
	"unityperf/internal/protocol"
	"unityperf/internal/unityprofiler"
)

const testPackageID = "com.company.game"

func newTestService() *PerfService {
	return NewPerfService()
}

func TestFrameTimelineContextUsesPlayerClock(t *testing.T) {
	svc := newTestService()
	appendFrame := func(index uint64, start, end int64, epoch int) {
		svc.session.AppendFrame(profileframe.Frame{
			Summary:       protocol.FrameData{FrameIndex: index, CPUTimeMs: 1},
			MainThread:    []protocol.SampleNode{{Name: "PlayerLoop", TotalMs: 1}},
			PlayerStartNS: start * 1_000_000,
			PlayerEndNS:   end * 1_000_000,
			ClockEpoch:    epoch,
		})
	}
	appendFrame(10, 10, 18, 1)
	appendFrame(11, 18, 30, 1)
	appendFrame(14, 40, 55, 1)
	appendFrame(15, 55, 64, 1)
	appendFrame(16, 64, 79, 1)
	appendFrame(17, 79, 90, 1)
	appendFrame(18, 90, 99, 1)
	appendFrame(19, 99, 110, 1)
	appendFrame(20, 110, 125, 2)
	got := svc.FrameTimelineContext(2, 14)
	wantOffsets := []float64{-30, -22, 0, 15, 24, 39}
	if len(got) != len(wantOffsets) {
		t.Fatalf("context length = %d, want %d", len(got), len(wantOffsets))
	}
	for position, want := range wantOffsets {
		if frame := got[position]; frame.OffsetMs != want || frame.Selected != (position == 2) || len(frame.Threads) != 1 {
			t.Fatalf("frame %d = %+v, want offset %g", position, frame, want)
		}
	}
	if got[2].DurationMs != 15 {
		t.Fatalf("selected frame duration = %g, want 15", got[2].DurationMs)
	}
	if next := svc.FrameTimelineContext(7, 19); len(next) != 4 || next[len(next)-1].FrameIndex != 19 {
		t.Fatalf("context crossed clock epochs: %+v", next)
	}
	if stale := svc.FrameTimelineContext(2, 13); len(stale) != 0 {
		t.Fatalf("stale selection returned %+v", stale)
	}
}

func TestFrameTimelineContextForLegacyCapture(t *testing.T) {
	svc := newTestService()
	for _, summary := range []protocol.FrameData{
		{FrameIndex: 20, CPUTimeMs: 11},
		{FrameIndex: 21, CPUTimeMs: 15},
		{FrameIndex: 22, MainThreadMs: 19},
		{FrameIndex: 24, CPUTimeMs: 17},
	} {
		svc.session.AppendFrame(profileframe.New(summary, nil))
	}
	got := svc.FrameTimelineContext(1, 21)
	if len(got) != 3 || got[0].OffsetMs != -11 || got[1].OffsetMs != 0 || got[2].OffsetMs != 15 || got[2].DurationMs != 19 {
		t.Fatalf("legacy timeline offsets = %+v", got)
	}
}

func TestFrameTimelineContextFromCapture(t *testing.T) {
	path := os.Getenv("UNITYPROFILER_TIMELINE_CAPTURE")
	if path == "" {
		t.Skip("set UNITYPROFILER_TIMELINE_CAPTURE to a real .upac capture")
	}
	svc := newTestService()
	count, err := svc.ImportCapture(path)
	if err != nil {
		t.Fatal(err)
	}
	for index := 3; index+3 < count; index++ {
		frameIndex := svc.FrameAt(index).FrameIndex
		frames := svc.FrameTimelineContext(index, frameIndex)
		if len(frames) == 7 && frames[3].Selected && frames[0].OffsetMs < 0 && frames[6].OffsetMs > 0 &&
			len(frames[0].Threads) > 0 && len(frames[3].Threads) > 0 && len(frames[6].Threads) > 0 {
			t.Logf("capture frames=%d selected=%d context range %.3fms to %.3fms", count, frameIndex, frames[0].OffsetMs, frames[6].OffsetMs+frames[6].DurationMs)
			return
		}
	}
	t.Fatalf("no selection with sampled frames on both sides in %d frames", count)
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

func TestConnectRejectsInvalidDirectIP(t *testing.T) {
	svc := newTestService()
	if _, err := svc.Connect("192.168.1.20:55000", 0); err == nil || !strings.Contains(err.Error(), "IP") {
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
	svc.onFrame(profileframe.New(protocol.FrameData{FrameIndex: 1, FPS: 60}, nil))
	if svc.FrameCount() != 1 {
		t.Fatalf("count = %d", svc.FrameCount())
	}
	svc.SetRecording(false)
	if svc.Status().Recording {
		t.Fatal("expected recording off")
	}
	svc.onFrame(profileframe.New(protocol.FrameData{FrameIndex: 2, FPS: 60}, nil))
	if svc.FrameCount() != 1 {
		t.Fatalf("paused capture still stored frame: %d", svc.FrameCount())
	}
	svc.SetRecording(true)
	svc.onFrame(profileframe.New(protocol.FrameData{FrameIndex: 3, FPS: 60}, nil))
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
	var progress []ImportProgressEvent
	svc.emit = func(name string, data any) {
		if name == ImportProgressEventName {
			progress = append(progress, data.(ImportProgressEvent))
		}
	}
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
	hasIntermediateProgress := false
	for _, event := range progress {
		if event.Phase == "parse" && event.Percent > 0 && event.Percent < 100 {
			hasIntermediateProgress = true
			break
		}
	}
	if !hasIntermediateProgress {
		t.Fatalf("import progress lacks intermediate parse event: %+v", progress)
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

func TestImportUPPCReplaysFullMemoryCounters(t *testing.T) {
	path := filepath.Join("..", "test", "same-session.uppc")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Skip("same-session UPPC fixture is not available")
	} else if err != nil {
		t.Fatal(err)
	}

	svc := newTestService()
	n, err := svc.ImportCapture(path)
	if err != nil {
		t.Fatalf("ImportCapture: %v", err)
	}
	if n != 321 || !svc.Status().HasRaw || svc.Status().SourceFile != path {
		t.Fatalf("loaded session: frames=%d status=%+v", n, svc.Status())
	}
	frame := svc.FrameAt(21)
	if frame.CommittedMemMB <= 0 || frame.UsedMemMB <= 0 || frame.TextureMemMB <= 0 || frame.MemoryCounterMask == 0 {
		t.Fatalf("UPPC Memory frame = %+v", frame)
	}
}

func TestExportAndImportUPACReplaysRawPD3U(t *testing.T) {
	source := newTestService()
	var progress []ExportProgressEvent
	source.emit = func(name string, event any) {
		if name == ExportProgressEventName {
			progress = append(progress, event.(ExportProgressEvent))
		}
	}
	for _, body := range unityprofiler.SampleProfilerCapture() {
		source.onRaw(body)
	}
	path := filepath.Join(t.TempDir(), "session.upac")
	if err := source.ExportCapture(path); err != nil {
		t.Fatalf("ExportCapture: %v", err)
	}
	if len(progress) == 0 || progress[len(progress)-1].Percent != 99 {
		t.Fatalf("expected write progress before completion: %+v", progress)
	}
	previous := 0
	for _, event := range progress {
		if event.Path != path || event.Percent <= previous || event.Percent >= 100 {
			t.Fatalf("invalid write progress: %+v", progress)
		}
		previous = event.Percent
	}

	loaded := newTestService()
	n, err := loaded.ImportCapture(path)
	if err != nil {
		t.Fatalf("ImportCapture: %v", err)
	}
	if n != 1 || loaded.Status().HasRaw != true {
		t.Fatalf("loaded session: frames=%d status=%+v", n, loaded.Status())
	}
	frame := loaded.FrameAt(0)
	if frame.FrameIndex != 1 || frame.CPUTimeMs <= 0 {
		t.Fatalf("replayed frame = %+v", frame)
	}
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

func TestImportTruncatedUPACFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "truncated.upac")
	if err := os.WriteFile(path, []byte("UPAC"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := newTestService()
	if _, err := svc.ImportCapture(path); err == nil || !strings.Contains(err.Error(), "UPAC") {
		t.Fatalf("err=%v", err)
	}
}

func TestExportRejectsUnityDataExtension(t *testing.T) {
	svc := newTestService()
	if err := svc.ExportCapture(filepath.Join(t.TempDir(), "capture.data")); err == nil || !strings.Contains(err.Error(), ".upac") {
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
	svc.onFrame(profileframe.New(protocol.FrameData{FrameIndex: 1, FPS: 60}, []protocol.SampleNode{
		{Name: "Main Thread", TotalMs: 16, Children: []protocol.SampleNode{{Name: "PlayerController.Update", TotalMs: 4}}},
	}))
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
	if _, err := svc.Connect("127.0.0.1", port); err != nil {
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
