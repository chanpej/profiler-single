package unityprofiler

import (
	"errors"

	"strings"
	"sync"
	"testing"
	"time"

	"unityperfagent/internal/profileframe"
)

func TestReplayRebuildsHierarchyFromRaw(t *testing.T) {
	raw := SampleProfilerCapture()
	frames, hiers, err := Replay(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 {
		t.Fatalf("frames=%d, want 1", len(frames))
	}
	if frames[0].FrameIndex != 1 {
		t.Fatalf("FrameIndex=%d", frames[0].FrameIndex)
	}
	if len(hiers) != 1 || !hierarchyHas(hiers[0], "PlayerController.Update") {
		t.Fatalf("hierarchy=%+v", hiers)
	}
	alloc := findSample(hiers[0], "GC.Alloc")
	if alloc == nil || len(alloc.Callstack) == 0 {
		t.Fatalf("GC.Alloc callstack missing: %+v", alloc)
	}
	memoryFrames, err := ReplayFrames(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(memoryFrames) != 1 || memoryFrames[0].Summary.FrameIndex != 1 || !hierarchyHas(memoryFrames[0].MainThread, "PlayerController.Update") {
		t.Fatalf("memory frames = %+v", memoryFrames)
	}
}

func TestReplayMissingStreamHeaderFails(t *testing.T) {
	full := SampleProfilerCapture()
	if len(full) < 3 {
		t.Fatal("sample stream too short")
	}
	_, _, err := Replay(full[1:])
	if !errors.Is(err, ErrNoStreamHeader) {
		t.Fatalf("err=%v, want ErrNoStreamHeader", err)
	}
}

func TestReplayEmptyFails(t *testing.T) {
	_, _, err := Replay(nil)
	if err == nil {
		t.Fatal("empty stream should fail")
	}
}

func TestReplayWithProgressReportsChunks(t *testing.T) {
	raw := SampleProfilerCapture()
	var lastDone, lastTotal, calls int
	frames, _, err := ReplayWithProgress(raw, func(done, total int) {
		calls++
		lastDone, lastTotal = done, total
	})
	if err != nil {
		t.Fatal(err)
	}
	if lastTotal != len(raw) || lastDone != len(raw) {
		t.Fatalf("progress done=%d total=%d want %d", lastDone, lastTotal, len(raw))
	}
	if calls < 2 {
		t.Fatalf("progress calls=%d", calls)
	}
	if len(frames) != 1 {
		t.Fatalf("frames=%d", len(frames))
	}
}

func TestReplayUnsupportedStreamVersionFails(t *testing.T) {
	hdr := EncodeStreamHeader(StreamHeader{FormatVersion: 1, MainThreadID: fakeMainThreadID}, 2018, 4, 12, 2, 1)
	_, _, err := Replay([][]byte{hdr})
	if err == nil || !strings.Contains(err.Error(), "不受支持") {
		t.Fatalf("err=%v", err)
	}
}

func TestRawHandlerReceivesProfilerBodies(t *testing.T) {
	p := NewPlayer()
	p.Interval = 5 * time.Millisecond
	port, err := p.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	var mu sync.Mutex
	var raw [][]byte
	sess := NewSession()
	sess.SetRawHandler(func(body []byte) {
		cp := make([]byte, len(body))
		copy(cp, body)
		mu.Lock()
		raw = append(raw, cp)
		mu.Unlock()
	})
	if _, err := sess.Connect("127.0.0.1", port, nil, nil); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	var chunks [][]byte
	for time.Now().Before(deadline) {
		mu.Lock()
		chunks = append([][]byte(nil), raw...)
		mu.Unlock()
		if frames, hiers, err := Replay(chunks); err == nil && len(frames) > 0 {
			sess.Close()
			if len(hiers) != len(frames) {
				t.Fatalf("frames=%d hierarchies=%d", len(frames), len(hiers))
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	sess.Close()
	if len(chunks) == 0 {
		t.Fatal("raw handler received no profiler bodies")
	}
	_, _, err = Replay(chunks)
	t.Fatalf("Replay: %v", err)
}

func TestSessionSurvivesPlayerPause(t *testing.T) {
	old := readIdleTimeout
	readIdleTimeout = 80 * time.Millisecond
	defer func() { readIdleTimeout = old }()

	p := NewPlayer()
	p.Interval = 10 * time.Millisecond
	port, err := p.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	var mu sync.Mutex
	var n int
	closed := make(chan error, 1)
	sess := NewSession()
	if _, err := sess.Connect("127.0.0.1", port, func(_ profileframe.Frame) {
		mu.Lock()
		n++
		mu.Unlock()
	}, func(err error) { closed <- err }); err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	waitFrames := func(min int, d time.Duration) int {
		deadline := time.Now().Add(d)
		for time.Now().Before(deadline) {
			mu.Lock()
			got := n
			mu.Unlock()
			if got >= min {
				return got
			}
			time.Sleep(5 * time.Millisecond)
		}
		mu.Lock()
		defer mu.Unlock()
		return n
	}
	before := waitFrames(3, time.Second)
	if before < 3 {
		t.Fatalf("暂停前只收到 %d 帧", before)
	}

	p.SetPaused(true)
	time.Sleep(5 * readIdleTimeout) // 超过多次读等待，旧逻辑会在这里断开
	if !sess.Connected() {
		t.Fatal("Player 暂停上报后会话不应断开")
	}
	select {
	case err := <-closed:
		t.Fatalf("暂停期间不应关闭: %v", err)
	default:
	}

	p.SetPaused(false)
	after := waitFrames(before+3, time.Second)
	if after < before+3 {
		t.Fatalf("恢复后应继续出帧: pause前=%d 之后=%d", before, after)
	}
}
