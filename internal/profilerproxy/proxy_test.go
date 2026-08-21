package profilerproxy

import (
	"bytes"
	"errors"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"unityperfagent/internal/capture"
	"unityperfagent/internal/unityprofiler"
)

func TestRelayForwardsBothDirectionsAndCapturesPD3U(t *testing.T) {
	editor, proxyEditor := net.Pipe()
	proxyPlayer, player := net.Pipe()
	defer editor.Close()
	defer player.Close()

	relayResult := make(chan struct {
		capture Capture
		err     error
	}, 1)
	go func() {
		capture, err := Relay(proxyEditor, proxyPlayer)
		relayResult <- struct {
			capture Capture
			err     error
		}{capture, err}
	}()

	editorResult := make(chan error, 1)
	go func() {
		if err := unityprofiler.WriteMessage(editor, unityprofiler.MsgIDProfilerAreas, []byte{0xfd, 0x7f, 0, 0}); err != nil {
			editorResult <- err
			return
		}
		for _, want := range []unityprofiler.MessageID{unityprofiler.MsgIDPlayerInfo, unityprofiler.MsgIDProfilerData} {
			got, err := unityprofiler.ReadMessage(editor)
			if err != nil {
				editorResult <- err
				return
			}
			if got.ID != want {
				editorResult <- errors.New("unexpected Player-to-Editor message")
				return
			}
		}
		editorResult <- editor.Close()
	}()

	playerResult := make(chan error, 1)
	profilerBody := []byte{0x50, 0x44, 0x33, 0x55, 1, 2, 3, 4}
	go func() {
		got, err := unityprofiler.ReadMessage(player)
		if err != nil {
			playerResult <- err
			return
		}
		if got.ID != unityprofiler.MsgIDProfilerAreas || !bytes.Equal(got.Body, []byte{0xfd, 0x7f, 0, 0}) {
			playerResult <- errors.New("Editor-to-Player message was not forwarded")
			return
		}
		if err := unityprofiler.WriteMessage(player, unityprofiler.MsgIDPlayerInfo, []byte("player-info")); err != nil {
			playerResult <- err
			return
		}
		if err := unityprofiler.WriteMessage(player, unityprofiler.MsgIDProfilerData, profilerBody); err != nil {
			playerResult <- err
			return
		}
		playerResult <- player.Close()
	}()

	assertNoRelayError(t, <-editorResult)
	assertNoRelayError(t, <-playerResult)
	select {
	case result := <-relayResult:
		if result.err != nil {
			t.Fatal(result.err)
		}
		if len(result.capture.Bodies) != 1 || !bytes.Equal(result.capture.Bodies[0], profilerBody) {
			t.Fatalf("capture = %#v, want one PD3U body %x", result.capture.Bodies, profilerBody)
		}
	case <-time.After(time.Second):
		t.Fatal("Relay did not finish")
	}
}

func TestProxyFixtureReplaysAgainstEditorData(t *testing.T) {
	pd3uPath := filepath.Join("..", "..", "test", "same-session.uppc")
	dataPath := filepath.Join("..", "..", "test", "testdata", "unity-profiler-proxy.data")
	if _, err := os.Stat(pd3uPath); errors.Is(err, os.ErrNotExist) {
		t.Skip("same-session PD3U fixture is not available")
	} else if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dataPath); errors.Is(err, os.ErrNotExist) {
		t.Skip("same-session Unity .data fixture is not available")
	} else if err != nil {
		t.Fatal(err)
	}

	captured, err := ReadFile(pd3uPath)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := unityprofiler.ReplayFrames(captured.Bodies)
	if err != nil {
		t.Fatal(err)
	}
	editorFile, err := capture.LoadFile(dataPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) == 0 || len(editorFile.Frames) == 0 {
		t.Fatalf("PD3U frames=%d, Editor frames=%d", len(frames), len(editorFile.Frames))
	}
	t.Logf("PD3U frames=%d (%d..%d), Editor frames=%d (%d..%d)",
		len(frames), frames[0].Summary.FrameIndex, frames[len(frames)-1].Summary.FrameIndex,
		len(editorFile.Frames), editorFile.Frames[0].Summary.FrameIndex, editorFile.Frames[len(editorFile.Frames)-1].Summary.FrameIndex)
	const warmupFrames = 21
	if len(frames) < len(editorFile.Frames)+warmupFrames {
		t.Fatalf("PD3U frames=%d cannot cover %d Editor frames after %d warmup frames", len(frames), len(editorFile.Frames), warmupFrames)
	}
	var maxCPUDelta float64
	for i, editorFrame := range editorFile.Frames {
		pd3uFrame := frames[i+warmupFrames]
		delta := math.Abs(float64(pd3uFrame.Summary.CPUTimeMs - editorFrame.Summary.CPUTimeMs))
		if delta > maxCPUDelta {
			maxCPUDelta = delta
		}
	}
	if maxCPUDelta > 0.01 {
		t.Fatalf("PD3U and Editor CPU frame alignment drifted by %.6fms", maxCPUDelta)
	}
	t.Logf("PD3U frame 22=%+v", frames[21].Summary)
	t.Logf("first Editor=%+v", editorFile.Frames[0].Summary)
	t.Logf("aligned %d frames after %d warmup frames; max CPU delta %.6fms", len(editorFile.Frames), warmupFrames, maxCPUDelta)
}

func TestCaptureFileRoundTrip(t *testing.T) {
	want := Capture{Bodies: [][]byte{{0x50, 0x44, 0x33, 0x55}, {1, 2, 3}, nil}}
	path := filepath.Join(t.TempDir(), "session.uppc")
	if err := WriteFile(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Bodies) != len(want.Bodies) {
		t.Fatalf("body count = %d, want %d", len(got.Bodies), len(want.Bodies))
	}
	for i := range want.Bodies {
		if !bytes.Equal(got.Bodies[i], want.Bodies[i]) {
			t.Fatalf("body %d = %x, want %x", i, got.Bodies[i], want.Bodies[i])
		}
	}
}

func TestReadCaptureRejectsInvalidHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.uppc")
	if err := os.WriteFile(path, []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(path); !errors.Is(err, ErrCaptureFormat) && !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("err = %v, want capture format error", err)
	}
}

func assertNoRelayError(t *testing.T, err error) {
	t.Helper()
	if err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
}
