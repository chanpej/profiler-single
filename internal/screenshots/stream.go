package screenshots

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"io"
	"regexp"
	"strconv"
	"time"

	"unityperf/internal/procutil"
)

const streamInterval = time.Second / 8
const streamSize = 496

var displaySize = regexp.MustCompile(`\bcur=(\d+)x(\d+)\b`)

func streamDimensions(output []byte) (int, int) {
	match := displaySize.FindSubmatch(output)
	if len(match) != 3 {
		return 320, 320
	}
	width, _ := strconv.Atoi(string(match[1]))
	height, _ := strconv.Atoi(string(match[2]))
	if width < 1 || height < 1 || width > 16384 || height > 16384 {
		return 320, 320
	}
	larger := max(width, height)
	scale := func(value int) int { return max(8, (value*min(larger, streamSize)/larger+4)/8*8) }
	return scale(width), scale(height)
}

func Stream(ctx context.Context, serial string, consume func(Shot) bool) (received bool) {
	sizeCtx, cancelSize := context.WithTimeout(ctx, 2*time.Second)
	display, _ := procutil.CommandContext(sizeCtx, "adb", "-s", serial, "shell", "dumpsys", "window", "displays").Output()
	cancelSize()
	width, height := streamDimensions(display)
	streamCtx, cancel := context.WithTimeout(ctx, 175*time.Second)
	defer cancel()
	cmd := procutil.CommandContext(streamCtx, "adb", "-s", serial, "exec-out", "screenrecord", "--output-format=frames", "--size", fmt.Sprintf("%dx%d", width, height), "--time-limit", "170", "-")
	output, err := cmd.StdoutPipe()
	if err != nil || cmd.Start() != nil {
		return false
	}
	defer func() {
		cancel()
		_ = cmd.Wait()
	}()
	startup := time.AfterFunc(5*time.Second, cancel)
	defer startup.Stop()
	reader := frameStream{pixels: make([]byte, streamSize*streamSize*3)}
	var previous time.Time
	for streamCtx.Err() == nil {
		width, height, err := reader.read(output)
		if err != nil {
			return received
		}
		if !received {
			startup.Stop()
			received = true
		}
		now := time.Now()
		if now.Sub(previous) < streamInterval {
			continue
		}
		previous = now
		encoded, err := thumbnailJPEG(reader.image(width, height))
		if err == nil && !consume(Shot{TimeNS: now.UnixNano(), JPEG: encoded, Streamed: true}) {
			break
		}
	}
	return received
}

type frameStream struct {
	pixels []byte
}

func (stream *frameStream) read(input io.Reader) (int, int, error) {
	var header [20]byte
	if _, err := io.ReadFull(input, header[:]); err != nil {
		return 0, 0, err
	}
	packet := binary.LittleEndian.Uint32(header[:4])
	width := binary.LittleEndian.Uint32(header[4:8])
	height := binary.LittleEndian.Uint32(header[8:12])
	stride := binary.LittleEndian.Uint32(header[12:16])
	format := binary.LittleEndian.Uint32(header[16:20])
	if width == 0 || height == 0 || width > streamSize || height > streamSize || stride != width*3 || format != 3 || packet != 16+stride*height {
		return 0, 0, errors.New("unsupported Android screenrecord frame")
	}
	_, err := io.ReadFull(input, stream.pixels[:stride*height])
	return int(width), int(height), err
}

func (stream *frameStream) image(width, height int) *image.RGBA {
	frame := image.NewRGBA(image.Rect(0, 0, width, height))
	for source, destination := 0, 0; destination < len(frame.Pix); source, destination = source+3, destination+4 {
		copy(frame.Pix[destination:destination+3], stream.pixels[source:source+3])
		frame.Pix[destination+3] = 255
	}
	return frame
}
