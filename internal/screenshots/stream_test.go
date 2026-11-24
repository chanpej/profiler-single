package screenshots

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image/color"
	"image/jpeg"
	"io"
	"os"
	"testing"
	"time"
)

func TestFrameStreamReadsConsecutiveRGBFrames(t *testing.T) {
	packet := make([]byte, 20, 26)
	for index, value := range []uint32{22, 2, 1, 6, 3} {
		binary.LittleEndian.PutUint32(packet[index*4:], value)
	}
	packet = append(packet, 255, 0, 0, 0, 255, 0)
	stream := frameStream{pixels: make([]byte, streamSize*streamSize*3)}
	input := bytes.NewReader(bytes.Repeat(packet, 3))
	for range 3 {
		width, height, err := stream.read(input)
		if err != nil || width != 2 || height != 1 {
			t.Fatalf("frame %dx%d: %v", width, height, err)
		}
		if pixel := stream.image(width, height).RGBAAt(1, 0); pixel != (color.RGBA{G: 255, A: 255}) {
			t.Fatalf("pixel = %v", pixel)
		}
	}
	if _, _, err := stream.read(bytes.NewReader(packet[:len(packet)-1])); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("truncated packet error = %v", err)
	}
	binary.LittleEndian.PutUint32(packet[4:], 50_000)
	if _, _, err := stream.read(bytes.NewReader(packet)); err == nil {
		t.Fatal("accepted invalid dimensions")
	}
}

func TestStreamDimensionsFollowDisplayAspectRatio(t *testing.T) {
	for _, test := range []struct {
		display       string
		width, height int
	}{{"cur=1440x2960", 240, 496}, {"cur=2960x1440", 496, 240}, {"", 320, 320}} {
		width, height := streamDimensions([]byte(test.display))
		if width != test.width || height != test.height {
			t.Fatalf("%q gave %dx%d, want %dx%d", test.display, width, height, test.width, test.height)
		}
	}
}

func TestStreamFromAndroid(t *testing.T) {
	serial := os.Getenv("UNITYPROFILER_SCREEN_TEST_SERIAL")
	if serial == "" {
		t.Skip("set UNITYPROFILER_SCREEN_TEST_SERIAL for a connected Android device")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 7*time.Second)
	defer cancel()
	var shots []Shot
	if received := Stream(ctx, serial, func(shot Shot) bool {
		shots = append(shots, shot)
		return len(shots) < 24
	}); !received || len(shots) < 10 {
		t.Fatalf("received %d usable screenshots", len(shots))
	}
	for index, shot := range shots {
		if _, err := jpeg.DecodeConfig(bytes.NewReader(shot.JPEG)); err != nil || !shot.Streamed {
			t.Fatalf("shot %d is invalid: %v", index, err)
		}
		if index > 0 && shot.TimeNS-shots[index-1].TimeNS < int64(streamInterval) {
			t.Fatal("stream exceeded the retained frame rate")
		}
	}
	duration := time.Duration(shots[len(shots)-1].TimeNS - shots[0].TimeNS)
	var largestGap time.Duration
	for index := 1; index < len(shots); index++ {
		largestGap = max(largestGap, time.Duration(shots[index].TimeNS-shots[index-1].TimeNS))
	}
	t.Logf("%d screenshots in %v, %.1f frames/s, largest gap %v", len(shots), duration, float64(len(shots)-1)/duration.Seconds(), largestGap)
}
