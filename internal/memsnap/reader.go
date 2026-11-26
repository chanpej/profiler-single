// Package memsnap reads the native-object columns of Unity 2022 queried snapshots.
package memsnap

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sort"
)

var le = binary.LittleEndian
var invalid = errors.New("Unity .snap 目录、分块或对象表无效")

const maxColumn = 128 << 20

type Row struct {
	Name  string `json:"name"`
	Bytes uint64 `json:"bytes"`
	Count int    `json:"count"`
}
type reader struct {
	file            *os.File
	size            uint64
	entries, blocks []byte
}

func (r *reader) read(off, n uint64) ([]byte, error) {
	if off > r.size || n > r.size-off || n > maxColumn {
		return nil, invalid
	}
	b := make([]byte, int(n))
	_, err := r.file.ReadAt(b, int64(off))
	return b, err
}
func (r *reader) table(off uint64) ([]byte, error) {
	b, e := r.read(off, 4)
	if e != nil {
		return nil, e
	}
	n := le.Uint32(b)
	if n > 65536 {
		return nil, invalid
	}
	return r.read(off+4, uint64(n)*8)
}
func pointer(table []byte, id uint32) (uint64, error) {
	if uint64(id) >= uint64(len(table)/8) {
		return 0, invalid
	}
	v := le.Uint64(table[int(id)*8:])
	if v == 0 {
		return 0, invalid
	}
	return v, nil
}
func (r *reader) directory() error {
	if r.size < 32 {
		return invalid
	}
	h, e := r.read(0, 4)
	if e != nil || le.Uint32(h) != 0xaeabcdcd {
		return invalid
	}
	tail, e := r.read(r.size-12, 12)
	if e != nil || le.Uint32(tail[8:]) != 0xabcdcdae {
		return invalid
	}
	d := le.Uint64(tail)
	b, e := r.read(d, 16)
	if e != nil || le.Uint32(b) != 0xcdcdaeab || le.Uint32(b[4:]) != 0x20170724 {
		return invalid
	}
	p := le.Uint64(b[8:])
	v, e := r.read(p, 4)
	if e != nil || le.Uint32(v) != 0x20170724 {
		return invalid
	}
	r.blocks, e = r.table(p + 4)
	if e != nil {
		return e
	}
	r.entries, e = r.table(d + 16)
	return e
}
func (r *reader) block(id uint32, start, n uint64) ([]byte, error) {
	address, e := pointer(r.blocks, id)
	if e != nil {
		return nil, e
	}
	h, e := r.read(address, 16)
	if e != nil {
		return nil, e
	}
	chunk, total := le.Uint64(h), le.Uint64(h[8:])
	if chunk == 0 || start > total || n > total-start || n > maxColumn || total > r.size {
		return nil, invalid
	}
	result := make([]byte, int(n))
	for pos := uint64(0); pos < n; {
		at := start + pos
		index := at / chunk
		if index > r.size/8 {
			return nil, invalid
		}
		offset, e := r.read(address+16+index*8, 8)
		if e != nil {
			return nil, e
		}
		fileOff := le.Uint64(offset)
		local := at % chunk
		take := min(n-pos, chunk-local)
		if fileOff > r.size || local > r.size-fileOff || take > r.size-fileOff-local {
			return nil, invalid
		}
		if _, e = r.file.ReadAt(result[pos:pos+take], int64(fileOff+local)); e != nil {
			return nil, e
		}
		pos += take
	}
	return result, nil
}
func (r *reader) header(id uint32) ([]byte, uint64, error) {
	p, e := pointer(r.entries, id)
	if e != nil {
		return nil, 0, e
	}
	b, e := r.read(p, 18)
	return b, p, e
}
func (r *reader) fixed(id, width uint32) ([]byte, error) {
	h, _, e := r.header(id)
	if e != nil {
		return nil, e
	}
	// Constant arrays store a 32-bit count. The following chapter may start at byte 14.
	count := uint64(le.Uint32(h[10:]))
	if le.Uint16(h) != 2 || le.Uint32(h[6:]) != width || count > maxColumn/uint64(width) {
		return nil, invalid
	}
	return r.block(le.Uint32(h[2:]), 0, count*uint64(width))
}
func (r *reader) names() ([]string, error) {
	h, p, e := r.header(5)
	if e != nil {
		return nil, e
	}
	n := le.Uint32(h[6:])
	if le.Uint16(h) != 3 || n > 65536 {
		return nil, invalid
	}
	offsets, e := r.read(p+10, (uint64(n)+1)*8)
	if e != nil {
		return nil, e
	}
	start := le.Uint64(offsets)
	end := le.Uint64(offsets[n*8:])
	if end < start {
		return nil, invalid
	}
	data, e := r.block(le.Uint32(h[2:]), start, end-start)
	if e != nil {
		return nil, e
	}
	names := make([]string, n)
	cursor := start
	for i := range names {
		next := le.Uint64(offsets[(i+1)*8:])
		if next < cursor || next > end {
			return nil, invalid
		}
		names[i] = string(data[cursor-start : next-start])
		cursor = next
	}
	return names, nil
}

func NativeTypes(path string) ([]Row, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return nil, e
	}
	r := reader{file: f, size: uint64(st.Size())}
	if e = r.directory(); e != nil {
		return nil, e
	}
	names, e := r.names()
	if e != nil {
		return nil, fmt.Errorf("读取 NativeTypes：%w", e)
	}
	types, e := r.fixed(7, 4)
	if e != nil {
		return nil, e
	}
	sizes, e := r.fixed(13, 8)
	if e != nil || len(types)/4 != len(sizes)/8 {
		return nil, invalid
	}
	rows := make([]Row, len(names))
	for i, name := range names {
		rows[i].Name = name
	}
	for i := 0; i < len(types)/4; i++ {
		idx := le.Uint32(types[i*4:])
		if int64(idx) >= int64(len(rows)) {
			return nil, invalid
		}
		b := le.Uint64(sizes[i*8:])
		if ^uint64(0)-rows[idx].Bytes < b {
			return nil, invalid
		}
		rows[idx].Bytes += b
		rows[idx].Count++
	}
	result := make([]Row, 0, len(rows))
	for _, row := range rows {
		if row.Count > 0 {
			result = append(result, row)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Bytes == result[j].Bytes {
			return result[i].Name < result[j].Name
		}
		return result[i].Bytes > result[j].Bytes
	})
	return result, nil
}
