package services

import (
	"os"
	"testing"

	"unityperf/internal/profileframe"
	"unityperf/internal/protocol"
)

func TestFrameHeavyProfileMergesPathsAndSummarizesFunctions(t *testing.T) {
	svc := newTestService()
	appendFrame := func(id uint64, samples ...protocol.SampleNode) {
		frame := profileframe.New(protocol.FrameData{FrameIndex: id}, samples)
		frame.Timeline = &profileframe.Timeline{}
		frame.Timeline.SetThread(9, "Job/Worker", 0, []protocol.SampleNode{{Name: "Update", TotalMs: 100, Calls: 1}})
		svc.session.AppendFrame(frame)
	}
	appendFrame(40, protocol.SampleNode{Name: "Outside", TotalMs: 300, Calls: 7})
	appendFrame(41, protocol.SampleNode{Name: "Parent", TotalMs: 16, Calls: 1, Children: []protocol.SampleNode{
		{Name: "Small", TotalMs: 1, Calls: 1}, {Name: "Update", TotalMs: 6, SelfMs: 3, Calls: 2},
	}}, protocol.SampleNode{Name: "OtherParent", TotalMs: 3, Calls: 1, Children: []protocol.SampleNode{{Name: "Update", TotalMs: 2, SelfMs: 2, Calls: 1}}})
	appendFrame(42, protocol.SampleNode{Name: "Parent", TotalMs: 20, Calls: 1, Children: []protocol.SampleNode{{Name: "Update", TotalMs: 9, SelfMs: 5, Calls: 1}}})
	got, err := svc.FrameHeavyProfile(1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got.FirstFrame != 41 || got.LastFrame != 42 || got.FrameCount != 2 || len(got.Threads) != 2 {
		t.Fatalf("profile = %+v", got)
	}
	main := got.Threads[0]
	if main.ID != "main" || len(main.Roots) != 2 || len(main.Roots[0].Children) != 2 {
		t.Fatalf("main thread = %+v", main)
	}
	hottest := main.Roots[0].Children[0]
	if hottest.Name != "Update" || hottest.Calls != 3 || hottest.TotalMs != 15 || hottest.SelfMs != 8 {
		t.Fatalf("hottest branch = %+v", hottest)
	}
	function := main.Functions[1]
	if function.Name != "Update" || function.Calls != 4 || function.TotalMs != 17 || function.SelfMs != 10 {
		t.Fatalf("merged function = %+v", function)
	}
	if worker := got.Threads[1]; len(worker.Functions) != 1 || worker.Functions[0].TotalMs != 200 {
		t.Fatalf("other thread = %+v", worker)
	}
	if _, err := svc.FrameHeavyProfile(0, 3); err == nil {
		t.Fatal("out-of-range end was accepted")
	}
}

func TestFrameHeavyProfileFromCapture(t *testing.T) {
	path := os.Getenv("UNITYPROFILER_TIMELINE_CAPTURE")
	if path == "" {
		t.Skip("set UNITYPROFILER_TIMELINE_CAPTURE to a real .upac capture")
	}
	svc := newTestService()
	count, err := svc.ImportCapture(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.FrameHeavyProfile(0, count-1)
	if err != nil {
		t.Fatal(err)
	}
	if got.FrameCount != count || len(got.Threads) == 0 || len(got.Threads[0].Functions) == 0 {
		t.Fatalf("profile = %+v", got)
	}
	t.Logf("frames=%d main-thread functions=%d", count, len(got.Threads[0].Functions))
}
