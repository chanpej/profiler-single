package adbforward

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"unityperf/internal/procutil"
)

var unityLogLine = regexp.MustCompile(`^([VDIWEAF])/Unity\s*\(\s*[0-9]+\): ?(.*)$`)

func parseUnityLog(line string) (level, text string, ok bool) {
	parts := unityLogLine.FindStringSubmatch(line)
	if parts == nil {
		return "", "", false
	}
	level = "Log"
	switch parts[1] {
	case "W":
		level = "Warning"
	case "E", "F":
		level = "Error"
	case "A":
		level = "Assert"
	}
	return level, parts[2], true
}

func StreamUnityLogs(ctx context.Context, run Runner, serial, packageID string, emit func(string, string)) error {
	if err := ValidatePackageID(packageID); err != nil {
		return err
	}
	probeCtx, cancel := context.WithTimeout(ctx, adbTimeout)
	out, err := run(probeCtx, adbArgs(serial, "shell", "pidof", "-s", packageID)...)
	cancel()
	if err != nil {
		return err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil || pid <= 0 {
		return fmt.Errorf("未找到 Unity 应用进程 %s", packageID)
	}
	cmd := procutil.CommandContext(ctx, "adb", adbArgs(serial, "logcat", "--pid", strconv.Itoa(pid), "-v", "brief", "-T", "1", "-s", "Unity:V")...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 8192), 256<<10)
	for scanner.Scan() {
		if level, text, ok := parseUnityLog(scanner.Text()); ok {
			if len(text) > 4096 {
				text = strings.ToValidUTF8(text[:4096], "�")
			}
			emit(level, text)
		}
	}
	if err := scanner.Err(); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}
	return cmd.Wait()
}
