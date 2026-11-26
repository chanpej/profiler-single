package services

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"unityperf/internal/memsnap"
	"unityperf/internal/unityprofiler"
	"unityperf/internal/upac"
)

func TestMemoryLibrarySessions(t *testing.T) {
	p := NewPerfService()
	defer p.ServiceShutdown()
	old := p.memorySession()
	sample := MemorySample{Source: "pss", Target: "game"}
	if _, err := p.rememberMemory(old, "", sample); err != nil {
		t.Fatal(err)
	}
	p.Clear()
	if _, err := p.rememberMemory(old, "", sample); err == nil {
		t.Fatal("stale capture accepted")
	}
	lib, err := p.MemorySnapshotLibrary()
	if err != nil || len(lib.Records) != 0 {
		t.Fatal(lib, err)
	}
}

func TestPortableMemoryCapture(t *testing.T) {
	for _, frames := range []bool{false, true} {
		dir := t.TempDir()
		svc := NewPerfService()
		defer svc.ServiceShutdown()
		data := []byte("test snapshot bytes")
		sample := MemorySample{Source: "snap", Target: "sample.snap", Rows: []memsnap.Row{{Name: "Mesh", Count: 2, Bytes: 42}}}
		if src := os.Getenv("UNITYPERF_PORTABLE_SNAP"); src != "" {
			var e error
			data, e = os.ReadFile(src)
			if e != nil {
				t.Fatal(e)
			}
			sample, e = readMemorySnapshot(src)
			if e != nil {
				t.Fatal(e)
			}
		}
		original := filepath.Join(dir, "sample.snap")
		if e := os.WriteFile(original, data, 0600); e != nil {
			t.Fatal(e)
		}
		if _, e := svc.rememberMemory(svc.memorySession(), original, sample); e != nil {
			t.Fatal(e)
		}
		os.Remove(original)
		if _, e := svc.rememberMemory(svc.memorySession(), "", MemorySample{Source: "pss", Target: "test process", Rows: []memsnap.Row{{Name: "Graphics", Bytes: 123}}}); e != nil {
			t.Fatal(e)
		}
		if frames {
			svc.session.SetRaw(unityprofiler.SampleProfilerCapture())
		}
		path := filepath.Join(dir, "recording.upac")
		if e := svc.ExportCapture(path); e != nil {
			t.Fatal(e)
		}
		replay := NewPerfService()
		defer replay.ServiceShutdown()
		n, e := replay.ImportCapture(path)
		if e != nil || (n > 0) != frames {
			t.Fatal(n, e)
		}
		lib, e := replay.MemorySnapshotLibrary()
		if e != nil || len(lib.Records) != 2 || lib.Records[0].Session != lib.Session {
			t.Fatal(lib, e)
		}
		if e = replay.ExportCapture(path); e != nil {
			t.Fatal(e)
		}
		f, e := upac.LoadFile(path)
		if e != nil || len(f.Memory) != 2 || !bytes.Equal(f.Memory[0].Data, data) || f.Memory[1].Rows[0].Bytes != 123 {
			t.Fatal("memory round trip failed", e)
		}
	}
}
