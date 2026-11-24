package screenshots

import (
	"encoding/binary"
	"image/color"
	"testing"
	"time"
)

func TestRawScreenshotSupportsAndroidHeaderVariants(t *testing.T) {
	for _, headerLength := range []int{12, 16} {
		data := make([]byte, headerLength+8)
		binary.LittleEndian.PutUint32(data, 2)
		binary.LittleEndian.PutUint32(data[4:], 1)
		binary.LittleEndian.PutUint32(data[8:], 1)
		copy(data[headerLength:], []byte{255, 0, 0, 0, 0, 255, 0, 0})
		screen := rawScreenshot(data)
		if screen == nil || screen.At(1, 0) != (color.RGBA{G: 255, A: 255}) {
			t.Fatalf("invalid screenshot for header length %d", headerLength)
		}
		if rawScreenshot(data[:len(data)-1]) != nil {
			t.Fatalf("accepted incomplete screenshot for header length %d", headerLength)
		}
	}
}

func TestTrackUsesPlayerTimeAndKeepsEpochsSeparate(t *testing.T) {
	const millisecond = int64(time.Millisecond)
	var track Track
	track.Observe(1, 2500*millisecond, 1000*millisecond)
	track.Observe(1, 2010*millisecond, 1010*millisecond)
	track.Observe(1, 3500*millisecond, 1020*millisecond)
	track.Append(Shot{Epoch: 1, TimeNS: 1900 * millisecond})
	track.Append(Shot{Epoch: 1, TimeNS: 2100 * millisecond})
	track.Observe(2, 2100*millisecond, 1000*millisecond)
	track.Append(Shot{Epoch: 2, TimeNS: 2150 * millisecond})
	shot, delta, ok := track.Nearest(1, 1070*millisecond)
	if !ok || shot.TimeNS != 2100*millisecond || delta != 30 {
		t.Fatalf("unexpected match %+v, %d, %v", shot, delta, ok)
	}
	shot, delta, ok = track.Nearest(2, 1010*millisecond)
	if !ok || shot.Epoch != 2 || delta != 40 {
		t.Fatalf("matched wrong connection %+v, %d, %v", shot, delta, ok)
	}
	if _, _, ok := track.Nearest(1, 2500*millisecond); ok {
		t.Fatal("matched stale screenshot")
	}
}
