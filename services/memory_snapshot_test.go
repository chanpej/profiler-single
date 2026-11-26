package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAndroidPSS(t *testing.T) {
	rows, err := parsePSS(" Native Heap    1000\n App Summary\n   Pss(KB)  Rss(KB)\n  Native Heap:     32    900\n  Graphics: 0\n TOTAL PSS: 32")
	if err != nil || len(rows) != 2 || rows[0].Bytes != 32768 || rows[1].Bytes != 0 {
		t.Fatal(rows, err)
	}
	if _, err = parsePSS("No process found"); err == nil {
		t.Fatal("missing process accepted")
	}
}
func TestForegroundMemoryOnPhone(t *testing.T) {
	if os.Getenv("UNITYPERF_TEST_PSS") == "" {
		t.Skip("requires Android device")
	}
	svc := NewPerfService()
	defer svc.ServiceShutdown()
	sample, err := svc.CaptureAndroidMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s %+v", sample.Target, sample.Rows)
}
func TestUnityCaptureOnPhone(t *testing.T) {
	dest := os.Getenv("UNITYPERF_TEST_CAPTURE_SNAP")
	if dest == "" {
		t.Skip("requires development probe")
	}
	svc := NewPerfService()
	defer svc.ServiceShutdown()
	sample, err := svc.CaptureUnityMemory()
	if err != nil {
		t.Fatal(err)
	}
	if len(sample.Rows) == 0 {
		t.Fatal("empty snapshot")
	}
	t.Logf("%s: %d types, largest %+v", sample.Target, len(sample.Rows), sample.Rows[0])
}

func TestWaitEditorSnapshotIgnoresExistingAndPartialFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "partial.snap"), make([]byte, 128), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := waitEditorSnapshot(ctx, dir, time.Millisecond)
	if err == nil || !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

func TestStandaloneWechatCapture(t *testing.T) {
	dest := os.Getenv("UNITYPERF_MINIGAME_SNAPSHOT")
	if dest == "" {
		t.Skip("requires a connected Autoconnected Player")
	}
	svc := NewPerfService()
	defer svc.ServiceShutdown()
	if err := svc.StartAutoconnect(37100); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(90 * time.Second)
	for !svc.connected() && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	sample, err := svc.CaptureMiniGameMemory()
	if err != nil {
		t.Fatal(err)
	}
	if len(sample.Rows) == 0 {
		t.Fatal("missing native objects")
	}
	t.Logf("real mini game over 37100: %d types, largest %+v", len(sample.Rows), sample.Rows[0])
}
