package memsnap

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
)

// The WeChat 2022.3.62t13 Player serializes records; the Editor saves columnar .snap files.
// Only accept the observed v16 layout and NativeObjects|ManagedObjects capture flags.
type wireCursor struct {
	data []byte
	at   int
}
type wireError struct{ error }

func (c *wireCursor) take(n int) []byte {
	if n < 0 || n > len(c.data)-c.at {
		panic(wireError{io.ErrUnexpectedEOF})
	}
	b := c.data[c.at : c.at+n]
	c.at += n
	return b
}
func (c *wireCursor) u32() uint32  { return le.Uint32(c.take(4)) }
func (c *wireCursor) blob() []byte { return c.take(int(c.u32())) }
func (c *wireCursor) sized() []byte {
	n := le.Uint64(c.take(8))
	if n > uint64(len(c.data)-c.at) {
		panic(wireError{io.ErrUnexpectedEOF})
	}
	return c.take(int(n))
}
func (c *wireCursor) marker(want uint32) {
	off := c.at
	if got := c.u32(); got != want {
		panic(wireError{fmt.Errorf("快照偏移 %d：段标记 %08x，期望 %08x", off, got, want)})
	}
}

type column struct {
	format uint16
	width  uint32
	count  uint32
	data   []byte
	ends   []uint64
}
type columns [88]column

func (a *columns) add(id int, fmtID uint16, b []byte) {
	col := &a[id]
	col.format = fmtID
	col.count++
	col.data = append(col.data, b...)
	if fmtID == 3 {
		col.ends = append(col.ends, uint64(len(col.data)))
	} else {
		col.width = uint32(len(b))
	}
}
func (a *columns) number(id int, n uint32) { a.add(id, 2, le.AppendUint32(nil, n)) }
func (c *wireCursor) heaps(a *columns, tag uint32, addrID int) {
	c.marker(tag)
	for range c.u32() {
		a.add(addrID, 2, c.take(8))
		a.add(addrID+1, 3, c.blob())
	}
}
func (c *wireCursor) managed(a *columns) {
	c.marker(0x4891aefd)
	var field uint32
	for i := range c.u32() {
		flags := c.u32()
		a.number(22, flags)
		a.add(27, 2, c.take(4))
		var indices, statics []byte
		if flags&2 == 0 {
			for range c.u32() {
				a.add(31, 2, c.take(4))
				a.add(32, 2, c.take(4))
				a.add(33, 3, c.blob())
				a.add(34, 2, c.take(1))
				indices = le.AppendUint32(indices, field)
				field++
			}
			statics = c.blob()
		}
		a.add(25, 3, indices)
		a.add(26, 3, statics)
		a.add(23, 3, c.blob())
		a.add(24, 3, c.blob())
		a.add(29, 2, c.take(8))
		a.add(28, 2, c.take(4))
		a.number(30, i)
	}
	c.marker(0x3456132c)
	for range c.u32() {
		a.add(15, 2, c.take(8))
	}
}
func (c *wireCursor) native(a *columns) {
	c.marker(0x78514753)
	for i := range c.u32() {
		if c.u32() != i {
			panic(wireError{errors.New("原生类型序号不连续")})
		}
		a.add(6, 2, c.take(4))
		a.add(5, 3, c.blob())
	}
	c.marker(0x6173fafe)
	for range c.u32() {
		a.add(11, 3, c.blob())
		a.add(10, 2, c.take(4))
		a.add(13, 2, c.take(8))
		a.add(7, 2, c.take(4))
		a.add(8, 2, c.take(4))
		a.add(9, 2, c.take(4))
		a.add(12, 2, c.take(8))
		a.add(14, 2, c.take(8))
		a.add(58, 2, c.take(4))
		for range c.u32() {
			a.add(16, 2, c.take(4))
			a.add(17, 2, c.take(4))
		}
		meta := c.u32()
		a.number(81, meta)
		if meta != 0xffffffff {
			a.add(82, 3, c.blob())
		}
	}
}
func (c *wireCursor) scenes(a *columns) {
	c.marker(0x2913beef)
	for range c.u32() {
		a.add(62, 3, c.blob())
		a.add(65, 2, c.take(4))
		a.add(63, 3, c.blob())
		a.add(64, 3, c.blob())
		a.add(67, 2, c.take(4))
		n := c.u32()
		a.number(66, n)
		for range n {
			a.add(68, 2, c.take(4))
		}
	}
}
func ConvertPlayerSnapshot(raw []byte, output io.Writer) (err error) {
	defer func() {
		if cause := recover(); cause != nil {
			if e, ok := cause.(wireError); ok {
				err = e.error
			} else {
				panic(cause)
			}
		}
	}()
	c := wireCursor{data: raw}
	a := new(columns)
	c.marker(0xfabced01)
	c.marker(16)
	a.add(0, 1, le.AppendUint32(nil, 16))
	a.add(1, 1, c.take(8))
	a.add(2, 1, c.sized())
	c.marker(3)
	a.add(3, 1, le.AppendUint32(nil, 3))
	c.heaps(a, 0x9111daaa, 18)
	c.heaps(a, 0x147358aa, 20)
	c.managed(a)
	c.native(a)
	c.scenes(a)
	c.marker(0x0183efac)
	a.add(4, 1, c.take(24))
	a.add(59, 1, c.sized())
	a.add(60, 1, c.sized())
	c.marker(0x22030901)
	c.marker(0)
	c.marker(0x22030902)
	c.marker(0x865eeaaf)
	if c.at != len(raw) {
		return errors.New("快照结尾包含未识别数据")
	}
	return a.write(output)
}

