package screenshots

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"sort"
	"strings"
	"time"

	"unityperf/internal/procutil"
)

const MaxShots = 1440

type Shot struct {
	Epoch     int
	TimeNS    int64
	CaptureMs int64
	Streamed  bool `json:",omitempty"`
	JPEG      []byte
}

type Track struct {
	Offsets map[int]int64
	Shots   []Shot
}

func (track *Track) Observe(epoch int, hostNS, playerEndNS int64) {
	if epoch <= 0 {
		return
	}
	if track.Offsets == nil {
		track.Offsets = make(map[int]int64)
	}
	offset := hostNS - playerEndNS
	if previous, ok := track.Offsets[epoch]; !ok || offset < previous {
		track.Offsets[epoch] = offset
	}
}

func (track *Track) Append(shot Shot) {
	if len(track.Shots) == MaxShots {
		copy(track.Shots, track.Shots[1:])
		track.Shots = track.Shots[:MaxShots-1]
	}
	track.Shots = append(track.Shots, shot)
}

func (track Track) Nearest(epoch int, playerNS int64) (Shot, int64, bool) {
	offset, ok := track.Offsets[epoch]
	if !ok {
		return Shot{}, 0, false
	}
	target := playerNS + offset
	start := sort.Search(len(track.Shots), func(index int) bool { return track.Shots[index].Epoch >= epoch })
	end := sort.Search(len(track.Shots), func(index int) bool { return track.Shots[index].Epoch > epoch })
	if start == end {
		return Shot{}, 0, false
	}
	shots := track.Shots[start:end]
	index := sort.Search(len(shots), func(index int) bool { return shots[index].TimeNS >= target })
	if index == len(shots) || (index > 0 && target-shots[index-1].TimeNS <= shots[index].TimeNS-target) {
		index--
	}
	delta := shots[index].TimeNS - target
	tolerance := 750 * time.Millisecond
	if shots[index].Streamed {
		tolerance = 250 * time.Millisecond
	}
	return shots[index], delta / int64(time.Millisecond), delta >= -int64(tolerance) && delta <= int64(tolerance)
}

func Capture(ctx context.Context, serial string) (Shot, error) {
	started := time.Now()
	output, err := adbCapture(ctx, serial)
	var source image.Image
	if err == nil {
		source = rawScreenshot(output)
		if source == nil {
			started = time.Now()
			output, err = adbCapture(ctx, serial, "-p")
			if err == nil {
				source, err = decodePNG(output)
			}
		}
	}
	if err != nil {
		return Shot{}, err
	}
	elapsed := time.Since(started)
	jpegBytes, err := thumbnailJPEG(source)
	return Shot{TimeNS: started.Add(elapsed / 2).UnixNano(), CaptureMs: elapsed.Milliseconds(), JPEG: jpegBytes}, err
}

func thumbnailJPEG(source image.Image) ([]byte, error) {
	bounds := source.Bounds()
	longEdge := max(bounds.Dx(), bounds.Dy())
	width := max(1, bounds.Dx()*min(longEdge, 640)/longEdge)
	height := max(1, bounds.Dy()*min(longEdge, 640)/longEdge)
	thumbnail := image.NewRGBA(image.Rect(0, 0, width, height))
	for row := range height {
		for column := range width {
			thumbnail.Set(column, row, source.At(column*bounds.Dx()/width, row*bounds.Dy()/height))
		}
	}
	var jpegBytes bytes.Buffer
	if err := jpeg.Encode(&jpegBytes, thumbnail, &jpeg.Options{Quality: 75}); err != nil {
		return nil, err
	}
	return jpegBytes.Bytes(), nil
}

func adbCapture(ctx context.Context, serial string, flags ...string) ([]byte, error) {
	cmd := procutil.CommandContext(ctx, "adb", append([]string{"-s", serial, "exec-out", "screencap"}, flags...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("ADB 截图超时或取消: %w", ctx.Err())
	}
	if err != nil && stderr.Len() != 0 {
		return nil, errors.New("ADB 截图失败: " + strings.TrimSpace(stderr.String()))
	}
	if err != nil {
		return nil, fmt.Errorf("ADB 截图失败: %w", err)
	}
	return output, nil
}

func decodePNG(output []byte) (image.Image, error) {
	config, err := png.DecodeConfig(bytes.NewReader(output))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 8192 || config.Height > 8192 {
		return nil, errors.New("ADB 返回无效截图")
	}
	return png.Decode(bytes.NewReader(output))
}

type rawImage struct {
	pixels []byte
	rect   image.Rectangle
}

func (screen rawImage) Bounds() image.Rectangle { return screen.rect }
func (screen rawImage) ColorModel() color.Model { return color.RGBAModel }
func (screen rawImage) At(column, row int) color.Color {
	offset := (row*screen.rect.Dx() + column) * 4
	return color.RGBA{R: screen.pixels[offset], G: screen.pixels[offset+1], B: screen.pixels[offset+2], A: 255}
}

func rawScreenshot(output []byte) image.Image {
	if len(output) < 12 {
		return nil
	}
	width, height, format := binary.LittleEndian.Uint32(output), binary.LittleEndian.Uint32(output[4:]), binary.LittleEndian.Uint32(output[8:])
	if width == 0 || height == 0 || width > 8192 || height > 8192 || (format != 1 && format != 2) {
		return nil
	}
	header := len(output) - int(width*height*4)
	if header != 12 && header != 16 {
		return nil
	}
	return rawImage{pixels: output[header:], rect: image.Rect(0, 0, int(width), int(height))}
}
