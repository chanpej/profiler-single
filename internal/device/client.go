// Package device 实现连到 Unity 2022 Development Player 的 Profiler 客户端。
// 采集协议见 internal/unityprofiler。
package device

import (
	"net"

	"unityperf/internal/protocol"
	"unityperf/internal/unityprofiler"
)

// FrameHandler 在收到每帧 Profiler 数据时被调用。
type FrameHandler = unityprofiler.FrameHandler

// ErrNotConnected 表示客户端尚未连接。
var ErrNotConnected = unityprofiler.ErrNotConnected

// Client 以 Unity Editor Profiler 身份连接真机 Development Build。
type Client struct {
	sess *unityprofiler.Session
}

// NewClient 创建客户端。
func NewClient() *Client {
	return &Client{sess: unityprofiler.NewSession()}
}

func (c *Client) SetLogHandler(fn func(string, string)) {
	c.sess.SetLogHandler(fn)
}

// Connect 连接到 host:port 上的 Unity Player（默认端口 55000），校验 Unity 2022。
func (c *Client) Connect(host string, port int, onFrame FrameHandler, onClosed func(error)) (protocol.Handshake, error) {
	if c.sess == nil {
		c.sess = unityprofiler.NewSession()
	}
	return c.sess.Connect(host, port, onFrame, onClosed)
}

func (c *Client) ConnectDirect(host string, onFrame FrameHandler, onClosed func(error)) (protocol.Handshake, int, error) {
	return c.sess.ConnectDirect(host, onFrame, onClosed)
}

// Handshake 返回当前 Player 信息。
func (c *Client) Handshake() (protocol.Handshake, error) {
	return c.sess.Handshake()
}

func (c *Client) ConnectIncoming(conn net.Conn, onFrame FrameHandler, onClosed func(error)) (protocol.Handshake, error) {
	return c.sess.ConnectIncoming(conn, onFrame, onClosed)
}

// PatchHandshake 补全握手字段（例如 USB 下的 Unity 版本）。
func (c *Client) PatchHandshake(fn func(*protocol.Handshake)) {
	if c.sess != nil {
		c.sess.PatchHandshake(fn)
	}
}

// DecodeStats 返回解码计数，便于排查"连上了但没有帧"的情况。
func (c *Client) DecodeStats() unityprofiler.DecodeStats {
	if c.sess == nil {
		return unityprofiler.DecodeStats{}
	}
	return c.sess.DecodeStats()
}

// Connected 返回是否已连接。
func (c *Client) Connected() bool {
	return c.sess != nil && c.sess.Connected()
}

// SetProfilerOptions 向已连接的 Player 下发 Deep Profile / Call Stacks。
func (c *Client) SetProfilerOptions(deep, callStacks bool) error {
	if c.sess == nil {
		return ErrNotConnected
	}
	return c.sess.SetProfilerOptions(deep, callStacks)
}

// ProfilerOptions 返回当前 Deep Profile / Call Stacks 状态。
func (c *Client) ProfilerOptions() (deep, callStacks bool) {
	if c.sess == nil {
		return false, false
	}
	return c.sess.ProfilerOptions()
}

// SetRawHandler 注册原始 Profiler 报文体回调，在解码前原样送达。
func (c *Client) SetRawHandler(fn func([]byte)) {
	if c.sess != nil {
		c.sess.SetRawHandler(fn)
	}
}

// Close 断开 Profiler 连接。
func (c *Client) Close() {
	if c.sess != nil {
		c.sess.Close()
	}
}
