package unityprofiler

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"net"
	"time"
)

func memoryID(value string) (id MessageID) {
	b, err := hex.DecodeString(value)
	if err != nil || len(b) != len(id) {
		panic("invalid memory message ID")
	}
	copy(id[:], b)
	return
}

var (
	memoryRequest = memoryID("b43968ed09a1a42ebb0780e4cd4a7c4f")
	memoryBegin   = memoryID("6c628fc5d1432963e99552f010f51892")
	memoryChunk   = memoryID("deacfd64a296fd1845f74f3fa02535c0")
	memoryEnd     = memoryID("0894b1537c46e57a08deebc7d1381483")
)

const maxMemoryCapture = 512 << 20

// The wire has no request ID. Drain an abandoned transfer through its end marker before retrying.
type memoryTransfer struct {
	started   bool
	abandoned bool
	data      bytes.Buffer
	result    chan error
}

func (s *Session) CaptureMemory(ctx context.Context) ([]byte, error) {
	s.mu.Lock()
	if s.conn == nil {
		s.mu.Unlock()
		return nil, ErrNotConnected
	}
	if s.memory != nil {
		s.mu.Unlock()
		return nil, errors.New("上一次快照仍在传输；如 Player 已停止响应，请重新连接")
	}
	t := &memoryTransfer{result: make(chan error, 1)}
	s.memory = t
	closed := s.done
	_ = s.conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	err := WriteMessage(s.conn, memoryRequest, []byte{3, 0, 0, 0})
	_ = s.conn.SetWriteDeadline(time.Time{})
	if err != nil {
		s.memory = nil
	}
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	select {
	case err := <-t.result:
		if err != nil {
			return nil, err
		}
		return t.data.Bytes(), nil
	case <-closed:
		return nil, ErrNotConnected
	case <-ctx.Done():
		s.mu.Lock()
		t.abandoned = true
		t.data = bytes.Buffer{}
		s.mu.Unlock()
		return nil, ctx.Err()
	}
}
func (s *Session) receiveMemory(conn net.Conn, msg Message) {
	if msg.ID != memoryBegin && msg.ID != memoryChunk && msg.ID != memoryEnd {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.memory
	if s.conn != conn || t == nil {
		return
	}
	switch msg.ID {
	case memoryBegin:
		t.started = true
	case memoryChunk:
		if t.abandoned {
			return
		}
		if !t.started || len(msg.Body) > maxMemoryCapture-t.data.Len() {
			t.abandoned = true
			t.data = bytes.Buffer{}
			t.result <- errors.New("快照分块无效或超过 512 MiB 上限")
			return
		}
		t.data.Write(msg.Body)
	case memoryEnd:
		s.memory = nil
		if !t.abandoned {
			if !t.started || t.data.Len() == 0 {
				t.result <- errors.New("Player 返回空快照")
			} else {
				t.result <- nil
			}
		}
	}
}
