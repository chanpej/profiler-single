package services

import (
	"context"
	"time"

	"unityperf/internal/adbforward"
)

const playerLogCapacity = 500

type PlayerLog struct {
	ID         int64  `json:"id"`
	ReceivedAt int64  `json:"receivedAt"`
	Level      string `json:"level"`
	Text       string `json:"text"`
}

type playerLogBuffer struct {
	entries [playerLogCapacity]PlayerLog
	next    int64
	count   int64
}

func (buffer *playerLogBuffer) append(level, text string) {
	buffer.next++
	buffer.entries[(buffer.next-1)%playerLogCapacity] = PlayerLog{buffer.next, time.Now().UnixMilli(), level, text}
	if buffer.count < playerLogCapacity {
		buffer.count++
	}
}

func (p *PerfService) PlayerLogs(after int64) []PlayerLog {
	p.mu.Lock()
	defer p.mu.Unlock()
	buffer := &p.playerLogs
	start := max(after, buffer.next-buffer.count)
	if start >= buffer.next {
		return nil
	}
	logs := make([]PlayerLog, 0, buffer.next-start)
	for index := start; index < buffer.next; index++ {
		logs = append(logs, buffer.entries[index%playerLogCapacity])
	}
	return logs
}

func (p *PerfService) onPlayerLog(level, text string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if text != "" && (p.incoming != nil || p.transport == "adb") {
		p.playerLogs.append(level, text)
	}
}

func (p *PerfService) startUSBLogsLocked(run adbforward.Runner, serial, packageID string) {
	if p.usbLogCancel != nil {
		p.usbLogCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.usbLogCancel = cancel
	go func() {
		err := adbforward.StreamUnityLogs(ctx, run, serial, packageID, func(level, text string) {
			p.mu.Lock()
			defer p.mu.Unlock()
			if ctx.Err() == nil && p.transport == "adb" {
				p.playerLogs.append(level, text)
			}
		})
		if err != nil && ctx.Err() == nil {
			p.logf("读取 Android Unity 日志失败: %v", err)
		}
	}()
}
