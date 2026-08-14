package device

import (
	"testing"
)

func TestHandshakeNotConnected(t *testing.T) {
	client := NewClient()
	if _, err := client.Handshake(); err != ErrNotConnected {
		t.Fatalf("err = %v, want ErrNotConnected", err)
	}
}
