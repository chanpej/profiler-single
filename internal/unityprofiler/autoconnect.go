package unityprofiler

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

type AutoconnectListener struct {
	listener net.Listener
	server   *http.Server
	cancel   context.CancelFunc
	active   atomic.Bool
	mu       sync.Mutex
	closed   bool
	handlers sync.WaitGroup
}

func ServeAutoconnect(listener net.Listener, handle func(net.Conn)) *AutoconnectListener {
	ctx, cancel := context.WithCancel(context.Background())
	receiver := &AutoconnectListener{listener: listener, cancel: cancel}
	receiver.server = &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			if !offersBinary(request.Header.Values("Sec-WebSocket-Protocol")) {
				http.Error(w, "binary subprotocol required", http.StatusBadRequest)
				return
			}
			receiver.mu.Lock()
			if receiver.closed || !receiver.active.CompareAndSwap(false, true) {
				receiver.mu.Unlock()
				http.Error(w, "Player slot unavailable", http.StatusConflict)
				return
			}
			receiver.handlers.Add(1)
			receiver.mu.Unlock()
			defer receiver.handlers.Done()
			defer receiver.active.Store(false)
			socket, err := websocket.Accept(w, request, &websocket.AcceptOptions{
				Subprotocols:   []string{"binary"},
				OriginPatterns: []string{"servicewechat.com", "*.servicewechat.com"},
			})
			if err != nil {
				return
			}
			defer socket.CloseNow()
			conn := websocket.NetConn(ctx, socket, websocket.MessageBinary)
			socket.SetReadLimit(ConnHeaderSize + MaxPayloadSize)
			handle(conn)
		}),
	}
	go func() { _ = receiver.server.Serve(listener) }()
	return receiver
}

func offersBinary(values []string) bool {
	for _, value := range values {
		for protocol := range strings.SplitSeq(value, ",") {
			if strings.TrimSpace(protocol) == "binary" {
				return true
			}
		}
	}
	return false
}

func (receiver *AutoconnectListener) Close() {
	receiver.mu.Lock()
	receiver.closed = true
	receiver.mu.Unlock()
	receiver.cancel()
	_ = receiver.server.Close()
	_ = receiver.listener.Close()
	receiver.handlers.Wait()
}
