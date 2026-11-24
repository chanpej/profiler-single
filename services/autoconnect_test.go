package services

import (
	"bytes"
	"context"
	"encoding/binary"
	"net"
	"slices"
	"testing"
	"time"

	"github.com/coder/websocket"

	"unityperf/internal/protocol"
	"unityperf/internal/unityprofiler"
)

func TestAutoconnectIPv4ListsUsableAddresses(t *testing.T) {
	addresses := []net.Addr{
		&net.IPAddr{IP: net.ParseIP("192.168.0.42")},
		&net.IPAddr{IP: net.ParseIP("127.0.0.1")},
		&net.IPAddr{IP: net.ParseIP("169.254.0.3")},
		&net.IPAddr{IP: net.ParseIP("fe80::1")},
		&net.IPNet{IP: net.ParseIP("192.168.0.42"), Mask: net.CIDRMask(24, 32)},
		&net.IPNet{IP: net.IPv4(10, 0, 0, 8), Mask: net.CIDRMask(24, 32)},
	}
	want := []string{"10.0.0.8", "192.168.0.42"}
	if got := autoconnectIPv4(addresses); !slices.Equal(got, want) {
		t.Fatalf("addresses = %v, want %v", got, want)
	}
}

func TestDisconnectClosesAutoconnectPortAndAllowsRestart(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	port := listener.Addr().(*net.TCPAddr).Port
	service := newTestService()
	if err := service.startAutoconnect(listener); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Disconnect)

	service.Disconnect()
	if status := service.Status(); status.Connected || status.Reconnecting || status.Transport != "" {
		t.Fatalf("unexpected status after closing listener: %+v", status)
	}
	if conn, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second); err == nil {
		_ = conn.Close()
		t.Fatal("小游戏端口在断开后仍然接受连接")
	}
	if err := service.StartAutoconnect(port); err != nil {
		t.Fatalf("failed to reopen listener on port %d: %v", port, err)
	}
	if status := service.Status(); !status.Reconnecting || status.Transport != autoConnectTransport {
		t.Fatalf("unexpected status after reopening listener: %+v", status)
	}
}

