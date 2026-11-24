package unityprofiler

import (
	"testing"

	"unityperf/internal/protocol"
)

func TestDynamicSampleNameUsesDeclaredNameField(t *testing.T) {
	marker := markerInfo{Name: "Profiler.Default"}
	marker.recordField(0, "Object")
	marker.recordField(1, "Name")
	object := value{Type: DataTypeString, Raw: []byte("object label")}
	for _, test := range []struct {
		field value
		want  string
	}{
		{value{DataTypeString16, []byte{'A', 0, 0x2d, 0x4e, 0, 0}}, "A中"},
		{value{DataTypeString, []byte("Player.Update\x00")}, "Player.Update"},
		{value{DataTypeString16, []byte{1}}, "Profiler.Default"},
		{value{DataTypeString, nil}, "Profiler.Default"},
	} {
		if got := marker.sampleName([]value{object, test.field}); got != test.want {
			t.Errorf("name = %q, want %q", got, test.want)
		}
	}
	ordinary := markerInfo{Name: "Render"}
	ordinary.recordField(0, "Name")
	if got := ordinary.sampleName([]value{object}); got != "Render" {
		t.Fatalf("ordinary marker renamed to %q", got)
	}
}

func TestTuanjieTimelineMatchesPaddedEndMarkers(t *testing.T) {
	const mainID = 42
	var frames []FrameSnapshot
	decoder := NewDecoder(func(frame FrameSnapshot) { frames = append(frames, frame) })
	feed := func(data []byte) {
		t.Helper()
		if err := decoder.Feed(data); err != nil {
			t.Fatal(err)
		}
	}
	feed(EncodeStreamHeader(StreamHeader{FormatVersion: protocol.Tuanjie2022StreamVersion, MainThreadID: mainID}, 2022, 3, 47, 4, 1))
	var records RecordWriter
	boundary := func(index uint32, at int64) {
		records.u32(recFrame)
		records.u32(index)
		records.i64(at * 1_000_000)
	}
	begin := func(marker uint32, at int64) {
		records.u32(recBeginPlain)
		records.u32(0)
		records.u32(marker)
		records.i64(at * 1_000_000)
	}
	end := func(marker uint32, at int64, padded bool) {
		records.u32(recEnd)
		if padded {
			records.u32(0)
		}
		records.u32(marker)
		records.i64(at * 1_000_000)
	}
	boundary(10, 100)
	begin(1, 100)
	begin(2, 102)
	begin(3, 104)
	begin(4, 106)
	end(99, 107, true)
	end(4, 108, true)
	end(2, 112, true)
	end(1, 115, false)
	boundary(11, 120)
	begin(5, 121)
	end(5, 127, true)
	boundary(12, 136)
	feed(EncodeChunk(0, mainID, records.Bytes()))
	if len(frames) != 2 {
		t.Fatalf("emitted %d frames, want 2", len(frames))
	}
	assertSample := func(nodes []protocol.SampleNode, start, total, self float32) protocol.SampleNode {
		t.Helper()
		if len(nodes) != 1 || nodes[0].StartMs != start || nodes[0].TotalMs != total || nodes[0].SelfMs != self {
			t.Fatalf("samples = %+v, want one sample at %gms with duration %gms and self time %gms", nodes, start, total, self)
		}
		return nodes[0]
	}
	root := assertSample(ToMemoryFrame(frames[0]).MainThread, 0, 15, 5)
	parent := assertSample(root.Children, 2, 10, 8)
	leaf := assertSample(parent.Children, 6, 2, 2)
	following := assertSample(ToMemoryFrame(frames[1]).MainThread, 1, 6, 6)
	if len(leaf.Children) != 0 || len(following.Children) != 0 {
		t.Fatal("unexpected incomplete scopes in the timeline")
	}
}
