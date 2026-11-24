package profileframe

import (
	"testing"

	"unityperf/internal/protocol"
)

func TestNewCombinesSummaryAndMainThread(t *testing.T) {
	summary := protocol.FrameData{FrameIndex: 1, CPUTimeMs: 16}
	main := []protocol.SampleNode{{Name: "Main Thread", TotalMs: 16}}
	frame := New(summary, main)
	if frame.Summary.FrameIndex != 1 || len(frame.MainThread) != 1 || frame.MainThread[0].Name != "Main Thread" {
		t.Fatalf("frame = %+v", frame)
	}
}
