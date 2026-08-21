package capture

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"unityperfagent/internal/protocol"
)

func TestLoadOfficialUnity2022Data(t *testing.T) {
	path := filepath.Join("..", "..", "test", "testdata", "unity-profiler.data")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Skip("official Unity .data fixture is not available")
	} else if err != nil {
		t.Fatal(err)
	}
	file, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if file.Handshake.UnityVersion != "2022.3.47f1" {
		t.Fatalf("unity version = %q", file.Handshake.UnityVersion)
	}
	if file.Handshake.ProfilerStreamVersion != unity2022DataVersion {
		t.Fatalf("format version = %#x", file.Handshake.ProfilerStreamVersion)
	}
	if len(file.Frames) != 299 {
		t.Fatalf("frames = %d, want 299", len(file.Frames))
	}
	first, last := file.Frames[0].Summary, file.Frames[len(file.Frames)-1].Summary
	if first.FrameIndex != 1 || last.FrameIndex != 299 {
		t.Fatalf("frame range = %d..%d, want 1..299", first.FrameIndex, last.FrameIndex)
	}
	if first.TimestampMs == 0 || first.FPS < 29 || first.FPS > 30 || first.CPUTimeMs < 33 || first.CPUTimeMs > 34 || first.CPURenderingMs <= 0 || first.CPUScriptsMs <= 0 {
		t.Fatalf("first frame = %+v", first)
	}
	if len(file.Frames[0].MainThread) == 0 || file.Frames[0].MainThread[0].Name != "Main Thread" || !hasSample(file.Frames[0].MainThread, "PlayerLoop") {
		t.Fatalf("hierarchy = %+v", file.Frames[0].MainThread)
	}
	// 第 59 帧（Player 内部编号 452）是 Unity Editor 中 GC Alloc 为 1.2 MB 的帧。
	// Size 元数据应归因至 GC.Alloc 采样，并向 Update/根节点上卷。
	frame59 := file.Frames[58]
	if frame59.Summary.GCAllocKB < 1200 || frame59.Summary.GCAllocKB > 1300 {
		t.Fatalf("frame 59 GC allocation = %.2f KB, want about 1230.6 KB", frame59.Summary.GCAllocKB)
	}
	update, ok := findSample(frame59.MainThread, "Update.ScriptRunBehaviourUpdate")
	if !ok || update.GCAllocKB < 1200 || update.GCAllocKB > 1300 {
		t.Fatalf("frame 59 ScriptRunBehaviourUpdate = %+v, want about 1230.6 KB", update)
	}
	if !IsFiniteFPS(first.FPS) {
		t.Fatalf("fps must be finite: %v", first.FPS)
	}
}

func TestLoadOfficialUnity2022DataReportsProgress(t *testing.T) {
	path := filepath.Join("..", "..", "test", "testdata", "unity-profiler.data")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Skip("official Unity .data fixture is not available")
	} else if err != nil {
		t.Fatal(err)
	}
	var updates [][2]int64
	_, err := LoadFileWithProgress(path, func(done, total int64) {
		updates = append(updates, [2]int64{done, total})
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) < 2 {
		t.Fatalf("progress updates = %v, want intermediate and final updates", updates)
	}
	last := updates[len(updates)-1]
	if last[1] <= 0 || last[0] != last[1] {
		t.Fatalf("final progress = %v, want complete file size", last)
	}
	for _, update := range updates[:len(updates)-1] {
		if update[0] > 0 && update[0] < update[1] {
			return
		}
	}
	t.Fatalf("progress lacks an intermediate update: %v", updates)
}

func hasSample(nodes []protocol.SampleNode, name string) bool {
	_, ok := findSample(nodes, name)
	return ok
}

func findSample(nodes []protocol.SampleNode, name string) (protocol.SampleNode, bool) {
	for _, node := range nodes {
		if node.Name == name {
			return node, true
		}
		if found, ok := findSample(node.Children, name); ok {
			return found, true
		}
	}
	return protocol.SampleNode{}, false
}

