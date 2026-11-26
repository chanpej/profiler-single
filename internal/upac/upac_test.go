package upac

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"unityperf/internal/protocol"
	"unityperf/internal/screenshots"
	"unityperf/internal/unityprofiler"
)

func TestReplayTimelineCapture(t *testing.T) {
	path := os.Getenv("UNITYPROFILER_TIMELINE_CAPTURE")
	if path == "" {
		t.Skip("set UNITYPROFILER_TIMELINE_CAPTURE to a real .upac capture")
	}
	capture, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := unityprofiler.ReplayFrames(capture.Raw)
	if err != nil {
		t.Fatal(err)
	}
	threadNames := make(map[string]int)
	multithreaded := 0
	maxMainSamples := 0
	expectedName := os.Getenv("UNITYPROFILER_EXPECT_SAMPLE")
	matchingSamples := 0
	var distinctNestedDurations bool
	var countSamples func([]protocol.SampleNode) int
	countSamples = func(samples []protocol.SampleNode) int {
		count := len(samples)
		for _, sample := range samples {
			if expectedName != "" && sample.Name == expectedName {
				matchingSamples++
			}
			for _, child := range sample.Children {
				if child.TotalMs > 0 && child.TotalMs < sample.TotalMs {
					distinctNestedDurations = true
				}
			}
			count += countSamples(sample.Children)
		}
		return count
	}
	for _, frame := range frames {
		threads := frame.TimelineThreads()
		if len(threads) > 1 {
			multithreaded++
		}
		for _, thread := range threads {
			if thread.ID == "main" {
				maxMainSamples = max(maxMainSamples, countSamples(thread.Samples))
			}
			if len(thread.Samples) > 0 {
				threadNames[thread.Name]++
			}
		}
	}
	t.Logf("version=%s frames=%d multithreaded=%d maxMainSamples=%d threads=%v", capture.Handshake.UnityVersion, len(frames), multithreaded, maxMainSamples, threadNames)
	if expectedName != "" {
		t.Logf("resolved %q on %d main-thread samples", expectedName, matchingSamples)
		if matchingSamples == 0 {
			t.Fatal("expected sample name missing from replay")
		}
	}
	if threadNames["Main Thread"] == 0 {
		t.Fatal("no frame has timestamped main-thread samples")
	}
	if !distinctNestedDurations {
		t.Fatal("no nested main-thread sample has a distinct positive duration")
	}
}

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

func TestScreenshotTrackRoundTripAcrossConnections(t *testing.T) {
	raw := unityprofiler.SampleProfilerCapture()
	want := File{
		Raw: append(raw, raw...),
		Screen: screenshots.Track{
			Offsets: map[int]int64{1: 900_000_000, 2: 1_900_000_000},
			Shots:   []screenshots.Shot{{Epoch: 2, TimeNS: 1_950_000_000, JPEG: []byte{0xff, 0xd8}}},
		},
	}
	var encoded bytes.Buffer
	if err := Encode(&encoded, want); err != nil {
		t.Fatal(err)
	}
	loaded, err := Decode(&encoded)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := unityprofiler.ReplayFrames(loaded.Raw)
	if err != nil || len(frames) != 2 {
		t.Fatalf("replay: %d frames, %v", len(frames), err)
	}
	if _, _, ok := loaded.Screen.Nearest(frames[0].ClockEpoch, frames[0].PlayerEndNS); ok {
		t.Fatal("matched a screenshot from another connection")
	}
	shot, delta, ok := loaded.Screen.Nearest(frames[1].ClockEpoch, frames[1].PlayerEndNS)
	if !ok || delta != 40 || !bytes.Equal(shot.JPEG, want.Screen.Shots[0].JPEG) {
		t.Fatalf("incorrect replay match: %+v, %d, %v", shot, delta, ok)
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

func TestRejectIncompleteDictionaryWithoutReplacingPreviousExport(t *testing.T) {
	raw := unityprofiler.SampleProfilerCapture()
	late := append([]byte(nil), raw[1]...)
	late[4] = 64
	for _, bad := range [][][]byte{raw[1:], {raw[0], raw[2]}, {raw[0], late, raw[2]}, {raw[0], raw[1], raw[2], raw[0], late}} {
		path := filepath.Join(t.TempDir(), "session.upac")
		if err := os.WriteFile(path, []byte("prior"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := SaveFile(path, File{Raw: bad}); err == nil {
			t.Fatal("saved incomplete stream")
		}
		content, err := os.ReadFile(path)
		if err != nil || string(content) != "prior" {
			t.Fatal("overwrote previous archive", err)
		}
	}
}
