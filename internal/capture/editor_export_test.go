package capture

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"unityperf/internal/profilerproxy"
	"unityperf/internal/unityprofiler"
)

func TestWriteEditorRawPreservesPD3U(t *testing.T) {
	chunks := unityprofiler.SampleProfilerCapture()
	path := filepath.Join(t.TempDir(), "recording.raw")
	if err := writeEditorRaw(path, chunks); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, bytes.Join(chunks, nil)) {
		t.Fatalf("raw bytes differ: %v", err)
	}
	if err := writeEditorRaw(path, chunks[1:]); err == nil {
		t.Fatal("accepted missing stream header")
	}
	if err := writeEditorRaw(path, append(chunks, chunks[0])); err == nil {
		t.Fatal("accepted multiple stream headers")
	}
}

func TestExportWithUnity2022Editor(t *testing.T) {
	editor := os.Getenv("UNITYPERF_TEST_EDITOR")
	if editor == "" {
		t.Skip("set UNITYPERF_TEST_EDITOR to a Unity 2022 Editor executable")
	}
	recording, err := profilerproxy.ReadFile(filepath.Join("..", "..", "test", "same-session.uppc"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	output := filepath.Join(t.TempDir(), "export.data")
	count, err := ExportWithEditor(ctx, editor, output, recording.Bodies)
	if err != nil {
		t.Fatal(err)
	}
	file, err := LoadFile(output)
	if err != nil || len(file.Frames) < 321 || count < len(file.Frames) {
		t.Fatalf("Editor reports %d frames, decoded %d: %v", count, len(file.Frames), err)
	}
	t.Logf("Editor reopened %d frames and app decoded %d", count, len(file.Frames))
}
