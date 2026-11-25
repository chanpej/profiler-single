package services

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"unityperf/internal/adbforward"
)

func wirelessEndpoint(value string) (string, error) {
	value = strings.TrimSpace(value)
	endpoint, err := netip.ParseAddrPort(value)
	if err != nil {
		ip, err := netip.ParseAddr(value)
		if err != nil {
			return "", errors.New("请输入手机 Wi-Fi IP 或 IP:连接端口")
		}
		endpoint = netip.AddrPortFrom(ip, 5555)
	}
	if endpoint.Port() == 0 || !endpoint.Addr().IsGlobalUnicast() {
		return "", errors.New("请输入有效的手机 Wi-Fi 地址和端口")
	}
	return endpoint.String(), nil
}

func connectScreenDevice(ctx context.Context, run adbforward.Runner, endpoint string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	message, err := run(ctx, "connect", endpoint)
	if err != nil {
		return err
	}
	state, err := run(ctx, "-s", endpoint, "get-state")
	if err != nil || strings.TrimSpace(state) != "device" {
		return fmt.Errorf("无线 ADB %s 未就绪（%s）。请确认手机已授权，地址使用连接端口。", endpoint, message)
	}
	return nil
}

// EnableWirelessScreenADB prepares a single USB-connected Android device before profiling.
func (p *PerfService) EnableWirelessScreenADB() (string, error) {
	p.mu.Lock()
	active := p.connectedLocked() || p.tunnel != nil
	run := p.adbRun
	p.mu.Unlock()
	if active {
		return "", errors.New("请在连接 Player 前通过 USB 开启无线 ADB")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	serial, err := run(ctx, "-d", "get-serialno")
	if err != nil {
		return "", fmt.Errorf("请连接一台已授权的 USB 手机：%w", err)
	}
	serial = strings.TrimSpace(serial)
	info, err := run(ctx, "-s", serial, "shell", "ip", "-4", "addr", "show", "wlan0")
	if err != nil {
		return "", err
	}
	fields := strings.Fields(info)
	for i, field := range fields {
		if field != "inet" || i+1 >= len(fields) {
			continue
		}
		prefix, err := netip.ParsePrefix(fields[i+1])
		if err != nil || !prefix.Addr().Is4() {
			continue
		}
		endpoint, err := wirelessEndpoint(prefix.Addr().String())
		if err != nil {
			return "", err
		}
		if _, err = run(ctx, "-s", serial, "tcpip", "5555"); err != nil {
			return "", err
		}
		return endpoint, nil
	}
	return "", errors.New("未取得 wlan0 的 IPv4 地址，请检查手机 Wi-Fi，或手动输入已开启的无线调试地址")
}
