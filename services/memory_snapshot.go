package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"unityperf/internal/adbforward"
	"unityperf/internal/memsnap"
)

type MemorySample struct {
	Source string        `json:"source"`
	Target string        `json:"target"`
	Rows   []memsnap.Row `json:"rows"`
}

func readMemorySnapshot(path string) (MemorySample, error) {
	rows, err := memsnap.NativeTypes(path)
	return MemorySample{Source: "snap", Target: filepath.Base(path), Rows: rows}, err
}

var activityPID = regexp.MustCompile(`(?m)^\s*ACTIVITY ([A-Za-z0-9_.]+)/\S+ \S+ pid=([1-9][0-9]*)`)
var packageName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*(\.[a-zA-Z0-9_]+)+$`)
var summaryLine = regexp.MustCompile(`^\s*(Java Heap|Native Heap|Code|Stack|Graphics|Private Other|System|Unknown):\s+([0-9]+)`)

func (p *PerfService) memoryTarget(ctx context.Context) (adbforward.Runner, string, string, string, error) {
	p.mu.Lock()
	run, serial, host := p.adbRun, p.usbSerial, p.liveHost
	p.mu.Unlock()
	if serial == "" {
		devices, err := adbforward.ListDevices(run)
		if err != nil {
			return nil, "", "", "", err
		}
		if len(devices) > 1 && host == "" {
			out, e := run(ctx, "-d", "get-serialno")
			if e == nil {
				serial = strings.TrimSpace(out)
			}
		}
		if serial == "" {
			serial, err = selectScreenDevice(ctx, run, host)
			if err != nil {
				return nil, "", "", "", err
			}
		}
	}
	out, err := run(ctx, "-s", serial, "shell", "dumpsys", "activity", "top")
	if err != nil {
		return nil, "", "", "", err
	}
	matches := activityPID.FindAllStringSubmatch(out, -1)
	if len(matches) == 0 {
		return nil, "", "", "", errors.New("无法定位前台 Activity 的进程，请打开目标游戏")
	}
	target := matches[len(matches)-1]
	if !packageName.MatchString(target[1]) {
		return nil, "", "", "", errors.New("无效的前台应用包名")
	}
	return run, serial, target[1], target[2], nil
}
func parsePSS(out string) ([]memsnap.Row, error) {
	_, text, ok := strings.Cut(out, "App Summary")
	if !ok {
		return nil, errors.New("Android 未返回 App Summary")
	}
	var rows []memsnap.Row
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "TOTAL") {
			break
		}
		m := summaryLine.FindStringSubmatch(line)
		if len(m) == 3 {
			v, e := strconv.ParseUint(m[2], 10, 52)
			if e != nil {
				return nil, e
			}
			rows = append(rows, memsnap.Row{Name: m[1], Bytes: v * 1024})
		}
	}
	if len(rows) == 0 {
		return nil, errors.New("无法识别 PSS 分类")
	}
	return rows, nil
}
func (p *PerfService) CaptureAndroidMemory() (MemorySample, error) {
	session := p.memorySession()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	run, serial, pkg, pid, err := p.memoryTarget(ctx)
	if err != nil {
		return MemorySample{}, err
	}
	out, err := run(ctx, "-s", serial, "shell", "dumpsys", "meminfo", pid)
	if err != nil {
		return MemorySample{}, err
	}
	rows, err := parsePSS(out)
	label := pkg + " PID " + pid
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "MEMINFO in pid "+pid+" [") {
			label = strings.Trim(line, " *\r\t")
			break
		}
	}
	if err != nil {
		return MemorySample{}, err
	}
	return p.rememberMemory(session, "", MemorySample{Source: "pss", Target: label, Rows: rows})
}

func (p *PerfService) CaptureUnityMemory() (MemorySample, error) {
	session := p.memorySession()
	var empty MemorySample
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	run, serial, pkg, _, err := p.memoryTarget(ctx)
	if err != nil {
		return empty, err
	}
	adb := func(args ...string) (string, error) { return run(ctx, append([]string{"-s", serial}, args...)...) }
	root := "/sdcard/Android/data/" + pkg + "/files/unityperf-memory/"
	if _, err = adb("shell", "test", "-d", root); err != nil {
		return empty, fmt.Errorf("%s 尚未启用快照采集。在 Unity 2022 Android Development Build 中加入 UnityPerfMemoryCapture.cs，或导入已有 .snap。", pkg)
	}
	var id [16]byte
	if _, err = rand.Read(id[:]); err != nil {
		return empty, err
	}
	prefix := root + hex.EncodeToString(id[:])
	if _, err = adb("shell", "touch", prefix+".request"); err != nil {
		return empty, err
	}
	for {
		if _, err = adb("shell", "test", "-f", prefix+".ready"); err == nil {
			break
		}
		if _, err = adb("shell", "test", "-f", prefix+".failed"); err == nil {
			return empty, errors.New("Unity 报告快照采集失败")
		}
		select {
		case <-ctx.Done():
			return empty, fmt.Errorf("等待 Unity 快照超时：%w", ctx.Err())
		case <-time.After(time.Second):
		}
	}
	tmp, err := os.CreateTemp("", ".unityperf-snap-*")
	if err != nil {
		return empty, err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	if _, err = adb("pull", prefix+".snap", tmp.Name()); err != nil {
		return empty, err
	}
	result, err := readMemorySnapshot(tmp.Name())
	if err != nil {
		return empty, err
	}
	_, _ = adb("shell", "rm", "-f", prefix+".snap", prefix+".ready")
	result.Target = "Memory " + time.Now().Format("15:04:05.000")
	return p.rememberMemory(session, tmp.Name(), result)
}

// WaitMiniGameSnapshot accepts only files created or replaced after the user starts listening.
func (p *PerfService) WaitMiniGameSnapshot(directory string) (MemorySample, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	return waitEditorSnapshot(ctx, directory, time.Second)
}

type snapshotStamp struct {
	size     int64
	modified time.Time
}

func snapFiles(directory string) (map[string]snapshotStamp, error) {
	files, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	result := make(map[string]snapshotStamp)
	for _, file := range files {
		if file.IsDir() || !strings.EqualFold(filepath.Ext(file.Name()), ".snap") {
			continue
		}
		info, err := file.Info()
		if err == nil && info.Mode().IsRegular() {
			result[file.Name()] = snapshotStamp{info.Size(), info.ModTime()}
		}
	}
	return result, nil
}
func waitEditorSnapshot(ctx context.Context, directory string, interval time.Duration) (MemorySample, error) {
	before, err := snapFiles(directory)
	if err != nil {
		return MemorySample{}, err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var previous map[string]snapshotStamp
	for {
		select {
		case <-ctx.Done():
			return MemorySample{}, errors.New("未收到完整快照，请检查所选目录和团结 Editor 的采集目标")
		case <-ticker.C:
			current, err := snapFiles(directory)
			if err != nil {
				return MemorySample{}, err
			}
			for name, stamp := range current {
				if stamp == before[name] || stamp != previous[name] || stamp.size < 32 {
					continue
				}
				rows, err := memsnap.NativeTypes(filepath.Join(directory, name))
				if err == nil {
					return MemorySample{Source: "snap", Target: name, Rows: rows}, nil
				}
			}
			previous = current
		}
	}
}

func (p *PerfService) CaptureMiniGameMemory() (MemorySample, error) {
	session := p.memorySession()
	var empty MemorySample
	p.mu.Lock()
	connected := p.transport == autoConnectTransport && p.connectedLocked()
	p.mu.Unlock()
	if !connected {
		return empty, errors.New("请在小程序模式开启监听并连接小游戏 Player")
	}
	f, err := os.CreateTemp("", ".minigame-snapshot-*")
	if err != nil {
		return empty, err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	data, err := p.client.CaptureMemory(ctx)
	if err != nil {
		return empty, err
	}
	if err = memsnap.ConvertPlayerSnapshot(data, f); err != nil {
		return empty, err
	}
	if err = f.Close(); err != nil {
		return empty, err
	}
	sample, err := readMemorySnapshot(f.Name())
	if err != nil {
		return empty, err
	}
	sample.Target = "Memory " + time.Now().Format("15:04:05.000")
	return p.rememberMemory(session, f.Name(), sample)
}
