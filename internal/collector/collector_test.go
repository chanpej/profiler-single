package collector

import (
	"math"
	"sync"
	"testing"

	"unityperfagent/internal/protocol"
)

func frameN(i int) protocol.FrameData {
	return protocol.FrameData{
		FrameIndex: uint64(i),
		FPS:        60,
		CPUTimeMs:  float32(10 + i%5),
		DrawCalls:  uint32(100 + i),
		GCAllocKB:  1,
		UsedMemMB:  float32(500 + i),
	}
}

func TestAppendAndRange(t *testing.T) {
	s := NewSession(10)
	for i := 0; i < 5; i++ {
		s.Append(frameN(i))
	}
	if s.Len() != 5 {
		t.Fatalf("Len = %d, want 5", s.Len())
	}
	got := s.Range(1, 3)
	if len(got) != 3 {
		t.Fatalf("Range len = %d, want 3", len(got))
	}
	for i, f := range got {
		if f.FrameIndex != uint64(i+1) {
			t.Fatalf("Range[%d].FrameIndex = %d, want %d", i, f.FrameIndex, i+1)
		}
	}
}

func TestRingBufferEviction(t *testing.T) {
	s := NewSession(4)
	for i := 0; i < 10; i++ {
		s.Append(frameN(i))
	}
	if s.Len() != 4 {
		t.Fatalf("Len = %d, want 4", s.Len())
	}
	if s.Total() != 10 {
		t.Fatalf("Total = %d, want 10", s.Total())
	}
	got := s.Range(0, 4)
	for i, f := range got {
		if f.FrameIndex != uint64(6+i) {
			t.Fatalf("Range[%d].FrameIndex = %d, want %d", i, f.FrameIndex, 6+i)
		}
	}
}

func TestRangeBounds(t *testing.T) {
	s := NewSession(8)
	for i := 0; i < 3; i++ {
		s.Append(frameN(i))
	}
	if got := s.Range(5, 10); got != nil {
		t.Fatalf("out-of-range should return nil, got %v", got)
	}
	if got := s.Range(-1, 2); len(got) != 2 || got[0].FrameIndex != 0 {
		t.Fatalf("negative offset should clamp to 0, got %v", got)
	}
	if got := s.Range(2, 100); len(got) != 1 {
		t.Fatalf("limit should be clamped, got len %d", len(got))
	}
	if got := s.Range(0, 0); got != nil {
		t.Fatalf("zero limit should return nil, got %v", got)
	}
}

func TestAt(t *testing.T) {
	s := NewSession(4)
	if _, ok := s.At(0); ok {
		t.Fatal("empty session At should miss")
	}
	for i := 0; i < 6; i++ {
		s.Append(frameN(i))
	}
	// 容量 4，保留帧 2..5。
	got, ok := s.At(0)
	if !ok || got.FrameIndex != 2 {
		t.Fatalf("At(0) = %+v ok=%v, want frame 2", got, ok)
	}
	got, ok = s.At(3)
	if !ok || got.FrameIndex != 5 {
		t.Fatalf("At(3) = %+v ok=%v, want frame 5", got, ok)
	}
	if _, ok := s.At(-1); ok {
		t.Fatal("At(-1) should miss")
	}
	if _, ok := s.At(4); ok {
		t.Fatal("At(4) should miss")
	}
}

func TestLatest(t *testing.T) {
	s := NewSession(16)
	for i := 0; i < 10; i++ {
		s.Append(frameN(i))
	}
	got := s.Latest(3)
	if len(got) != 3 {
		t.Fatalf("Latest len = %d, want 3", len(got))
	}
	if got[0].FrameIndex != 7 || got[2].FrameIndex != 9 {
		t.Fatalf("Latest order wrong: %v", got)
	}
	if got := s.Latest(100); len(got) != 10 {
		t.Fatalf("Latest(100) len = %d, want 10", len(got))
	}
}

func TestClear(t *testing.T) {
	s := NewSession(8)
	s.Append(frameN(0))
	s.Clear()
	if s.Len() != 0 || s.Total() != 0 {
		t.Fatalf("Clear failed: len=%d total=%d", s.Len(), s.Total())
	}
}

func TestReplace(t *testing.T) {
	s := NewSession(8)
	s.Append(frameN(99))
	frames := []protocol.FrameData{frameN(1), frameN(2), frameN(3)}
	s.Replace(frames)
	if s.Len() != 3 || s.Total() != 3 {
		t.Fatalf("len=%d total=%d", s.Len(), s.Total())
	}
	got := s.Range(0, 3)
	for i, f := range got {
		if f.FrameIndex != uint64(i+1) {
			t.Fatalf("Range[%d] = %d", i, f.FrameIndex)
		}
	}
}

