package services

import (
	"errors"
	"fmt"

	"os"

	"time"
	"unityperf/internal/upac"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type MemoryRecord struct {
	ID      string       `json:"id"`
	Session string       `json:"session"`
	Time    int64        `json:"time"`
	Path    string       `json:"path"`
	Sample  MemorySample `json:"sample"`
}
type MemoryLibrary struct {
	Session string         `json:"session"`
	Records []MemoryRecord `json:"records"`
}

func (p *PerfService) memorySession() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return fmt.Sprintf("%s-%d", p.historyInstance, p.captureGeneration)
}
func (p *PerfService) rememberMemory(session, path string, sample MemorySample) (MemorySample, error) {
	var data []byte
	if path != "" {
		var err error
		data, err = os.ReadFile(path)
		if err != nil {
			return MemorySample{}, err
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if session != fmt.Sprintf("%s-%d", p.historyInstance, p.captureGeneration) {
		return MemorySample{}, errors.New("采集期间会话已切换，请重新采集")
	}
	p.memoryEntries = append(p.memoryEntries, upac.MemoryEntry{Time: time.Now().UnixMilli(), Source: sample.Source, Target: sample.Target, Rows: sample.Rows, Data: data})
	return sample, nil
}
func (p *PerfService) OpenMemorySnapshot(path string) (MemorySample, error) {
	return readMemorySnapshot(path)
}
func (p *PerfService) MemorySnapshotLibrary() (MemoryLibrary, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	session := fmt.Sprintf("%s-%d", p.historyInstance, p.captureGeneration)
	result := MemoryLibrary{Session: session, Records: []MemoryRecord{}}
	for i := len(p.memoryEntries) - 1; i >= 0; i-- {
		e := p.memoryEntries[i]
		result.Records = append(result.Records, MemoryRecord{ID: fmt.Sprintf("%s-%d", session, i), Session: session, Time: e.Time, Sample: MemorySample{Source: e.Source, Target: e.Target, Rows: e.Rows}})
	}
	return result, nil
}
func (p *PerfService) ShowMemoryWindow() {
	p.memoryWindowMu.Lock()
	defer p.memoryWindowMu.Unlock()
	app := application.Get()
	if w, ok := app.Window.GetByName("memory-snapshots"); ok {
		if w.IsMinimised() {
			w.Restore()
		}
		w.Show()
		w.Focus()
		return
	}
	app.Window.NewWithOptions(application.WebviewWindowOptions{Name: "memory-snapshots", Title: "内存快照 · unityperf", URL: "/?window=memory", Width: 1020, Height: 680, MinWidth: 700, MinHeight: 440, BackgroundColour: application.NewRGB(10, 12, 16), Windows: application.WindowsWindow{Theme: application.Dark}})
}
