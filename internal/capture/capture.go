// Package capture 实现 Profiler 会话存档。
//
// 存档保存的是 Player 下发的原始 Profiler 数据流（MsgIDProfilerData 报文体），
// 而不是本工具解码后的帧总览 / Hierarchy。打开时由解码器按当前版本重新分析。
//
// 文件布局（小端）：
//
//	0   4  Magic "UPAC"
//	4   2  Version（1 = 旧版已解码帧；2 = 原始数据流）
//	6   2  Flags（保留）
//	8   2  FrameSize（v1 为帧负载字节数；v2 为 0）
//	10  2  Reserved
//	12  4  Count（v1 帧数；v2 原始分块数）
//	16  4  MetaLen（握手 JSON 字节数，可为 0）
//	20  N  Meta JSON
//
// v1：随后 Count * FrameSize 连续帧负载（protocol.EncodeFrame）。仅兼容旧文件。
// v2：随后 Count 个原始分块，每块为 u32 长度 + 报文体（流头或 PD3U 分块）。
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
	// Version 是当前写入版本：原始 Profiler 数据流。
	Version uint16 = 2
	// VersionLegacy 是旧版已解码帧格式，读取时仍支持。
	VersionLegacy uint16 = 1
	// headerSize 是固定文件头长度。
	headerSize = 20
	// maxFrames 防止异常 v1 文件把内存打爆。
	maxFrames = 1 << 20
	// maxChunks 防止异常 v2 文件把内存打爆。
	maxChunks = 1 << 24
	// maxMetaLen 限制握手 JSON 大小。
	maxMetaLen = 1 << 20
	// maxChunkBytes 与 PlayerConnection 单条报文上限一致。
	maxChunkBytes = 16 << 20
)

// Magic 是存档文件魔数。
var Magic = [4]byte{'U', 'P', 'A', 'C'}

// ErrBadMagic 表示文件不是 UPAC 存档。
var ErrBadMagic = errors.New("不是有效的 UPAC 存档")

// ErrUnsupportedVersion 表示存档版本无法读取。
var ErrUnsupportedVersion = errors.New("存档版本不受支持")

// File 是一次导出/加载的会话快照。
//
// 新存档只填 Handshake 与 Raw。Frames 仅用于读取旧版 v1 文件。
type File struct {
	Handshake protocol.Handshake
	Frames    []protocol.FrameData
	Raw       [][]byte
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

func marshalMeta(hs protocol.Handshake) ([]byte, error) {
	if hs == (protocol.Handshake{}) {
		return nil, nil
	}
	b, err := json.Marshal(hs)
	if err != nil {
		return nil, fmt.Errorf("capture: encode handshake: %w", err)
	}
	return b, nil
}

// Encode 将会话写入 w。优先写原始数据流（v2）；没有 Raw 时回退为旧版帧格式（v1）。
func Encode(w io.Writer, f File) error {
	if len(f.Raw) > 0 {
		return encodeRaw(w, f)
	}
	return encodeFrames(w, f)
}

func encodeFrames(w io.Writer, f File) error {
	meta, err := marshalMeta(f.Handshake)
	if err != nil {
		return err
	}
	h := header{
		Magic:      Magic,
		Version:    VersionLegacy,
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

func encodeRaw(w io.Writer, f File) error {
	meta, err := marshalMeta(f.Handshake)
	if err != nil {
		return err
	}
	h := header{
		Magic:      Magic,
		Version:    Version,
		FrameCount: uint32(len(f.Raw)),
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
	for i, chunk := range f.Raw {
		if len(chunk) > maxChunkBytes {
			return fmt.Errorf("第 %d 个原始分块过大（%d 字节）", i, len(chunk))
		}
		if err := writeU32(w, uint32(len(chunk))); err != nil {
			return err
		}
		if len(chunk) > 0 {
			if _, err := w.Write(chunk); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeU32(w io.Writer, v uint32) error {
	var tmp [4]byte
	binary.LittleEndian.PutUint32(tmp[:], v)
	_, err := w.Write(tmp[:])
	return err
}

func readU32(r io.Reader) (uint32, error) {
	var tmp [4]byte
	if _, err := io.ReadFull(r, tmp[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(tmp[:]), nil
}

// Decode 从 r 读取存档。v2 只还原 Raw；v1 只还原 Frames。
func Decode(r io.Reader) (File, error) {
	h, err := readHeader(r)
	if err != nil {
		return File{}, err
	}
	if h.Magic != Magic {
		return File{}, ErrBadMagic
	}
	if h.Version != Version && h.Version != VersionLegacy {
		return File{}, fmt.Errorf("%w（版本 %d）", ErrUnsupportedVersion, h.Version)
	}
	if h.MetaLen > maxMetaLen {
		return File{}, fmt.Errorf("握手信息过大（%d 字节）", h.MetaLen)
	}

	var out File
	if h.MetaLen > 0 {
		meta := make([]byte, h.MetaLen)
		if _, err := io.ReadFull(r, meta); err != nil {
			return File{}, fmt.Errorf("读取握手信息失败：%w", err)
		}
		if err := json.Unmarshal(meta, &out.Handshake); err != nil {
			return File{}, fmt.Errorf("握手信息损坏：%w", err)
		}
	}

	if h.Version == Version {
		return decodeRaw(r, h, out)
	}
	return decodeFrames(r, h, out)
}

func decodeFrames(r io.Reader, h header, out File) (File, error) {
	if int(h.FrameSize) != protocol.FramePayloadSize && int(h.FrameSize) != protocol.LegacyFramePayloadSize {
		return File{}, fmt.Errorf("旧版存档帧大小 %d 不受支持", h.FrameSize)
	}
	if h.FrameCount > maxFrames {
		return File{}, fmt.Errorf("旧版存档帧数 %d 超过上限 %d", h.FrameCount, maxFrames)
	}
	out.Frames = make([]protocol.FrameData, h.FrameCount)
	payload := make([]byte, h.FrameSize)
	for i := uint32(0); i < h.FrameCount; i++ {
		if _, err := io.ReadFull(r, payload); err != nil {
			return File{}, fmt.Errorf("读取第 %d 帧失败：%w", i, err)
		}
		frame, err := protocol.DecodeFrame(payload)
		if err != nil {
			return File{}, fmt.Errorf("第 %d 帧数据损坏：%w", i, err)
		}
		out.Frames[i] = frame
	}
	return out, nil
}

func decodeRaw(r io.Reader, h header, out File) (File, error) {
	if h.FrameCount > maxChunks {
		return File{}, fmt.Errorf("原始分块数 %d 超过上限 %d", h.FrameCount, maxChunks)
	}
	out.Raw = make([][]byte, h.FrameCount)
	for i := uint32(0); i < h.FrameCount; i++ {
		n, err := readU32(r)
		if err != nil {
			return File{}, fmt.Errorf("读取第 %d 个原始分块长度失败：%w", i, err)
		}
		if n > maxChunkBytes {
			return File{}, fmt.Errorf("第 %d 个原始分块过大（%d 字节）", i, n)
		}
		if n == 0 {
			continue
		}
		buf := make([]byte, n)
		if _, err := io.ReadFull(r, buf); err != nil {
			return File{}, fmt.Errorf("读取第 %d 个原始分块失败：%w", i, err)
		}
		out.Raw[i] = buf
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

// EncodedSize 返回 n 帧旧版（v1）存档的精确字节数。
func EncodedSize(frameCount, metaLen int) int {
	return headerSize + metaLen + frameCount*protocol.FramePayloadSize
}
