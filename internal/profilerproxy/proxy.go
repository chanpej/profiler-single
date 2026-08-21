// Package profilerproxy provides a transparent PlayerConnection proxy for
// capturing the exact PD3U messages Unity Editor receives from a Player.
package profilerproxy

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"

	"unityperfagent/internal/unityprofiler"
)

const (
	captureMagic   uint32 = 0x43505055 // "UPPC"
	captureVersion uint16 = 1
	captureHeader  int    = 8
	maxCaptureSize        = 1 << 30
)

var (
	ErrCaptureFormat   = errors.New("无效的 Profiler proxy 抓包文件")
	ErrCaptureTooLarge = errors.New("Profiler proxy 抓包超过大小上限")
)

// Config defines one transparent Editor-to-Player proxy session.
type Config struct {
	ListenAddr  string
	TargetAddr  string
	CapturePath string
}

// Capture contains Player-to-Editor MsgIDProfilerData bodies in their original
// receive order. Each body is either the PD3U stream header or a PD3U chunk.
type Capture struct {
	Bodies [][]byte
}

// Run accepts one Unity Editor connection, relays it to the configured Player,
// and saves the captured PD3U messages when the session ends.
func Run(ctx context.Context, cfg Config) error {
	if cfg.ListenAddr == "" || cfg.TargetAddr == "" || cfg.CapturePath == "" {
		return errors.New("listen、target 和 out 参数不能为空")
	}
	listener, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return err
	}
	defer listener.Close()

	stopAccept := make(chan struct{})
	defer close(stopAccept)
	go func() {
		select {
		case <-ctx.Done():
			_ = listener.Close()
		case <-stopAccept:
		}
	}()

	editor, err := listener.Accept()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	defer editor.Close()

	player, err := (&net.Dialer{}).DialContext(ctx, "tcp", cfg.TargetAddr)
	if err != nil {
		return fmt.Errorf("连接 Player %s 失败：%w", cfg.TargetAddr, err)
	}
	defer player.Close()

	capture, relayErr := Relay(editor, player)
	if err := WriteFile(cfg.CapturePath, capture); err != nil {
		return fmt.Errorf("写入 PD3U 抓包失败：%w", err)
	}
	return relayErr
}

// Relay transparently forwards messages between Editor and Player. It returns
// the Player-to-Editor PD3U bodies even after either peer closes the session.
func Relay(editor, player net.Conn) (Capture, error) {
	var capture Capture
	var mu sync.Mutex
	appendCapture := func(body []byte) error {
		mu.Lock()
		defer mu.Unlock()
		if captureSize(capture.Bodies)+len(body) > maxCaptureSize {
			return ErrCaptureTooLarge
		}
		copyBody := append([]byte(nil), body...)
		capture.Bodies = append(capture.Bodies, copyBody)
		return nil
	}

	errs := make(chan error, 2)
	go func() { errs <- forward(editor, player, nil) }()
	go func() { errs <- forward(player, editor, appendCapture) }()

	first := <-errs
	_ = editor.Close()
	_ = player.Close()
	second := <-errs
	if err := relayError(first, second); err != nil {
		return Capture{}, err
	}
	return capture, nil
}

func forward(source, destination net.Conn, onProfilerData func([]byte) error) error {
	for {
		message, err := unityprofiler.ReadMessage(source)
		if err != nil {
			return err
		}
		if onProfilerData != nil && message.ID == unityprofiler.MsgIDProfilerData {
			if err := onProfilerData(message.Body); err != nil {
				return err
			}
		}
		if err := unityprofiler.WriteMessage(destination, message.ID, message.Body); err != nil {
			return err
		}
	}
}

func relayError(errs ...error) error {
	for _, err := range errs {
		if err == nil || errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, net.ErrClosed) {
			continue
		}
		var opErr *net.OpError
		if errors.As(err, &opErr) && opErr.Err != nil && errors.Is(opErr.Err, net.ErrClosed) {
			continue
		}
		return err
	}
	return nil
}

// WriteFile persists a PD3U capture. Its simple length-prefixed container is
// specific to this tool and intentionally never uses the .data extension.
func WriteFile(path string, capture Capture) error {
	if captureSize(capture.Bodies) > maxCaptureSize {
		return ErrCaptureTooLarge
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if err := writeCapture(file, capture); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// ReadFile loads a capture written by WriteFile.
func ReadFile(path string) (Capture, error) {
	file, err := os.Open(path)
	if err != nil {
		return Capture{}, err
	}
	defer file.Close()
	return readCapture(file)
}

func writeCapture(w io.Writer, capture Capture) error {
	var header [captureHeader]byte
	binary.LittleEndian.PutUint32(header[0:4], captureMagic)
	binary.LittleEndian.PutUint16(header[4:6], captureVersion)
	if err := writeAll(w, header[:]); err != nil {
		return err
	}
	for _, body := range capture.Bodies {
		if len(body) > unityprofiler.MaxPayloadSize {
			return unityprofiler.ErrPayloadTooLarge
		}
		var length [4]byte
		binary.LittleEndian.PutUint32(length[:], uint32(len(body)))
		if err := writeAll(w, length[:]); err != nil {
			return err
		}
		if err := writeAll(w, body); err != nil {
			return err
		}
	}
	return nil
}

func readCapture(r io.Reader) (Capture, error) {
	var header [captureHeader]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return Capture{}, err
	}
	if binary.LittleEndian.Uint32(header[0:4]) != captureMagic || binary.LittleEndian.Uint16(header[4:6]) != captureVersion || binary.LittleEndian.Uint16(header[6:8]) != 0 {
		return Capture{}, ErrCaptureFormat
	}

	var capture Capture
	var total int
	for {
		var length [4]byte
		if _, err := io.ReadFull(r, length[:]); err != nil {
			if errors.Is(err, io.EOF) {
				return capture, nil
			}
			return Capture{}, err
		}
		n := int(binary.LittleEndian.Uint32(length[:]))
		if n > unityprofiler.MaxPayloadSize {
			return Capture{}, unityprofiler.ErrPayloadTooLarge
		}
		if total+n > maxCaptureSize {
			return Capture{}, ErrCaptureTooLarge
		}
		body := make([]byte, n)
		if _, err := io.ReadFull(r, body); err != nil {
			return Capture{}, err
		}
		capture.Bodies = append(capture.Bodies, body)
		total += n
	}
}

func captureSize(bodies [][]byte) int {
	total := 0
	for _, body := range bodies {
		total += len(body)
	}
	return total
}

func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}
