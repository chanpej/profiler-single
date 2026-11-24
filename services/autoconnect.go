package services

import (
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"

	"unityperf/internal/protocol"
	"unityperf/internal/unityprofiler"
)

const autoConnectTransport = "autoconnect"
const defaultAutoconnectPort = 37100

func (p *PerfService) startDefaultAutoconnect(port int) {
	if err := p.listenAutoconnect(port); err != nil {
		p.mu.Lock()
		p.lastErr = err.Error()
		p.mu.Unlock()
		p.logf("%v", err)
		p.notifyUI()
	}
}

func (p *PerfService) AutoconnectAddresses() []string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var addresses []net.Addr
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if assigned, err := iface.Addrs(); err == nil {
			addresses = append(addresses, assigned...)
		}
	}
	return autoconnectIPv4(addresses)
}

func autoconnectIPv4(addresses []net.Addr) []string {
	var result []string
	for _, address := range addresses {
		ip, _, err := net.ParseCIDR(address.String())
		if err != nil {
			ip = net.ParseIP(address.String())
		}
		if ip.To4() != nil && ip.IsGlobalUnicast() {
			result = append(result, ip.String())
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func (p *PerfService) StartAutoconnect(port int) error {
	p.connectionMu.Lock()
	defer p.connectionMu.Unlock()
	return p.listenAutoconnect(port)
}

func (p *PerfService) listenAutoconnect(port int) error {
	if port < 1 || port > 65535 {
		return errors.New("自动连接端口必须在 1–65535 之间")
	}
	listener, err := net.Listen("tcp4", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("无法监听小游戏端口 %d，请关闭占用此端口的团结 Editor 或 WebSocket 代理：%w", port, err)
	}
	if err := p.startAutoconnect(listener); err != nil {
		_ = listener.Close()
		return err
	}
	return nil
}

func (p *PerfService) startAutoconnect(listener net.Listener) error {
	p.mu.Lock()
	if p.connectedLocked() || p.wantLive || p.incoming != nil {
		p.mu.Unlock()
		return errors.New("已有连接或正在等待连接，请先断开")
	}
	p.session.ClearAll()
	p.resetScreenLocked(true)
	p.transport = autoConnectTransport
	p.sourceFile = ""
	p.loadedHS = protocol.Handshake{}
	p.liveHS = protocol.Handshake{}
	p.lastErr = ""
	p.wantLive = true
	p.reconnecting = true
	p.recording = true
	p.captureRaw = true
	p.incoming = unityprofiler.ServeAutoconnect(listener, p.handleAutoconnect)
	p.incomingPort = listener.Addr().(*net.TCPAddr).Port
	p.mu.Unlock()
	p.notifyUI()
	return nil
}

func (p *PerfService) handleAutoconnect(conn net.Conn) {
	ended := make(chan error, 1)
	hs, err := p.client.ConnectIncoming(conn, p.onFrame, func(err error) { ended <- err })
	if err == nil {
		p.mu.Lock()
		current := p.incoming != nil
		if current {
			p.liveHS = hs
			p.lastErr = ""
			p.reconnecting = false
		}
		deep, stacks := p.deep, p.callStacks
		p.mu.Unlock()
		if current {
			if deep || stacks {
				_ = p.client.SetProfilerOptions(deep, stacks)
			}
			p.notifyUI()
		}
		err = <-ended
	}

	p.mu.Lock()
	if p.incoming != nil {
		p.reconnecting = true
		if err != nil && !unityprofiler.IsLostConnection(err) {
			p.lastErr = strings.TrimPrefix(err.Error(), "unityprofiler: ")
		}
	}
	p.mu.Unlock()
	p.notifyUI()
}
