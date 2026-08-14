// Package adbforward 用 adb forward 把 USB 手机上的 Unity Player 通道转到本机。
//
// 这不是性能数据采集通道：采集仍走 Editor Profiler 协议。
// 对应 Editor 对 Android USB 设备的做法：adb forward tcp:<local> localabstract:Unity-{bundleId}，
// 再作为 Profiler 客户端连接 127.0.0.1:<local>。
package adbforward

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"unityperfagent/internal/procutil"
	"unityperfagent/internal/unityprofiler"
)

// Runner 执行一条 adb 命令，返回合并后的 stdout/stderr。
type Runner func(ctx context.Context, args ...string) (string, error)

// DefaultRun 通过 PATH 中的 adb 执行命令（Windows 下隐藏控制台窗口）。
func DefaultRun(ctx context.Context, args ...string) (string, error) {
	cmd := procutil.CommandContext(ctx, "adb", args...)
	out, err := cmd.CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		if s != "" {
			return s, fmt.Errorf("adb %s: %s", strings.Join(args, " "), s)
		}
		return s, fmt.Errorf("adb %s: %w", strings.Join(args, " "), err)
	}
	return s, nil
}

func runWithTimeout(run Runner, timeout time.Duration, args ...string) (string, error) {
	if run == nil {
		run = DefaultRun
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return run(ctx, args...)
}

const adbTimeout = 8 * time.Second

// Device 是一台 adb 可见且状态为 device 的手机。
type Device struct {
	Serial string `json:"serial"`
	Model  string `json:"model"`
}

// Tunnel 是一条本机端口 → 手机 Player 的转发。
type Tunnel struct {
	Serial    string
	LocalPort int
	// Remote 是 adb forward 的远端，例如 localabstract:Unity-com.game 或 tcp:55000。
	Remote string
}

// ParseDevices 解析 `adb devices -l` 输出。
func ParseDevices(out string) []Device {
	var list []Device
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "List of devices") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[1] != "device" {
			continue
		}
		d := Device{Serial: fields[0]}
		for _, f := range fields[2:] {
			if m, ok := strings.CutPrefix(f, "model:"); ok && m != "" {
				d.Model = strings.ReplaceAll(m, "_", " ")
			}
		}
		list = append(list, d)
	}
	return list
}

// ListDevices 列出在线 USB/adb 设备。
func ListDevices(run Runner) ([]Device, error) {
	out, err := runWithTimeout(run, adbTimeout, "devices", "-l")
	if err != nil {
		return nil, err
	}
	return ParseDevices(out), nil
}

// ParseListeningTCPPorts 从 /proc/net/tcp 或 tcp6 文本中解析处于 LISTEN 的本地端口。
func ParseListeningTCPPorts(procNet string) []int {
	seen := map[int]struct{}{}
	var ports []int
	sc := bufio.NewScanner(strings.NewReader(procNet))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "sl") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		// local_address 形如 0100007F:D6D8 或 IPv6 地址:PORT
		local := fields[1]
		colon := strings.LastIndexByte(local, ':')
		if colon < 0 || colon == len(local)-1 {
			continue
		}
		st := fields[3]
		if !strings.EqualFold(st, "0A") {
			continue
		}
		p64, err := strconv.ParseInt(local[colon+1:], 16, 32)
		if err != nil || p64 <= 0 {
			continue
		}
		p := int(p64)
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		ports = append(ports, p)
	}
	return ports
}

// FilterPlayerPorts 保留 Unity Player Connection 默认端口范围。
func FilterPlayerPorts(ports []int) []int {
	var out []int
	for _, p := range ports {
		if p >= unityprofiler.PlayerPortMin && p <= unityprofiler.PlayerPortMax {
			out = append(out, p)
		}
	}
	return out
}

// ProbePlayerPorts 读取设备 /proc/net/tcp{,6}，找出 Player 监听端口。
// 探测失败时返回默认 55000，以便仍尝试 adb forward。
func ProbePlayerPorts(run Runner, serial string) []int {
	args := []string{"shell", "cat /proc/net/tcp /proc/net/tcp6 2>/dev/null"}
	if serial != "" {
		args = append([]string{"-s", serial}, args...)
	}
	out, err := runWithTimeout(run, adbTimeout, args...)
	if err != nil || out == "" {
		return []int{unityprofiler.DefaultPort}
	}
	found := FilterPlayerPorts(ParseListeningTCPPorts(out))
	if len(found) == 0 {
		return []int{unityprofiler.DefaultPort}
	}
	return found
}

// ForwardSpec 描述一条候选转发。
type ForwardSpec struct {
	Remote string // localabstract:Unity-xxx 或 tcp:55000
}

func (s ForwardSpec) String() string { return s.Remote }

