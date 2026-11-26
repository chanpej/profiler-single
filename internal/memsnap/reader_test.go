package memsnap

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeTypes(t *testing.T) {
	// Minimal queried snapshot with two types and three objects across several chunks.
	var buf []byte
	appendVal := func(v any) uint64 {
		p := uint64(len(buf))
		var err error
		buf, err = binary.Append(buf, le, v)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	appendVal(uint32(0xaeabcdcd))
	var blocks []uint64
	entries := make([]uint64, 14)
	add := func(id, typ, width uint32, count uint64, data []byte, offsets []uint64) {
		var chunks []uint64
		for i := 0; i < len(data); i += 3 {
			chunks = append(chunks, appendVal(data[i:min(i+3, len(data))]))
		}
		blocks = append(blocks, appendVal(uint64(3)))
		appendVal(uint64(len(data)))
		appendVal(chunks)
		entries[id] = appendVal(uint16(typ))
		appendVal(uint32(len(blocks) - 1))
		appendVal(width)
		appendVal(count)
		appendVal(offsets)
	}
	add(5, 3, 2, 0, []byte("MeshTexture"), []uint64{4, 11})
	types, _ := binary.Append(nil, le, []uint32{0, 1, 0})
	sizes, _ := binary.Append(nil, le, []uint64{5, 20, 4})
	add(7, 2, 4, 3, types, nil)
	add(13, 2, 8, 3, sizes, nil)
	bp := appendVal(uint32(0x20170724))
	appendVal(uint32(len(blocks)))
	appendVal(blocks)
	d := appendVal(uint32(0xcdcdaeab))
	appendVal(uint32(0x20170724))
	appendVal(bp)
	appendVal(uint32(len(entries)))
	appendVal(entries)
	appendVal(d)
	appendVal(uint32(0xabcdcdae))
	path := filepath.Join(t.TempDir(), "test.snap")
	if err := os.WriteFile(path, buf, 0600); err != nil {
		t.Fatal(err)
	}
	rows, err := NativeTypes(path)
	if err != nil || len(rows) != 2 || rows[1] != (Row{"Mesh", 9, 2}) || rows[0] != (Row{"Texture", 20, 1}) {
		t.Fatal(rows, err)
	}
	buf[len(buf)-1] = 0
	os.WriteFile(path, buf, 0600)
	if _, err = NativeTypes(path); err == nil {
		t.Fatal("accepted damaged footer")
	}
}
func TestUnityGeneratedSnapshot(t *testing.T) {
	path := os.Getenv("UNITYPERF_TEST_SNAP")
	if path == "" {
		t.Skip("external Unity snapshot")
	}
	rows, err := NativeTypes(path)
	if err != nil || len(rows) == 0 {
		t.Fatal(err)
	}
	t.Logf("%d types; largest %+v", len(rows), rows[0])
}