func TestAutoconnectReceivesTuanjieFrames(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	service := newTestService()
	if err := service.startAutoconnect(listener); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Disconnect)
	url := "ws://" + listener.Addr().String()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if status := service.Status(); !status.Reconnecting || status.Transport != autoConnectTransport {
		t.Fatalf("unexpected listener status: %+v", status)
	}
	for range 2 {
		player, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{Subprotocols: []string{"binary"}})
		if err != nil {
			t.Fatal(err)
		}
		defer player.CloseNow()
		write := func(id unityprofiler.MessageID, data []byte) {
			t.Helper()
			var packet bytes.Buffer
			if err := unityprofiler.WriteMessage(&packet, id, data); err != nil {
				t.Fatal(err)
			}
			for _, part := range [][]byte{packet.Bytes()[:unityprofiler.ConnHeaderSize], packet.Bytes()[unityprofiler.ConnHeaderSize:]} {
				if err := player.Write(ctx, websocket.MessageBinary, part); err != nil {
					t.Fatal(err)
				}
			}
		}
		write(unityprofiler.MsgIDAnnounce, []byte("[Port] 55000 [Flags] 2"))
		write(unityprofiler.MsgIDPlayerInfo, unityprofiler.EncodePlayerEngineInfo(50, 2022, 3, 62, 4, 13))
		for _, id := range []unityprofiler.MessageID{unityprofiler.MsgIDProfilerSetup, unityprofiler.MsgIDProfilerAreas, unityprofiler.MsgIDProfilerCategories, unityprofiler.MsgIDAnnounce} {
			kind, data, err := player.Read(ctx)
			if err != nil || kind != websocket.MessageBinary {
				t.Fatalf("invalid profiler command: %v, %v", kind, err)
			}
			message, err := unityprofiler.ReadMessage(bytes.NewReader(data))
			if err != nil || message.ID != id {
				t.Fatalf("unexpected profiler message: %v, %v", message.ID, err)
			}
		}
		previous := service.FrameCount()
		cleanLogID := unityprofiler.MessageID{0xd3, 0xde, 0xd2, 0xad, 0xdc, 0x2f, 0x64, 0x8d, 0x3a, 0x6f, 0x10, 0x47, 0x71, 0x14, 0x7e, 0x9a}
		fullLogID := unityprofiler.MessageID{0x93, 0xa4, 0xad, 0x30, 0xb8, 0x0a, 0xf4, 0x62, 0x0b, 0x10, 0xa1, 0xc6, 0xed, 0x0b, 0xa5, 0x26}
		beforeLog := service.PlayerLogs(0)
		previousLogID := int64(0)
		if len(beforeLog) > 0 {
			previousLogID = beforeLog[len(beforeLog)-1].ID
		}
		write(cleanLogID, append([]byte{2, 0, 0, 0}, []byte("握手日志\x00")...))
		header := unityprofiler.EncodeStreamHeader(unityprofiler.StreamHeader{FormatVersion: protocol.Tuanjie2022StreamVersion}, 2022, 3, 62, 4, 13)
		write(unityprofiler.MsgIDProfilerData, header)
		start := make([]byte, 0, 100)
		for _, word := range []uint32{0x22, 1, 100_000_000, 0, 0x24, 0, 22, 100_000_000, 0, 0x25, 0, 22, 115_000_000, 0, 0x22, 2, 116_000_000, 0} {
			start = binary.LittleEndian.AppendUint32(start, word)
		}
		write(unityprofiler.MsgIDProfilerData, unityprofiler.EncodeChunk(0, 0, start[:32]))
		write(fullLogID, append([]byte{4, 0, 0, 0}, []byte("异常\nPlayer.Update()\x00")...))
		forwarderID := unityprofiler.MessageID{0xac, 0xdf, 0x7d, 0x10, 0x18, 0xb9, 0x4f, 0x2f, 0xba, 0xb2, 0x5f, 0xd7, 0x3e, 0xb8, 0xcf, 0xa1}
		write(forwarderID, []byte("UnityPerf.Log/1\x00\x02小游戏转发日志"))
		write(forwarderID, []byte("UnityPerfAgent.Log/1\x00\x04旧版转发日志"))
		write(unityprofiler.MsgIDProfilerData, unityprofiler.EncodeChunk(1, 0, start[32:]))
		waitAutoconnect(t, ctx, func() bool { return service.FrameCount() == previous+1 && service.Status().Connected })
		logs := service.PlayerLogs(previousLogID)
		if len(logs) != 4 || logs[0].Level != "Warning" || logs[0].Text != "握手日志" || logs[1].Level != "Exception" || logs[1].Text != "异常\nPlayer.Update()" || logs[2].Level != "Warning" || logs[2].Text != "小游戏转发日志" {
			t.Fatalf("unexpected Player logs: %+v", logs)
		}
		if logs[3].Level != "Exception" || logs[3].Text != "旧版转发日志" {
			t.Fatalf("unexpected legacy forwarded log: %+v", logs[3])
		}
		status := service.Status()
		if status.Device.UnityVersion != "2022.3.62t13" || !status.UnitySupported || status.Device.ProfilerStreamVersion != protocol.Tuanjie2022StreamVersion {
			t.Fatalf("unexpected device status: %+v", status)
		}
		if got := service.FrameAt(previous).CPUTimeMs; got < 15 || got > 16 {
			t.Fatalf("unexpected frame time: %.2f", got)
		}
		write(unityprofiler.MsgIDProfilerData, unityprofiler.EncodeChunk(1028, 0, []byte{0xff, 0xff, 0xff, 0xff}))
		resumed := make([]byte, 0, 32)
		for _, word := range []uint32{0x22, 9, 360_000_000, 0, 0x22, 10, 380_000_000, 0} {
			resumed = binary.LittleEndian.AppendUint32(resumed, word)
		}
		write(unityprofiler.MsgIDProfilerData, unityprofiler.EncodeChunk(1029, 0, append([]byte{0xff, 0xff, 0xff, 0xff}, resumed[:16]...)))
		write(unityprofiler.MsgIDProfilerData, unityprofiler.EncodeChunk(1030, 0, resumed[:16]))
		write(unityprofiler.MsgIDProfilerData, unityprofiler.EncodeChunk(1031, 0, resumed[16:]))
		waitAutoconnect(t, ctx, func() bool { return service.FrameCount() == previous+2 && service.Status().Connected })
		if got := service.FrameAt(previous + 1); got.FrameIndex != 9 || got.CPUTimeMs != 20 {
			t.Fatalf("unexpected recovered frame: %+v", got)
		}
		if stats := service.client.DecodeStats(); stats.MissingMainChunks != 1026 || stats.DiscardedMainChunks != 2 || stats.FrameResyncs != 1 || stats.WaitingForFrame || stats.Desynced {
			t.Fatalf("unexpected recovery stats: %+v", stats)
		}
		_ = player.CloseNow()
		waitAutoconnect(t, ctx, func() bool { return service.Status().Reconnecting })
	}
}

func waitAutoconnect(t *testing.T, ctx context.Context, ready func() bool) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for !ready() {
		select {
		case <-ctx.Done():
			t.Fatal("autoconnect condition did not become ready")
		case <-ticker.C:
		}
	}
}