var (
	pkgInActivity  = regexp.MustCompile(`([A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z][A-Za-z0-9_]*)+)/`)
	packageIDRe    = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z][A-Za-z0-9_]*)+$`)
	unityVersionRe = regexp.MustCompile(`Initialize engine version:\s*([0-9]+\.[0-9][^\s]*)`)
)

// ParseUnityAbstractSockets 从 /proc/net/unix 找出 Unity Player 抽象套接字名（不含 @）。
func ParseUnityAbstractSockets(procUnix string) []string {
	seen := map[string]struct{}{}
	var names []string
	sc := bufio.NewScanner(strings.NewReader(procUnix))
	for sc.Scan() {
		line := sc.Text()
		idx := strings.Index(line, "@Unity-")
		if idx < 0 {
			idx = strings.Index(line, " Unity-")
		}
		if idx < 0 {
			continue
		}
		rest := strings.TrimSpace(line[idx:])
		rest = strings.TrimPrefix(rest, "@")
		name := strings.Fields(rest)[0]
		if !strings.HasPrefix(name, "Unity-") {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

// NormalizePackageID 规整用户输入的包名（去空白、去掉 Unity- 前缀）。
func NormalizePackageID(pkg string) string {
	pkg = strings.TrimSpace(pkg)
	if strings.HasPrefix(pkg, "Unity-") {
		pkg = strings.TrimPrefix(pkg, "Unity-")
	}
	return strings.TrimSpace(pkg)
}

// ValidatePackageID 校验 Android 应用包名格式。
func ValidatePackageID(pkg string) error {
	pkg = NormalizePackageID(pkg)
	if pkg == "" {
		return fmt.Errorf("请输入应用包名")
	}
	if !packageIDRe.MatchString(pkg) {
		return fmt.Errorf("包名格式无效: %q", pkg)
	}
	return nil
}

func ParseForegroundPackage(dumpsys string) string {
	for _, key := range []string{"topResumedActivity", "mResumedActivity", "mFocusedActivity", "mCurrentFocus"} {
		i := strings.Index(dumpsys, key)
		if i < 0 {
			continue
		}
		chunk := dumpsys[i:]
		if len(chunk) > 500 {
			chunk = chunk[:500]
		}
		m := pkgInActivity.FindStringSubmatch(chunk)
		if len(m) > 1 {
			return m[1]
		}
	}
	return ""
}

// ParseUnityVersion 从 logcat 解析 Initialize engine version。
func ParseUnityVersion(logcat string) string {
	m := unityVersionRe.FindStringSubmatch(logcat)
	if len(m) > 1 {
		return strings.TrimRight(m[1], ")")
	}
	return ""
}

// ProbeUnitySockets 列出设备上的 Unity-{bundleId} 抽象套接字。
func ProbeUnitySockets(run Runner, serial string) []string {
	out, err := runWithTimeout(run, adbTimeout, adbArgs(serial, "shell", "cat /proc/net/unix 2>/dev/null")...)
	if err != nil || out == "" {
		return nil
	}
	return ParseUnityAbstractSockets(out)
}

// ForegroundPackage 读取前台 Activity 的包名。
func ForegroundPackage(run Runner, serial string) string {
	out, err := runWithTimeout(run, adbTimeout, adbArgs(serial, "shell", "dumpsys", "activity", "activities")...)
	if err != nil {
		return ""
	}
	return ParseForegroundPackage(out)
}

// ProbeUnityVersion 从 logcat 读取引擎版本。
func ProbeUnityVersion(run Runner, serial string) string {
	out, err := runWithTimeout(run, adbTimeout, adbArgs(serial, "logcat", "-d", "-t", "400", "-s", "Unity")...)
	if err != nil {
		return ""
	}
	return ParseUnityVersion(out)
}

// ProbeForwardSpecs 只给出 USB 官方通道 localabstract:Unity-{bundleId}。
// packageID 非空时仅使用该包名，不再探测设备；为空时从 /proc/net/unix 与前台应用解析。
// 禁止回退到 tcp:55000（那是 Wi‑Fi Direct Connection 端口，不是 USB Profiler 通道）。
func ProbeForwardSpecs(run Runner, serial, packageID string) []ForwardSpec {
	if pkg := NormalizePackageID(packageID); pkg != "" {
		return []ForwardSpec{{Remote: "localabstract:Unity-" + pkg}}
	}
	seen := map[string]struct{}{}
	var specs []ForwardSpec
	add := func(remote string) {
		if remote == "" {
			return
		}
		if _, ok := seen[remote]; ok {
			return
		}
		seen[remote] = struct{}{}
		specs = append(specs, ForwardSpec{Remote: remote})
	}
	for _, name := range ProbeUnitySockets(run, serial) {
		add("localabstract:" + name)
	}
	if pkg := ForegroundPackage(run, serial); pkg != "" {
		add("localabstract:Unity-" + pkg)
	}
	return specs
}

func adbArgs(serial string, rest ...string) []string {
	if serial == "" {
		return rest
	}
	return append([]string{"-s", serial}, rest...)
}

// Forward 执行 `adb forward tcp:0 tcp:remotePort`。
func Forward(run Runner, serial string, remotePort int) (Tunnel, error) {
	if remotePort <= 0 {
		remotePort = unityprofiler.DefaultPort
	}
	return ForwardRemote(run, serial, fmt.Sprintf("tcp:%d", remotePort))
}

// ForwardRemote 执行 `adb forward tcp:0 <remote>`，remote 如 localabstract:Unity-com.game。
func ForwardRemote(run Runner, serial, remote string) (Tunnel, error) {
	if remote == "" {
		remote = fmt.Sprintf("tcp:%d", unityprofiler.DefaultPort)
	}
	out, err := runWithTimeout(run, adbTimeout, adbArgs(serial, "forward", "tcp:0", remote)...)
	if err != nil {
		return Tunnel{}, err
	}
	local, err := strconv.Atoi(firstToken(out))
	if err != nil || local <= 0 {
		return Tunnel{}, fmt.Errorf("adb forward 未返回本机端口: %q", out)
	}
	return Tunnel{Serial: serial, LocalPort: local, Remote: remote}, nil
}

// RemoveStale 拆除本机指向同一 remote 的历史转发规则。
//
// 进程崩溃或强杀会留下规则，反复连接会不断堆积；同一 remote 只需要一条。
func RemoveStale(run Runner, serial, remote string) {
	out, err := runWithTimeout(run, adbTimeout, adbArgs(serial, "forward", "--list")...)
	if err != nil {
		return
	}
	for _, line := range strings.Split(out, "\n") {
		// 每行形如 `<serial> tcp:<port> <remote>`。
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 3 || fields[2] != remote {
			continue
		}
		if serial != "" && fields[0] != serial {
			continue
		}
		port, err := strconv.Atoi(strings.TrimPrefix(fields[1], "tcp:"))
		if err != nil || port <= 0 {
			continue
		}
		_ = Remove(run, Tunnel{Serial: serial, LocalPort: port, Remote: remote})
	}
}

// Remove 拆除 `adb forward tcp:localPort`。
func Remove(run Runner, t Tunnel) error {
	if t.LocalPort <= 0 {
		return nil
	}
	_, err := runWithTimeout(run, adbTimeout, adbArgs(t.Serial, "forward", "--remove", fmt.Sprintf("tcp:%d", t.LocalPort))...)
	return err
}

func firstToken(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n\t "); i >= 0 {
		return s[:i]
	}
	return s
}

// ParseHost 识别 UI 里的 USB 目标：usb / adb / adb:<serial>。
func ParseHost(host string) (serial string, viaADB bool) {
	host = strings.TrimSpace(host)
	switch {
	case host == "usb" || host == "adb":
		return "", true
	case strings.HasPrefix(host, "adb:"):
		return strings.TrimPrefix(host, "adb:"), true
	default:
		return "", false
	}
}

// ResolveSerial 将空 serial 解析为第一台在线设备。
func ResolveSerial(run Runner, serial string) (Device, error) {
	devices, err := ListDevices(run)
	if err != nil {
		return Device{}, err
	}
	if len(devices) == 0 {
		return Device{}, fmt.Errorf("未发现 USB 设备。请连接手机并开启 USB 调试（adb devices）")
	}
	if serial == "" {
		return devices[0], nil
	}
	for _, d := range devices {
		if d.Serial == serial {
			return d, nil
		}
	}
	return Device{}, fmt.Errorf("未找到 adb 设备 %s", serial)
}

// USBPlayers 把在线 adb 设备转换成可连接的 Player 条目（尚未建立转发）。
func USBPlayers(run Runner) ([]unityprofiler.PlayerInfo, error) {
	devices, err := ListDevices(run)
	if err != nil {
		return nil, err
	}
	out := make([]unityprofiler.PlayerInfo, 0, len(devices))
	for _, d := range devices {
		model := d.Model
		if model == "" {
			model = "Android"
			if m, err := runWithTimeout(run, adbTimeout, adbArgs(d.Serial, "shell", "getprop", "ro.product.model")...); err == nil {
				if m = strings.TrimSpace(m); m != "" {
					model = m
					d.Model = m
				}
			}
		}
		out = append(out, unityprofiler.PlayerInfo{
			IP:          "adb:" + d.Serial,
			Port:        0,
			Flags:       unityprofiler.FlagSupportsProfile | unityprofiler.FlagRequestImmediateConnect,
			GUID:        d.Serial,
			ID:          fmt.Sprintf("AndroidPlayer(USB:%s)", model),
			Debug:       true,
			PackageName: "adb",
			ProjectName: model,
			Transport:   "adb",
			Serial:      d.Serial,
		})
	}
	return out, nil
}
