package unityprofiler

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"
)

func TestMemoryRequestDrainsCancelledStreamBeforeRetry(t *testing.T) {
	client, player := net.Pipe()
	defer client.Close()
	defer player.Close()
	s := &Session{conn: client, done: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	var result = make(chan error, 1)
	go func() { _, err := s.CaptureMemory(ctx); result <- err }()
	_ = player.SetReadDeadline(time.Now().Add(time.Second))
	request, err := ReadMessage(player)
	if err != nil || request.ID != memoryRequest || !bytes.Equal(request.Body, []byte{3, 0, 0, 0}) {
		t.Fatal(request, err)
	}
	cancel()
	if <-result != context.Canceled {
		t.Fatal("missing cancellation")
	}
	if _, err := s.CaptureMemory(context.Background()); err == nil {
		t.Fatal("overlapping transfer")
	}
	s.receiveMemory(client, Message{ID: memoryChunk, Body: []byte("old")})
	s.receiveMemory(client, Message{ID: memoryEnd})
	go func() {
		data, err := s.CaptureMemory(context.Background())
		if err == nil && string(data) != "new data" {
			t.Errorf("data %q", data)
		}
		result <- err
	}()
	if _, err := ReadMessage(player); err != nil {
		t.Fatal(err)
	}
	s.receiveMemory(client, Message{ID: memoryBegin})
	for _, chunk := range []string{"new ", "data"} {
		s.receiveMemory(client, Message{ID: memoryChunk, Body: []byte(chunk)})
	}
	s.receiveMemory(client, Message{ID: memoryEnd})
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}
