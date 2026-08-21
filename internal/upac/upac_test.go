package upac

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"unityperfagent/internal/protocol"
	"unityperfagent/internal/unityprofiler"
)

func TestEncodeDecodeRawRoundTrip(t *testing.T) {
	want := File{
		Handshake: protocol.Handshake{DeviceName: "Pixel 7", UnityVersion: "2022.3.47f1", MainThreadID: 42},
		Raw:       unityprofiler.SampleProfilerCapture(),
	}
	var encoded bytes.Buffer
	if err := Encode(&encoded, want); err != nil {
		t.Fatal(err)
	}
	got, err := Decode(&encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.Handshake != want.Handshake || len(got.Raw) != len(want.Raw) {
		t.Fatalf("decoded archive = %+v, raw chunks=%d", got.Handshake, len(got.Raw))
	}
	for i := range want.Raw {
		if !bytes.Equal(got.Raw[i], want.Raw[i]) {
			t.Fatalf("chunk %d mismatch", i)
		}
	}
	frames, err := unityprofiler.ReplayFrames(got.Raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 || frames[0].Summary.FrameIndex != 1 {
		t.Fatalf("replayed frames = %+v", frames)
	}
}

func TestSaveLoadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.upac")
	want := File{Handshake: protocol.Handshake{UnityVersion: "2022.3.47f1"}, Raw: unityprofiler.SampleProfilerCapture()}
	if err := SaveFile(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Handshake.UnityVersion != want.Handshake.UnityVersion || len(got.Raw) != len(want.Raw) {
		t.Fatalf("loaded archive = %+v chunks=%d", got.Handshake, len(got.Raw))
	}
}

func TestRejectsEmptyAndForeignArchives(t *testing.T) {
	if err := Encode(&bytes.Buffer{}, File{}); !errors.Is(err, ErrNoRawData) {
		t.Fatalf("empty archive error = %v", err)
	}
	if _, err := Decode(bytes.NewReader([]byte("UPPD"))); err == nil {
		t.Fatal("expected foreign format to fail")
	}
}
