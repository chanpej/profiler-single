package unityprofiler

import (
	"context"
	"net"
	"strconv"
	"sync"
	"time"
)

// Discover 在 timeout 内监听 Unity Player 组播，返回去重后的 Development Player 列表。
// 与 Editor Attach to Player 下拉框的发现机制相同。
func Discover(ctx context.Context, timeout time.Duration) ([]PlayerInfo, error) {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	group := net.ParseIP(MulticastGroup)
	var mu sync.Mutex
	seen := map[string]PlayerInfo{}
	var conns []*net.UDPConn
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	for _, port := range MulticastPorts {
		addr := &net.UDPAddr{IP: group, Port: port}
		c, err := net.ListenMulticastUDP("udp4", nil, addr)
		if err != nil {
			continue
		}
		_ = c.SetReadBuffer(4096)
		conns = append(conns, c)
		go func(conn *net.UDPConn) {
			buf := make([]byte, 2048)
			for {
				_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
				n, _, err := conn.ReadFromUDP(buf)
				if ctx.Err() != nil {
					return
				}
				if err != nil || n == 0 {
					continue
				}
				info, err := ParseAnnouncement(string(buf[:n]))
				if err != nil {
					continue
				}
				key := net.JoinHostPort(info.IP, strconv.Itoa(info.Port))
				mu.Lock()
				seen[key] = info
				mu.Unlock()
			}
		}(c)
	}
	if len(conns) == 0 {
		return nil, nil
	}
	<-ctx.Done()
	mu.Lock()
	out := make([]PlayerInfo, 0, len(seen))
	for _, v := range seen {
		out = append(out, v)
	}
	mu.Unlock()
	return out, nil
}