func TestReplaceOverflowKeepsLatest(t *testing.T) {
	s := NewSession(4)
	frames := make([]protocol.FrameData, 10)
	for i := 0; i < 10; i++ {
		frames[i] = frameN(i)
	}
	s.Replace(frames)
	if s.Len() != 4 || s.Total() != 4 {
		t.Fatalf("len=%d total=%d, want 4/4", s.Len(), s.Total())
	}
	got := s.Range(0, 4)
	for i, f := range got {
		if f.FrameIndex != uint64(6+i) {
			t.Fatalf("Range[%d] = %d, want %d", i, f.FrameIndex, 6+i)
		}
	}
}

func TestReplaceEmpty(t *testing.T) {
	s := NewSession(8)
	s.Append(frameN(1))
	s.Replace(nil)
	if s.Len() != 0 || s.Total() != 0 {
		t.Fatalf("empty replace: len=%d total=%d", s.Len(), s.Total())
	}
}

func TestStats(t *testing.T) {
	s := NewSession(100)
	// 帧: fps 30/60, cpu 10/20
	for i := 0; i < 10; i++ {
		f := protocol.FrameData{FPS: 60, CPUTimeMs: 10, DrawCalls: 100, GCAllocKB: 2, UsedMemMB: 512}
		if i%2 == 1 {
			f.FPS, f.CPUTimeMs, f.DrawCalls = 30, 20, 200
		}
		s.Append(f)
	}
	st := s.Stats()
	if st.FrameCount != 10 {
		t.Fatalf("FrameCount = %d", st.FrameCount)
	}
	if math.Abs(st.AvgFPS-45) > 1e-9 {
		t.Fatalf("AvgFPS = %v, want 45", st.AvgFPS)
	}
	if st.MinFPS != 30 || st.MaxFPS != 60 {
		t.Fatalf("Min/Max FPS = %v/%v", st.MinFPS, st.MaxFPS)
	}
	if math.Abs(st.AvgCPUMs-15) > 1e-9 || st.MaxCPUMs != 20 {
		t.Fatalf("CPU stats = avg %v max %v", st.AvgCPUMs, st.MaxCPUMs)
	}
	if st.MaxDrawCalls != 200 {
		t.Fatalf("MaxDrawCalls = %d", st.MaxDrawCalls)
	}
	if math.Abs(st.TotalGCKB-20) > 1e-9 {
		t.Fatalf("TotalGCKB = %v", st.TotalGCKB)
	}
	if st.LastUsedMB != 512 {
		t.Fatalf("LastUsedMB = %v", st.LastUsedMB)
	}
	if st.P95CPUMs < 19 || st.P95CPUMs > 20 {
		t.Fatalf("P95CPUMs = %v", st.P95CPUMs)
	}
}

func TestStatsEmpty(t *testing.T) {
	s := NewSession(4)
	st := s.Stats()
	if st.FrameCount != 0 || st.AvgFPS != 0 {
		t.Fatalf("empty stats = %+v", st)
	}
}

func TestPercentile(t *testing.T) {
	if p := percentile(nil, 0.95); p != 0 {
		t.Fatalf("percentile(nil) = %v", p)
	}
	if p := percentile([]float64{5}, 0.5); p != 5 {
		t.Fatalf("percentile single = %v", p)
	}
	if p := percentile([]float64{1, 2, 3, 4}, 0.5); math.Abs(p-2.5) > 1e-9 {
		t.Fatalf("percentile median = %v", p)
	}
}

func TestConcurrentAppendAndRead(t *testing.T) {
	s := NewSession(1000)
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			for i := 0; i < 250; i++ {
				s.Append(frameN(base*250 + i))
			}
		}(w)
	}
	for r := 0; r < 2; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_ = s.Latest(50)
				_ = s.Stats()
			}
		}()
	}
	wg.Wait()
	if s.Len() != 1000 || s.Total() != 1000 {
		t.Fatalf("len=%d total=%d, want 1000/1000", s.Len(), s.Total())
	}
}

