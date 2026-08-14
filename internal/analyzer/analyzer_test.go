package analyzer

import (
	"strings"
	"testing"

	"unityperfagent/internal/protocol"
)

// steadyFrames 生成 n 帧平稳的 60fps 数据。
func steadyFrames(n int) []protocol.FrameData {
	frames := make([]protocol.FrameData, n)
	for i := range frames {
		frames[i] = protocol.FrameData{
			FrameIndex:  uint64(i),
			TimestampMs: int64(i) * 16,
			FPS:         60,
			CPUTimeMs:   16,
			DrawCalls:   200,
			GCAllocKB:   4,
			UsedMemMB:   512,
		}
	}
	return frames
}

func hasRule(issues []Issue, rule string) bool {
	for _, is := range issues {
		if is.Rule == rule {
			return true
		}
	}
	return false
}

func TestAnalyzeEmpty(t *testing.T) {
	r := Analyze(nil)
	if r.FrameCount != 0 || len(r.Issues) != 0 {
		t.Fatalf("empty analyze = %+v", r)
	}
	if r.Summary == "" {
		t.Fatal("summary should not be empty")
	}
}

func TestAnalyzeCleanRun(t *testing.T) {
	r := Analyze(steadyFrames(120))
	if len(r.Issues) != 0 {
		t.Fatalf("expected no issues, got %+v", r.Issues)
	}
	if !strings.Contains(r.Summary, "未发现") {
		t.Fatalf("summary = %q", r.Summary)
	}
}

func TestDetectFrameSpike(t *testing.T) {
	frames := steadyFrames(100)
	frames[50].CPUTimeMs = 120 // 明显尖峰
	r := Analyze(frames)
	if !hasRule(r.Issues, "frame_spike") {
		t.Fatalf("expected frame_spike, got %+v", r.Issues)
	}
	for _, is := range r.Issues {
		if is.Rule == "frame_spike" && is.FrameIndex != 50 {
			t.Fatalf("spike frame = %d, want 50", is.FrameIndex)
		}
	}
}

func TestFrameSpikeCapped(t *testing.T) {
	frames := steadyFrames(200)
	for i := range frames {
		if i%2 == 0 {
			frames[i].CPUTimeMs = 200
		}
	}
	r := Analyze(frames)
	count := 0
	for _, is := range r.Issues {
		if is.Rule == "frame_spike" {
			count++
		}
	}
	if count > maxReportedSpikes {
		t.Fatalf("spike issues = %d, want <= %d", count, maxReportedSpikes)
	}
}

func TestDetectGCSpike(t *testing.T) {
	frames := steadyFrames(60)
	frames[10].GCAllocKB = 1024
	r := Analyze(frames)
	if !hasRule(r.Issues, "gc_spike") {
		t.Fatalf("expected gc_spike, got %+v", r.Issues)
	}
}

func TestDetectLowFPS(t *testing.T) {
	frames := steadyFrames(60)
	for i := range frames {
		frames[i].FPS = 20
		frames[i].CPUTimeMs = 50
	}
	r := Analyze(frames)
	if !hasRule(r.Issues, "low_fps") {
		t.Fatalf("expected low_fps, got %+v", r.Issues)
	}
}

func TestDetectHighDrawCalls(t *testing.T) {
	frames := steadyFrames(60)
	frames[30].DrawCalls = 1500
	r := Analyze(frames)
	found := false
	for _, is := range r.Issues {
		if is.Rule == "high_drawcalls" {
			found = true
			if is.Severity != SeverityCritical {
				t.Fatalf("severity = %s, want critical", is.Severity)
			}
		}
	}
	if !found {
		t.Fatalf("expected high_drawcalls, got %+v", r.Issues)
	}
}

func TestDetectMemoryGrowth(t *testing.T) {
	frames := steadyFrames(120)
	// 2 秒内增长 40MB → 1200MB/分钟，远超阈值
	for i := range frames {
		frames[i].UsedMemMB = 512 + float32(i)/3
	}
	r := Analyze(frames)
	if !hasRule(r.Issues, "memory_growth") {
		t.Fatalf("expected memory_growth, got %+v", r.Issues)
	}
}

func TestMemoryGrowthNeedsEnoughFrames(t *testing.T) {
	frames := steadyFrames(10)
	for i := range frames {
		frames[i].UsedMemMB = 512 + float32(i*100)
	}
	if issues := detectMemoryGrowth(frames); issues != nil {
		t.Fatalf("should skip short sessions, got %+v", issues)
	}
}

func TestMedian(t *testing.T) {
	if m := median(nil); m != 0 {
		t.Fatalf("median(nil) = %v", m)
	}
	if m := median([]float64{3, 1, 2}); m != 2 {
		t.Fatalf("median odd = %v", m)
	}
	if m := median([]float64{4, 1, 3, 2}); m != 2.5 {
		t.Fatalf("median even = %v", m)
	}
}

func TestSummaryCountsSeverities(t *testing.T) {
	frames := steadyFrames(60)
	frames[10].GCAllocKB = 1024
	frames[20].DrawCalls = 1500
	r := Analyze(frames)
	if !strings.Contains(r.Summary, "发现") {
		t.Fatalf("summary = %q", r.Summary)
	}
}
