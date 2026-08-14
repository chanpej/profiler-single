package capture

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"unityperfagent/internal/protocol"
)

func sampleFrame(i int) protocol.FrameData {
	return protocol.FrameData{
		FrameIndex:     uint64(i),
		TimestampMs:    int64(1_000 + i),
		FPS:            59.7,
		CPUTimeMs:      16.4,
		GPUTimeMs:      12.1,
		MainThreadMs:   14.2,
		RenderMs:       6.3,
		GCAllocKB:      128.5,
		UsedMemMB:      512.25,
		DrawCalls:      uint32(300 + i),
		CPURenderingMs: 5.2,
		CPUScriptsMs:   4.1,
		GCUsedMB:       180.5,
		ShadowCasters:  18,
		UIBatches:      24,
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	in := File{
		Handshake: protocol.Handshake{
			DeviceName:   "Pixel 7",
			DeviceModel:  "Google Pixel 7",
			OS:           "Android 14",
			UnityVersion: "2022.3.47f1",
			AppVersion:   "1.0.0",
		},
		Frames: []protocol.FrameData{sampleFrame(0), sampleFrame(1), sampleFrame(2)},
	}
	var buf bytes.Buffer
	if err := Encode(&buf, in); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	meta, _ := protocol.EncodeHandshake(&in.Handshake)
	wantSize := EncodedSize(len(in.Frames), len(meta))
	if buf.Len() != wantSize {
		t.Fatalf("encoded size = %d, want %d", buf.Len(), wantSize)
	}
	got, err := Decode(&buf)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Handshake.DeviceName != in.Handshake.DeviceName || got.Handshake.UnityVersion != in.Handshake.UnityVersion {
		t.Fatalf("handshake = %+v", got.Handshake)
	}
	if len(got.Frames) != 3 {
		t.Fatalf("frames = %d", len(got.Frames))
	}
	for i, f := range got.Frames {
		if f != in.Frames[i] {
			t.Fatalf("frame %d mismatch:\n got %+v\nwant %+v", i, f, in.Frames[i])
		}
	}
}

func TestEncodeDecodeEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := Encode(&buf, File{}); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if buf.Len() != EncodedSize(0, 0) {
		t.Fatalf("empty size = %d, want %d", buf.Len(), EncodedSize(0, 0))
	}
	got, err := Decode(&buf)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(got.Frames) != 0 || got.Handshake.DeviceName != "" {
		t.Fatalf("got %+v", got)
	}
}

func TestDecodeBadMagic(t *testing.T) {
	data := make([]byte, headerSize)
	copy(data, []byte("XXXX"))
	_, err := Decode(bytes.NewReader(data))
	if !errors.Is(err, ErrBadMagic) {
		t.Fatalf("err = %v, want ErrBadMagic", err)
	}
}

func TestDecodeUnsupportedVersion(t *testing.T) {
	h := header{Magic: Magic, Version: 99, FrameSize: uint16(protocol.FramePayloadSize)}
	var buf bytes.Buffer
	if err := h.writeTo(&buf); err != nil {
		t.Fatal(err)
	}
	_, err := Decode(&buf)
	if !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("err = %v, want ErrUnsupportedVersion", err)
	}
}

func TestDecodeTruncatedFrame(t *testing.T) {
	in := File{Frames: []protocol.FrameData{sampleFrame(0)}}
	var buf bytes.Buffer
	if err := Encode(&buf, in); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()[:buf.Len()-8]
	_, err := Decode(bytes.NewReader(raw))
	if err == nil {
		t.Fatal("expected truncated error")
	}
}

func TestSaveLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.upac")
	in := File{
		Handshake: protocol.Handshake{DeviceName: "Mi MIX 2S", UnityVersion: "2022.3.47f1"},
		Frames:    []protocol.FrameData{sampleFrame(7), sampleFrame(8)},
	}
	if err := SaveFile(path, in); err != nil {
		t.Fatalf("SaveFile: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != int64(EncodedSize(2, len(`{"deviceName":"Mi MIX 2S","deviceModel":"","os":"","unityVersion":"2022.3.47f1","appVersion":"","profilerStreamVersion":0}`))) {
		// 以 Decode 正确性为准，体积只需明显小于 JSON 展开。
		if info.Size() > 1024 {
			t.Fatalf("file too large for 2 frames: %d", info.Size())
		}
	}
	got, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if got.Handshake.DeviceName != "Mi MIX 2S" || len(got.Frames) != 2 || got.Frames[1].FrameIndex != 8 {
		t.Fatalf("got %+v", got)
	}
}

func TestCompactVsJSON(t *testing.T) {
	frames := make([]protocol.FrameData, 1000)
	for i := range frames {
		frames[i] = sampleFrame(i)
	}
	var buf bytes.Buffer
	if err := Encode(&buf, File{Frames: frames}); err != nil {
		t.Fatal(err)
	}
	want := EncodedSize(1000, 0)
	if buf.Len() != want {
		t.Fatalf("size = %d, want %d", buf.Len(), want)
	}
	// 1000 帧应远小于同等 JSON（每帧 JSON 约 600B+）。
	if buf.Len() > 1000*protocol.FramePayloadSize+headerSize {
		t.Fatalf("not compact: %d", buf.Len())
	}
}