func TestHierarchyAt(t *testing.T) {
	s := NewSession(4)
	s.AppendDetail(frameN(1), []protocol.SampleNode{{Name: "Main Thread", TotalMs: 16, Calls: 1}})
	s.Append(frameN(2))
	h, ok := s.HierarchyAt(0)
	if !ok || len(h) != 1 || h[0].Name != "Main Thread" {
		t.Fatalf("HierarchyAt(0)=%v ok=%v", h, ok)
	}
	h, ok = s.HierarchyAt(1)
	if !ok || h != nil {
		t.Fatalf("frame without hierarchy: %v ok=%v", h, ok)
	}
	if _, ok := s.HierarchyAt(2); ok {
		t.Fatal("out of range should miss")
	}
	s.Clear()
	if _, ok := s.HierarchyAt(0); ok {
		t.Fatal("cleared session should miss")
	}
}

func TestReplaceClearsHierarchy(t *testing.T) {
	s := NewSession(8)
	s.AppendDetail(frameN(99), []protocol.SampleNode{{Name: "Main Thread"}})
	s.Replace([]protocol.FrameData{frameN(1)})
	h, ok := s.HierarchyAt(0)
	if !ok || h != nil {
		t.Fatalf("Replace should drop hierarchy: %v ok=%v", h, ok)
	}
}

func TestReplaceDetailRestoresHierarchy(t *testing.T) {
	s := NewSession(8)
	s.Append(frameN(99))
	frames := []protocol.FrameData{frameN(1), frameN(2)}
	hiers := [][]protocol.SampleNode{
		{{Name: "Main Thread", TotalMs: 16, Children: []protocol.SampleNode{{Name: "Update", TotalMs: 4}}}},
		nil,
	}
	s.ReplaceDetail(frames, hiers)
	if s.Len() != 2 {
		t.Fatalf("len=%d", s.Len())
	}
	h, ok := s.HierarchyAt(0)
	if !ok || len(h) != 1 || h[0].Name != "Main Thread" || len(h[0].Children) != 1 || h[0].Children[0].Name != "Update" {
		t.Fatalf("HierarchyAt(0)=%v ok=%v", h, ok)
	}
	h, ok = s.HierarchyAt(1)
	if !ok || h != nil {
		t.Fatalf("frame without hierarchy: %v ok=%v", h, ok)
	}
	got := s.Hierarchies()
	if len(got) != 2 || got[0][0].Name != "Main Thread" || got[1] != nil {
		t.Fatalf("Hierarchies=%v", got)
	}
}

func TestReplaceDetailOverflowKeepsLatestHierarchy(t *testing.T) {
	s := NewSession(2)
	frames := []protocol.FrameData{frameN(0), frameN(1), frameN(2)}
	hiers := [][]protocol.SampleNode{
		{{Name: "A"}},
		{{Name: "B"}},
		{{Name: "C"}},
	}
	s.ReplaceDetail(frames, hiers)
	h, _ := s.HierarchyAt(0)
	if len(h) != 1 || h[0].Name != "B" {
		t.Fatalf("oldest kept hierarchy = %v, want B", h)
	}
	h, _ = s.HierarchyAt(1)
	if len(h) != 1 || h[0].Name != "C" {
		t.Fatalf("latest hierarchy = %v, want C", h)
	}
}

func TestHierarchiesEmpty(t *testing.T) {
	s := NewSession(4)
	if s.Hierarchies() != nil {
		t.Fatal("empty session should return nil hierarchies")
	}
	s.Append(frameN(1))
	got := s.Hierarchies()
	if len(got) != 1 || got[0] != nil {
		t.Fatalf("frame without tree: %v", got)
	}
}

func TestRawRoundTrip(t *testing.T) {
	s := NewSession(4)
	a := []byte{1, 2, 3}
	b := []byte{4, 5}
	s.AppendRaw(a)
	s.AppendRaw(b)
	got := s.Raw()
	if len(got) != 2 || !bytesEqual(got[0], a) || !bytesEqual(got[1], b) {
		t.Fatalf("Raw = %v", got)
	}
	a[0] = 9
	if got[0][0] != 1 {
		t.Fatal("AppendRaw should copy")
	}
	s.Clear()
	if s.Raw() == nil {
		t.Fatal("Clear should keep raw stream")
	}
	s.ClearAll()
	if s.Raw() != nil {
		t.Fatal("ClearAll should drop raw")
	}
	s.AppendRaw([]byte{1})
	s.Replace([]protocol.FrameData{frameN(1)})
	if s.Raw() != nil {
		t.Fatal("Replace should drop raw")
	}
	s.SetRaw([][]byte{{9, 8}, {7}})
	got = s.Raw()
	if len(got) != 2 || got[0][0] != 9 || got[1][0] != 7 {
		t.Fatalf("SetRaw = %v", got)
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
