package memsnap

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPairedCapture(t *testing.T) {
	raw := os.Getenv("UNITYPERF_PAIRED_RAW")
	if raw == "" {
		t.Skip("requires paired Editor capture")
	}
	data, err := os.ReadFile(raw)
	if err != nil {
		t.Fatal(err)
	}
	dest := os.Getenv("UNITYPERF_CONVERT_OUT")
	if dest == "" {
		dest = filepath.Join(t.TempDir(), "result.snap")
	}
	f, err := os.Create(dest)
	if err != nil {
		t.Fatal(err)
	}
	err = ConvertPlayerSnapshot(data, f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	got, err := NativeTypes(dest)
	if err != nil {
		t.Fatal(err)
	}
	want, err := NativeTypes(os.Getenv("UNITYPERF_PAIRED_SNAP"))
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("native object columns differ", err)
	}
	t.Logf("matched %d native types", len(got))
}
func TestMalformedCapture(t *testing.T) {
	for _, b := range [][]byte{nil, {1, 0xed, 0xbc, 0xfa}, {1, 0xed, 0xbc, 0xfa, 16, 0, 0, 0}} {
		if ConvertPlayerSnapshot(b, io.Discard) == nil {
			t.Fatal("accepted incomplete snapshot")
		}
	}
}
