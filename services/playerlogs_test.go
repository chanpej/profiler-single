package services

import (
	"context"
	"net"
	"strconv"
	"testing"
)

func TestDefaultAutoconnectPortBusyAndUSBRetry(t *testing.T) {
	socket, err := net.Listen("tcp4", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	port := socket.Addr().(*net.TCPAddr).Port
	service := newTestService()
	t.Cleanup(func() { _ = service.ServiceShutdown() })
	service.startDefaultAutoconnect(port)
	if status := service.Status(); status.LastError == "" || status.Reconnecting {
		t.Fatalf("occupied port should be reported without entering wait state: %+v", status)
	}
	_ = socket.Close()
	service.startDefaultAutoconnect(port)
	if status := service.Status(); !status.Reconnecting || status.LastError != "" {
		t.Fatalf("startup should listen and clear the previous error: %+v", status)
	}
	called := false
	service.adbRun = func(context.Context, ...string) (string, error) {
		called = true
		return "List of devices attached\n", nil
	}
	if _, err := service.ConnectUSB("", "com.company.game"); err == nil || !called {
		t.Fatalf("USB should attempt to connect while waiting for a Mini Game: %v", err)
	}
	if status := service.Status(); !status.Reconnecting || status.Transport != autoConnectTransport {
		t.Fatalf("failed USB attempt should restore autoconnect: %+v", status)
	}
	service.Disconnect()
	socketAfterDisconnect, err := net.Listen("tcp4", ":"+strconv.Itoa(port))
	if err != nil {
		t.Fatalf("listener remains open after disconnect: %v", err)
	}
	_ = socketAfterDisconnect.Close()
}

func TestPlayerLogsRetainLatestEntriesAndUseCursor(t *testing.T) {
	service := newTestService()
	t.Cleanup(func() { _ = service.ServiceShutdown() })
	service.mu.Lock()
	service.transport = "adb"
	service.mu.Unlock()
	for index := range playerLogCapacity + 2 {
		service.onPlayerLog("Log", strconv.Itoa(index))
	}
	logs := service.PlayerLogs(0)
	if len(logs) != playerLogCapacity || logs[0].ID != 3 || logs[0].Text != "2" {
		t.Fatalf("unexpected log retention: count=%d", len(logs))
	}
	latest := logs[len(logs)-1].ID
	if batch := service.PlayerLogs(latest - 1); len(batch) != 1 || batch[0].ID != latest {
		t.Fatalf("unexpected incremental result: %+v", batch)
	}
}
