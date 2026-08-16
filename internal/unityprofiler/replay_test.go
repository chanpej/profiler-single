package unityprofiler

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
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
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(raw)
		mu.Unlock()
		if n >= 3 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	sess.Close()

	mu.Lock()
	chunks := append([][]byte(nil), raw...)
	mu.Unlock()
	if len(chunks) == 0 {
		t.Fatal("raw handler received no profiler bodies")
	}
	frames, hiers, err := Replay(chunks)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(frames) == 0 {
		t.Fatalf("replay of captured raw produced 0 frames from %d chunks", len(chunks))
	}
	if len(hiers) != len(frames) {
		t.Fatalf("frames=%d hierarchies=%d", len(frames), len(hiers))
	}
}
