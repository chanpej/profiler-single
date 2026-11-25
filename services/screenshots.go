package services

import (
	"context"
	"encoding/base64"
	"errors"
	"maps"
	"slices"
	"strconv"
	"time"

	"unityperf/internal/adbforward"
	"unityperf/internal/screenshots"
)

type screenState struct {
	track     screenshots.Track
	epoch     int
	cancel    context.CancelFunc
	runID     uint64
	revision  uint64
	lastError string
}

type ScreenThumbnail struct {
	ID        string `json:"id"`
	Frame     int    `json:"frame"`
	Image     string `json:"image"`
	DeltaMs   int64  `json:"deltaMs"`
	CaptureMs int64  `json:"captureMs"`
	Streamed  bool   `json:"streamed"`
}

type ScreenFilmstrip struct {
	Enabled    bool              `json:"enabled"`
	Available  bool              `json:"available"`
	Error      string            `json:"error"`
	Thumbnails []ScreenThumbnail `json:"thumbnails"`
	Selected   *ScreenThumbnail  `json:"selected"`
}

func (p *PerfService) SetScreenCapture(enabled bool) error {
	return p.setScreenCapture(enabled, "")
}

func (p *PerfService) StartWirelessScreenCapture(address string) error {
	endpoint, err := wirelessEndpoint(address)
	if err != nil {
		return err
	}
	return p.setScreenCapture(true, endpoint)
}

func (p *PerfService) setScreenCapture(enabled bool, endpoint string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !enabled {
		p.stopScreenLocked()
		return nil
	}
	if p.screen.cancel != nil {
		return nil
	}
	if !p.connectedLocked() || p.sourceFile != "" {
		return errors.New("请先连接 Player")
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.screen.cancel = cancel
	p.screen.lastError = ""
	p.screen.runID++
	go p.captureScreen(ctx, p.screen.runID, p.adbRun, p.usbSerial, endpoint)
	return nil
}

func (p *PerfService) captureScreen(ctx context.Context, runID uint64, run adbforward.Runner, serial, endpoint string) {
	if endpoint != "" {
		if err := connectScreenDevice(ctx, run, endpoint); err != nil {
			p.screenCaptureError(runID, err)
			return
		}
		serial = endpoint
	} else if serial == "" {
		devices, err := adbforward.ListDevices(run)
		if err != nil {
			p.screenCaptureError(runID, err)
			return
		}
		if len(devices) != 1 {
			p.screenCaptureError(runID, errors.New("自动选择截图设备需要一台在线 ADB 手机；也可在无线截图中指定地址"))
			return
		}
		serial = devices[0].Serial
	}
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()
	tryStream := true
	for {
		p.mu.Lock()
		epoch, revision := p.screen.epoch, p.screen.revision
		active := p.recording && p.connectedLocked() && epoch > 0
		p.mu.Unlock()
		if active && ctx.Err() == nil {
			store := func(shot screenshots.Shot) bool {
				p.mu.Lock()
				defer p.mu.Unlock()
				if ctx.Err() != nil || p.screen.epoch != epoch || p.screen.revision != revision || !p.recording {
					return false
				}
				shot.Epoch = epoch
				p.screen.track.Append(shot)
				return true
			}
			if tryStream {
				tryStream = screenshots.Stream(ctx, serial, store)
				if tryStream || ctx.Err() != nil {
					continue
				}
			}
			captureCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			shot, err := screenshots.Capture(captureCtx, serial)
			cancel()
			if err != nil {
				p.screenCaptureError(runID, err)
				return
			}
			store(shot)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (p *PerfService) screenCaptureError(runID uint64, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.screen.runID == runID && p.screen.cancel != nil {
		p.screen.lastError = err.Error()
		p.stopScreenLocked()
	}
}

func (p *PerfService) stopScreenLocked() {
	if p.screen.cancel != nil {
		p.screen.cancel()
		p.screen.cancel = nil
	}
}

func (p *PerfService) resetScreenLocked(stop bool) {
	if stop {
		p.stopScreenLocked()
	}
	p.screen.track = screenshots.Track{}
	p.screen.lastError = ""
	p.screen.revision++
	if stop {
		p.screen.epoch = 0
	}
}

func (p *PerfService) screenTrackLocked() screenshots.Track {
	return screenshots.Track{Offsets: maps.Clone(p.screen.track.Offsets), Shots: slices.Clone(p.screen.track.Shots)}
}

func (p *PerfService) ScreenFilmstrip(offset, count, selected int) ScreenFilmstrip {
	p.mu.Lock()
	track := p.screenTrackLocked()
	result := ScreenFilmstrip{Enabled: p.screen.cancel != nil, Available: len(track.Shots) > 0, Error: p.screen.lastError}
	p.mu.Unlock()
	if !result.Available {
		return result
	}
	const maxThumbs = 10
	frames := p.session.RangeFrames(offset, min(count, 2000))
	type candidate struct {
		thumbnail ScreenThumbnail
		shot      screenshots.Shot
	}
	var candidates []candidate
	for position, frame := range frames {
		shot, delta, ok := track.Nearest(frame.ClockEpoch, frame.PlayerEndNS)
		if !ok {
			continue
		}
		item := candidate{ScreenThumbnail{ID: strconv.FormatInt(shot.TimeNS, 10), Frame: offset + position, DeltaMs: delta, CaptureMs: shot.CaptureMs, Streamed: shot.Streamed}, shot}
		last := len(candidates) - 1
		if last < 0 || candidates[last].shot.TimeNS != shot.TimeNS {
			candidates = append(candidates, item)
		} else if absolute(delta) < absolute(candidates[last].thumbnail.DeltaMs) {
			candidates[last] = item
		}
	}
	seen := min(maxThumbs, len(candidates))
	for index := range seen {
		item := candidates[index*(len(candidates)-1)/max(1, seen-1)]
		item.thumbnail.Image = base64.StdEncoding.EncodeToString(item.shot.JPEG)
		result.Thumbnails = append(result.Thumbnails, item.thumbnail)
	}
	if frame, ok := p.session.FrameAt(selected); ok {
		if shot, delta, found := track.Nearest(frame.ClockEpoch, frame.PlayerEndNS); found {
			result.Selected = &ScreenThumbnail{ID: strconv.FormatInt(shot.TimeNS, 10), Frame: selected, Image: base64.StdEncoding.EncodeToString(shot.JPEG), DeltaMs: delta, CaptureMs: shot.CaptureMs, Streamed: shot.Streamed}
		}
	}
	return result
}

func absolute(delta int64) int64 {
	return max(delta, -delta)
}
