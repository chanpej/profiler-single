package adbforward

import (
	"context"
	"strings"
	"testing"
)

func TestParseDevices(t *testing.T) {
	out := `List of devices attached
7ddbdf55               device product:polaris model:Mi_MIX_2S device:polaris transport_id:1
emulator-5554          offline
unauthorized           unauthorized
`
	got := ParseDevices(out)
	if len(got) != 1 || got[0].Serial != "7ddbdf55" || got[0].Model != "Mi MIX 2S" {
		t.Fatalf("got %+v", got)
	}
}

func TestParseDevicesEmpty(t *testing.T) {
	if got := ParseDevices("List of devices attached\n"); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestParseListeningTCPPorts(t *testing.T) {
	const proc = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:D6D8 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 1 1 0000000000000000 100 0 0 10 0
   1: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 2 1 0000000000000000 100 0 0 10 0
   2: 00000000:0050 8C0AA8C0:E1D1 01 00000000:00000000 00:00000000 00000000     0        0 3 1 0000000000000000 100 0 0 10 0
`
	ports := ParseListeningTCPPorts(proc)
	if len(ports) != 2 || ports[0] != 55000 || ports[1] != 8080 {
		t.Fatalf("ports=%v", ports)
	}
	player := FilterPlayerPorts(ports)
	if len(player) != 1 || player[0] != 55000 {
		t.Fatalf("player=%v", player)
	}
}

func TestParseListeningTCP6(t *testing.T) {
	const proc = `  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000000000000:D6E8 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 1 1 0000000000000000 100 0 0 10 0
`
	ports := FilterPlayerPorts(ParseListeningTCPPorts(proc))
	if len(ports) != 1 || ports[0] != 55016 {
		t.Fatalf("ports=%v", ports)
	}
}

func TestForwardAndUSBPlayers(t *testing.T) {
	run := func(_ context.Context, args ...string) (string, error) {
		cmd := strings.Join(args, " ")
		switch {
		case strings.Contains(cmd, "devices"):
			return "List of devices attached\n7ddbdf55 device product:polaris model:Mi_MIX_2S\n", nil
		case strings.Contains(cmd, "/proc/net/tcp"):
			return "  sl  local_address rem_address   st\n   0: 00000000:D6D8 00000000:0000 0A 0 0 0 0 0 0 0 0 0 0 0 0 0 0\n", nil
		case strings.HasPrefix(cmd, "-s 7ddbdf55 forward tcp:0 tcp:55000") || cmd == "forward tcp:0 tcp:55000":
			return "61234\n", nil
		case strings.Contains(cmd, "forward --remove"):
			return "", nil
		case strings.Contains(cmd, "getprop"):
			return "Mi MIX 2S\n", nil
		default:
			t.Fatalf("unexpected adb: %s", cmd)
			return "", nil
		}
	}

	devs, err := ListDevices(run)
	if err != nil || len(devs) != 1 {
		t.Fatalf("ListDevices: %v %+v", err, devs)
	}

	ports := ProbePlayerPorts(run, "7ddbdf55")
	if len(ports) != 1 || ports[0] != 55000 {
		t.Fatalf("ports=%v", ports)
	}

	tun, err := Forward(run, "7ddbdf55", 55000)
	if err != nil {
		t.Fatal(err)
	}
	if tun.LocalPort != 61234 || tun.Remote != "tcp:55000" || tun.Serial != "7ddbdf55" {
		t.Fatalf("tunnel=%+v", tun)
	}
	if err := Remove(run, tun); err != nil {
		t.Fatal(err)
	}

	players, err := USBPlayers(run)
	if err != nil {
		t.Fatal(err)
	}
	if len(players) != 1 {
		t.Fatalf("players=%+v", players)
	}
	p := players[0]
	if p.Transport != "adb" || p.Serial != "7ddbdf55" || p.IP != "adb:7ddbdf55" {
		t.Fatalf("player=%+v", p)
	}
	if !strings.Contains(p.ID, "USB") {
		t.Fatalf("id=%q", p.ID)
	}
}

func TestProbeFallsBackToDefault(t *testing.T) {
	run := func(_ context.Context, args ...string) (string, error) {
		return "", nil
	}
	ports := ProbePlayerPorts(run, "x")
	if len(ports) != 1 || ports[0] != 55000 {
		t.Fatalf("ports=%v", ports)
	}
}

func TestParseHost(t *testing.T) {
	serial, ok := ParseHost("usb")
	if !ok || serial != "" {
		t.Fatalf("usb -> %q %v", serial, ok)
	}
	serial, ok = ParseHost("adb:7ddbdf55")
	if !ok || serial != "7ddbdf55" {
		t.Fatalf("adb: -> %q %v", serial, ok)
	}
	if _, ok := ParseHost("192.168.1.20"); ok {
		t.Fatal("ip should not be adb")
	}
}

func TestForwardBadOutput(t *testing.T) {
	run := func(_ context.Context, args ...string) (string, error) {
		return "not-a-port", nil
	}
	if _, err := Forward(run, "s", 55000); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseUnityAbstractSockets(t *testing.T) {
	const proc = `Num       RefCount Protocol Flags    Type St Inode Path
0000000000000000: 00000002 00000000 00010000 0001 01  1234 @Unity-com.company.game
0000000000000000: 00000002 00000000 00010000 0001 01  5678 /dev/socket/logdw
`
	got := ParseUnityAbstractSockets(proc)
	if len(got) != 1 || got[0] != "Unity-com.company.game" {
		t.Fatalf("got %v", got)
	}
}

func TestParseForegroundPackage(t *testing.T) {
	dump := `  mResumedActivity: ActivityRecord{e060a9d u0 com.company.game/.MainActivity t296}
`
	if got := ParseForegroundPackage(dump); got != "com.company.game" {
		t.Fatalf("got %q", got)
	}
}

func TestParseUnityVersion(t *testing.T) {
	log := `08-13 19:00:00.000  1234  1250 I Unity   : Initialize engine version: 2022.3.45f1 (abc)
`
	if got := ParseUnityVersion(log); got != "2022.3.45f1" {
		t.Fatalf("got %q", got)
	}
}

func TestProbeForwardSpecsPrefersAbstract(t *testing.T) {
	run := func(_ context.Context, args ...string) (string, error) {
		cmd := strings.Join(args, " ")
		switch {
		case strings.Contains(cmd, "/proc/net/unix"):
			return "@Unity-com.company.game\n", nil
		case strings.Contains(cmd, "dumpsys"):
			return "mResumedActivity: ActivityRecord{x u0 com.company.game/.Main t1}\n", nil
		default:
			t.Fatalf("unexpected %s", cmd)
			return "", nil
		}
	}
	specs := ProbeForwardSpecs(run, "s", "")
	if len(specs) != 1 {
		t.Fatalf("specs=%v", specs)
	}
	if specs[0].Remote != "localabstract:Unity-com.company.game" {
		t.Fatalf("first spec=%v, want localabstract", specs[0])
	}
}

func TestProbeForwardSpecsExplicitPackage(t *testing.T) {
	run := func(_ context.Context, args ...string) (string, error) {
		t.Fatalf("unexpected adb when package id is set: %v", args)
		return "", nil
	}
	specs := ProbeForwardSpecs(run, "s", "com.example.app")
	if len(specs) != 1 || specs[0].Remote != "localabstract:Unity-com.example.app" {
		t.Fatalf("specs=%v", specs)
	}
}

func TestValidatePackageID(t *testing.T) {
	if err := ValidatePackageID(""); err == nil {
		t.Fatal("expected error for empty package")
	}
	if err := ValidatePackageID("com.company.game"); err != nil {
		t.Fatalf("valid package: %v", err)
	}
	if err := ValidatePackageID("Unity-com.company.game"); err != nil {
		t.Fatalf("Unity- prefix should be stripped: %v", err)
	}
	if err := ValidatePackageID("not a package"); err == nil {
		t.Fatal("expected error for invalid package")
	}
}

func TestProbeForwardSpecsNoTCPFallback(t *testing.T) {
	run := func(_ context.Context, args ...string) (string, error) {
		return "", nil
	}
	specs := ProbeForwardSpecs(run, "s", "")
	if len(specs) != 0 {
		t.Fatalf("specs=%v, want none without localabstract", specs)
	}
}

func TestForwardRemoteAbstract(t *testing.T) {
	run := func(_ context.Context, args ...string) (string, error) {
		cmd := strings.Join(args, " ")
		if !strings.Contains(cmd, "localabstract:Unity-com.game") {
			t.Fatalf("cmd=%s", cmd)
		}
		return "34999\n", nil
	}
	tun, err := ForwardRemote(run, "s", "localabstract:Unity-com.game")
	if err != nil {
		t.Fatal(err)
	}
	if tun.LocalPort != 34999 || tun.Remote != "localabstract:Unity-com.game" {
		t.Fatalf("%+v", tun)
	}
}

// TestRemoveStale 校验只清理同一设备、同一 remote 的历史规则：
// 反复连接会堆积 tcp:0 规则，误删别的 remote（例如 Editor 建的）会打断其他工具。
func TestRemoveStale(t *testing.T) {
	list := "7ddbdf55 tcp:55000 localabstract:Unity-com.game\n" +
		"7ddbdf55 tcp:62539 localabstract:Unity-com.game\n" +
		"7ddbdf55 tcp:41000 localabstract:Unity-com.other\n" +
		"other tcp:41001 localabstract:Unity-com.game\n"
	var removed []string
	run := func(_ context.Context, args ...string) (string, error) {
		cmd := strings.Join(args, " ")
		switch {
		case strings.Contains(cmd, "forward --list"):
			return list, nil
		case strings.Contains(cmd, "forward --remove"):
			removed = append(removed, args[len(args)-1])
			return "", nil
		default:
			t.Fatalf("unexpected adb: %s", cmd)
			return "", nil
		}
	}
	RemoveStale(run, "7ddbdf55", "localabstract:Unity-com.game")
	if strings.Join(removed, ",") != "tcp:55000,tcp:62539" {
		t.Fatalf("removed=%v", removed)
	}
}

func TestResolveSerial(t *testing.T) {
	run := func(_ context.Context, args ...string) (string, error) {
		return "List of devices attached\n7ddbdf55 device model:Pixel_7\n", nil
	}
	d, err := ResolveSerial(run, "")
	if err != nil || d.Serial != "7ddbdf55" {
		t.Fatalf("empty serial: %+v %v", d, err)
	}
	d, err = ResolveSerial(run, "7ddbdf55")
	if err != nil || d.Model != "Pixel 7" {
		t.Fatalf("named: %+v %v", d, err)
	}
	if _, err := ResolveSerial(run, "missing"); err == nil {
		t.Fatal("expected missing serial error")
	}

	empty := func(_ context.Context, args ...string) (string, error) {
		return "List of devices attached\n", nil
	}
	if _, err := ResolveSerial(empty, ""); err == nil {
		t.Fatal("expected no-device error")
	}
}