func TestReadGCAllocMetadataAssociatesSampleSizes(t *testing.T) {
	body := make([]byte, 40)
	// 元数据记录按 sample index / Size(bytes) 连续保存；从 metadataStart+4
	// 开始，覆盖不同于零偏移的 8-byte 对齐情况。
	binary.LittleEndian.PutUint32(body[12:], 0)
	binary.LittleEndian.PutUint32(body[16:], 64)
	binary.LittleEndian.PutUint32(body[20:], 2)
	binary.LittleEndian.PutUint32(body[24:], 512)
	binary.LittleEndian.PutUint32(body[28:], ^uint32(0))

	samples := []diskSample{{markerID: 7}, {markerID: 8}, {markerID: 7}}
	readGCAllocMetadata(body, 8, samples, map[uint32]markerInfo{
		7: {name: "GC.Alloc"},
		8: {name: "Other"},
	})
	if samples[0].gcAllocBytes != 64 || samples[1].gcAllocBytes != 0 || samples[2].gcAllocBytes != 512 {
		t.Fatalf("GC allocation bytes = %+v", samples)
	}
}

func TestDecodeRejectsUPAC(t *testing.T) {
	_, err := Decode(bytes.NewReader([]byte("UPAC")))
	if !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("err = %v, want unsupported version", err)
	}
}

func TestDecodeRejectsCorruptFrameEndMarker(t *testing.T) {
	var b bytes.Buffer
	header := testDataHeader(4)
	b.Write(header)
	b.Write(make([]byte, 4))
	binary.Write(&b, binary.LittleEndian, fileEndMarker)
	_, err := Decode(&b)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("结束标记")) {
		t.Fatalf("err = %v, want frame end-marker error", err)
	}
}

func TestDecodeRejectsSyntheticFrameWithoutGatheredData(t *testing.T) {
	var b bytes.Buffer
	body := make([]byte, 32)
	binary.LittleEndian.PutUint32(body[0:], 1)
	binary.LittleEndian.PutUint32(body[4:], 1)
	binary.LittleEndian.PutUint32(body[16:], 16_000)
	binary.LittleEndian.PutUint32(body[28:], frameEndMarker)
	b.Write(testDataHeader(uint32(len(body))))
	b.Write(body)
	binary.Write(&b, binary.LittleEndian, fileEndMarker)

	_, err := Decode(&b)
	if !errors.Is(err, ErrIncompleteFrame) {
		t.Fatalf("err = %v, want ErrIncompleteFrame", err)
	}
}

func TestLoadPerfAgentSyntheticFixtureRejected(t *testing.T) {
	path := filepath.Join("..", "..", "test", "testdata", "perfagent-profiler.data")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Skip("synthetic PerfAgent fixture is not available")
	} else if err != nil {
		t.Fatal(err)
	}

	_, err := LoadFile(path)
	if !errors.Is(err, ErrIncompleteFrame) {
		t.Fatalf("err = %v, want ErrIncompleteFrame", err)
	}
}

func testDataHeader(bodySize uint32) []byte {
	header := make([]byte, blockHeaderSize)
	binary.LittleEndian.PutUint32(header[0:], unity2022DataVersion)
	binary.LittleEndian.PutUint32(header[4:], bodySize)
	binary.LittleEndian.PutUint32(header[8:], 2022)
	binary.LittleEndian.PutUint32(header[12:], 3)
	binary.LittleEndian.PutUint32(header[16:], 47)
	binary.LittleEndian.PutUint32(header[20:], 2)
	binary.LittleEndian.PutUint32(header[24:], 1)
	return header
}

func TestLoadFileDoesNotAcceptOldUPAC(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.upac")
	if err := os.WriteFile(path, []byte("UPAC"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadFile(path)
	if !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("err = %v", err)
	}
}
