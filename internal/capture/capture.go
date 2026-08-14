// Package capture 实现 Profiler 会话的紧凑二进制存档。
//
// 文件布局（小端）：
//
//	0   4  Magic "UPAC"
//	4   2  Version（当前为 1）
//	6   2  Flags（保留）
//	8   2  FrameSize（v2 帧负载字节数，固定 164）
//	10  2  Reserved
//	12  4  FrameCount
//	16  4  MetaLen（握手 JSON 字节数，可为 0）
//	20  N  Meta JSON
//	20+N   FrameCount * FrameSize  连续帧负载（protocol.EncodeFrame）
//
// 单帧 164 字节、无填充、无压缩，十万帧约 16MB，加载为顺序解码。
package capture

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"unityperfagent/internal/protocol"
)

const (
	// Version 是当前存档格式版本。
	Version uint16 = 1
	// headerSize 是固定文件头长度。
	headerSize = 20
	// maxFrames 防止异常文件把内存打爆（与采集环形缓冲同量级）。
	maxFrames = 1 << 20
	// maxMetaLen 限制握手 JSON 大小。
	maxMetaLen = 1 << 20
)

// Magic 是存档文件魔数。
var Magic = [4]byte{'U', 'P', 'A', 'C'}

// ErrBadMagic 表示文件不是 UPAC 存档。
var ErrBadMagic = errors.New("capture: bad magic")

// ErrUnsupportedVersion 表示存档版本无法读取。
var ErrUnsupportedVersion = errors.New("capture: unsupported version")

// File 是一次导出/加载的会话快照。
type File struct {
	Handshake protocol.Handshake
	Frames    []protocol.FrameData
}

type header struct {
	Magic      [4]byte
	Version    uint16
	Flags      uint16
	FrameSize  uint16
	Reserved   uint16
	FrameCount uint32
	MetaLen    uint32
}

func (h header) writeTo(w io.Writer) error {
	buf := make([]byte, headerSize)
	copy(buf[0:4], h.Magic[:])
	le := binary.LittleEndian
	le.PutUint16(buf[4:], h.Version)
	le.PutUint16(buf[6:], h.Flags)
	le.PutUint16(buf[8:], h.FrameSize)
	le.PutUint16(buf[10:], h.Reserved)
	le.PutUint32(buf[12:], h.FrameCount)
	le.PutUint32(buf[16:], h.MetaLen)
	_, err := w.Write(buf)
	return err
}

func readHeader(r io.Reader) (header, error) {
	buf := make([]byte, headerSize)
	if _, err := io.ReadFull(r, buf); err != nil {
		return header{}, err
	}
	var h header
	copy(h.Magic[:], buf[0:4])
	le := binary.LittleEndian
	h.Version = le.Uint16(buf[4:])
	h.Flags = le.Uint16(buf[6:])
	h.FrameSize = le.Uint16(buf[8:])
	h.Reserved = le.Uint16(buf[10:])
	h.FrameCount = le.Uint32(buf[12:])
	h.MetaLen = le.Uint32(buf[16:])
	return h, nil
}

// Encode 将会话写入 w。
func Encode(w io.Writer, f File) error {
	var meta []byte
	if f.Handshake != (protocol.Handshake{}) {
		b, err := json.Marshal(f.Handshake)
		if err != nil {
			return fmt.Errorf("capture: encode handshake: %w", err)
		}
		meta = b
	}
	h := header{
		Magic:      Magic,
		Version:    Version,
		FrameSize:  uint16(protocol.FramePayloadSize),
		FrameCount: uint32(len(f.Frames)),
		MetaLen:    uint32(len(meta)),
	}
	if err := h.writeTo(w); err != nil {
		return err
	}
	if len(meta) > 0 {
		if _, err := w.Write(meta); err != nil {
			return err
		}
	}
	for i := range f.Frames {
		if _, err := w.Write(protocol.EncodeFrame(&f.Frames[i])); err != nil {
			return err
		}
	}
	return nil
}

// Decode 从 r 读取存档。
func Decode(r io.Reader) (File, error) {
	h, err := readHeader(r)
	if err != nil {
		return File{}, err
	}
	if h.Magic != Magic {
		return File{}, ErrBadMagic
	}
	if h.Version != Version {
		return File{}, fmt.Errorf("%w %d", ErrUnsupportedVersion, h.Version)
	}
	if int(h.FrameSize) != protocol.FramePayloadSize && int(h.FrameSize) != protocol.LegacyFramePayloadSize {
		return File{}, fmt.Errorf("capture: frame size %d not supported", h.FrameSize)
	}
	if h.FrameCount > maxFrames {
		return File{}, fmt.Errorf("capture: frame count %d exceeds limit %d", h.FrameCount, maxFrames)
	}
	if h.MetaLen > maxMetaLen {
		return File{}, fmt.Errorf("capture: meta length %d exceeds limit", h.MetaLen)
	}

	var out File
	if h.MetaLen > 0 {
		meta := make([]byte, h.MetaLen)
		if _, err := io.ReadFull(r, meta); err != nil {
			return File{}, fmt.Errorf("capture: read meta: %w", err)
		}
		if err := json.Unmarshal(meta, &out.Handshake); err != nil {
			return File{}, fmt.Errorf("capture: decode handshake: %w", err)
		}
	}

	out.Frames = make([]protocol.FrameData, h.FrameCount)
	payload := make([]byte, h.FrameSize)
	for i := uint32(0); i < h.FrameCount; i++ {
		if _, err := io.ReadFull(r, payload); err != nil {
			return File{}, fmt.Errorf("capture: read frame %d: %w", i, err)
		}
		frame, err := protocol.DecodeFrame(payload)
		if err != nil {
			return File{}, fmt.Errorf("capture: decode frame %d: %w", i, err)
		}
		out.Frames[i] = frame
	}
	return out, nil
}

// SaveFile 将会话写入 path。
func SaveFile(path string, f File) error {
	w, err := os.Create(path)
	if err != nil {
		return err
	}
	defer w.Close()
	if err := Encode(w, f); err != nil {
		return err
	}
	return w.Close()
}

// LoadFile 从 path 读取会话。
func LoadFile(path string) (File, error) {
	r, err := os.Open(path)
	if err != nil {
		return File{}, err
	}
	defer r.Close()
	return Decode(r)
}

// EncodedSize 返回 n 帧存档的精确字节数（不含握手时）。
func EncodedSize(frameCount, metaLen int) int {
	return headerSize + metaLen + frameCount*protocol.FramePayloadSize
}
