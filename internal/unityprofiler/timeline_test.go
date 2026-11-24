package unityprofiler

import (
	"testing"

	"unityperf/internal/profileframe"
)

func TestTimelineAttachesLateWorkerToItsOwnFrame(t *testing.T) {
	const mainID, workerID = 16, 17
	const startNS = 100_000_000
	var frames []profileframe.Frame
	decoder := NewDecoder(func(snapshot FrameSnapshot) {
		frames = append(frames, ToMemoryFrame(snapshot))
	})
	feed := func(body []byte) {
		t.Helper()
		if err := decoder.Feed(body); err != nil {
			t.Fatal(err)
		}
	}
	feed(EncodeStreamHeader(StreamHeader{FormatVersion: Unity2022StreamVersion, MainThreadID: mainID}, 2022, 3, 47, 4, 1))
	var meta RecordWriter
	meta.ThreadDef(workerID, 0, "Job", "Worker 0")
	feed(EncodeChunk(0, metaThreadID, meta.Bytes()))
	var main RecordWriter
	main.FrameBoundary(20, startNS)
	main.Begin(1, startNS)
	main.End(1, startNS+10_000_000)
	main.FrameBoundary(21, startNS+16_000_000)
	main.FrameBoundary(22, startNS+32_000_000)
	feed(EncodeChunk(0, mainID, main.Bytes()))
	if len(frames) != 2 || len(frames[0].TimelineThreads()) != 1 {
		t.Fatalf("main frames=%d", len(frames))
	}
	var worker RecordWriter
	worker.FrameBoundary(20, startNS-1_000_000)
	worker.Begin(2, startNS+2_000_000)
	worker.Begin(3, startNS+4_000_000)
	worker.FrameBoundary(21, startNS+15_000_000)
	worker.End(3, startNS+16_000_000)
	worker.End(2, startNS+17_000_000)
	worker.FrameBoundary(22, startNS+31_000_000)
	feed(EncodeChunk(0, workerID, worker.Bytes()))
	threads := frames[0].TimelineThreads()
	if len(threads) != 2 || threads[1].Name != "Job/Worker 0" || threads[1].OffsetMs != -1 {
		t.Fatalf("threads=%+v", threads)
	}
	first := threads[1].Samples[0]
	if first.StartMs != 3 || first.TotalMs != 13 || len(first.Children) != 1 || first.Children[0].StartMs != 5 {
		t.Fatalf("first worker sample=%+v", first)
	}
	second := frames[1].TimelineThreads()[0].Samples[0]
	if second.StartMs != 0 || second.TotalMs != 2 {
		t.Fatalf("continued worker sample=%+v", second)
	}
	if len(frames[0].MainThread) != 1 || len(frames[1].MainThread) != 0 {
		t.Fatal("worker samples changed Main Thread hierarchy")
	}
}
