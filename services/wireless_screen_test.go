package services

import (
	"bytes"
	"context"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unityperf/internal/unityprofiler"
	"unityperf/internal/upac"
)

func TestWirelessScreenAddress(t *testing.T) {
	for _, s := range []string{"", "-d", "phone:5555", "0.0.0.0", "10.1.1.1:0"} {
		if _, err := wirelessEndpoint(s); err == nil {
			t.Fatalf("accepted %q", s)
		}
	}
	endpoint, err := wirelessEndpoint(" 10.1.1.1 ")
	if err != nil || endpoint != "10.1.1.1:5555" {
		t.Fatal(endpoint, err)
	}
	commands := []string{}
	run := func(_ context.Context, args ...string) (string, error) {
		commands = append(commands, strings.Join(args, " "))
		if len(commands) == 1 {
			return "failed to connect", nil
		}
		return "offline", nil
	}
	if connectScreenDevice(context.Background(), run, endpoint) == nil || len(commands) != 2 || commands[1] != "-s 10.1.1.1:5555 get-state" {
		t.Fatal(commands)
	}
}

// Opt-in: use the connected phone for the image and a local Player for repeatable frame timing.
func TestWirelessScreenPhoneArchive(t *testing.T) {
	if os.Getenv("UNITYPERF_TEST_WIFI_SCREEN") == "" {
		t.Skip("requires an authorized USB phone on Wi-Fi")
	}
	svc := NewPerfService()
	defer svc.Disconnect()
	endpoint, err := svc.EnableWirelessScreenADB()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Second)
	player := unityprofiler.NewPlayer()
	player.Interval = 18 * time.Millisecond
	port, err := player.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer player.Stop()
	if _, err = svc.Connect("127.0.0.1", port); err != nil {
		t.Fatal(err)
	}
	if err = svc.StartWirelessScreenCapture(endpoint); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		strip := svc.ScreenFilmstrip(0, svc.FrameCount(), -1)
		if strip.Error != "" {
			t.Fatal(strip.Error)
		}
		if len(strip.Thumbnails) >= 2 {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if err = svc.SetScreenCapture(false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "wireless.upac")
	if err = svc.ExportCapture(path); err != nil {
		t.Fatal(err)
	}
	file, err := upac.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Screen.Shots) < 2 {
		t.Fatalf("only %d images", len(file.Screen.Shots))
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(file.Screen.Shots[0].JPEG))
	if err != nil {
		t.Fatal(err)
	}
	replay := NewPerfService()
	n, err := replay.ImportCapture(path)
	if err != nil {
		t.Fatal(err)
	}
	strip := replay.ScreenFilmstrip(0, n, -1)
	if len(strip.Thumbnails) < 2 {
		t.Fatal("missing aligned thumbnails after reopen")
	}
	t.Logf("wireless %s: %d images %dx%d, %d frames, %d aligned thumbnails", endpoint, len(file.Screen.Shots), config.Width, config.Height, n, len(strip.Thumbnails))
}