// Store each column in one contiguous data block. Dynamic entries carry logical offsets.
func (a *columns) write(output io.Writer) error {
	w := bufio.NewWriter(output)
	offset := uint64(4)
	if _, err := w.Write(le.AppendUint32(nil, 0xaeabcdcd)); err != nil {
		return err
	}
	var blocks, entries [88]uint64
	for i, col := range a {
		if col.format == 0 {
			continue
		}
		start := offset
		n, err := w.Write(col.data)
		if err != nil {
			return err
		}
		offset += uint64(n)
		blocks[i] = offset
		header := le.AppendUint64(nil, max(1, uint64(n)))
		header = le.AppendUint64(header, uint64(n))
		if n > 0 {
			header = le.AppendUint64(header, start)
		}
		if _, err = w.Write(header); err != nil {
			return err
		}
		offset += uint64(len(header))
	}
	var tail bytes.Buffer
	u32 := func(v uint32) { tail.Write(le.AppendUint32(nil, v)) }
	u64 := func(v uint64) { tail.Write(le.AppendUint64(nil, v)) }
	address := func() uint64 { return offset + uint64(tail.Len()) }
	blockDir := address()
	u32(0x20170724)
	var count uint32
	for _, pos := range blocks {
		if pos != 0 {
			count++
		}
	}
	u32(count)
	for _, pos := range blocks {
		if pos != 0 {
			u64(pos)
		}
	}
	var blockIndex uint32
	for i, col := range a {
		if col.format == 0 {
			continue
		}
		entries[i] = address()
		tail.Write(le.AppendUint16(nil, col.format))
		u32(blockIndex)
		blockIndex++
		switch col.format {
		case 1:
			u32(col.width)
			u64(0)
		case 2:
			u32(col.width)
			u64(uint64(col.count))
		case 3:
			u32(col.count)
			u64(0)
			for _, end := range col.ends {
				u64(end)
			}
		}
	}
	directory := address()
	u32(0xcdcdaeab)
	u32(0x20170724)
	u64(blockDir)
	u32(uint32(len(entries)))
	for _, pos := range entries {
		u64(pos)
	}
	u64(directory)
	u32(0xabcdcdae)
	if _, err := tail.WriteTo(w); err != nil {
		return err
	}
	return w.Flush()
}
