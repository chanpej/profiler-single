package unityprofiler

import (
	"net"
	"strconv"
	"testing"
	"time"

	"unityperf/internal/profileframe"
)

func TestConnectDirectFindsPlayerAfterNonPlayerPort(t *testing.T) {
	player := NewPlayer()
	port, err := player.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(player.Stop)
	unrelated, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port-1)))
	if err != nil {
		t.Skipf("adjacent port unavailable: %v", err)
	}
	t.Cleanup(func() { _ = unrelated.Close() })
	go func() {
		conn, err := unrelated.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()

	session := NewSession()
	t.Cleanup(session.Close)
	frames := make(chan profileframe.Frame, 1)
	handshake, selected, err := session.connectDirectRange("127.0.0.1", port-1, port, func(frame profileframe.Frame) {
		select {
		case frames <- frame:
		default:
		}
	}, nil)
	if err != nil || selected != port || handshake.UnityVersion != "2022.3.47f1" {
		t.Fatalf("direct connection: port=%d, handshake=%+v, err=%v", selected, handshake, err)
	}
	select {
	case <-frames:
	case <-time.After(3 * time.Second):
		t.Fatal("no profiler frames after direct connection")
	}
}
