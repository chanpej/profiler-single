// Package upac stores unityperf sessions without pretending to be a
// Unity Editor .data file. A UPAC archive preserves the original PD3U message
// bodies so newer decoders can replay old captures losslessly.
package upac

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"unityperf/internal/protocol"
	"unityperf/internal/screenshots"
	"unityperf/internal/unityprofiler"
)

const (
	Version          uint16 = 2
	headerSize              = 20
	maxChunks               = 1 << 24
	maxMetadataBytes        = 1 << 20
	maxRawBytes             = 1 << 30
	maxScreenBytes          = 96 << 20
	screenFlag       uint16 = 1
)

var (
	Magic                 = [4]byte{'U', 'P', 'A', 'C'}
	ErrBadMagic           = errors.New("不是有效的 unityperf .upac 存档")
	ErrUnsupportedVersion = errors.New("UPAC 存档版本不受支持")
	ErrNoRawData          = errors.New("没有可保存的原始 PD3U 数据")
)

// File is a complete native capture. Raw contains Player-to-Editor
// MsgIDProfilerData bodies in receive order, including the PD3U stream header.
type PhoneInfo struct {
	Serial   string `json:"serial"`
	Model    string `json:"model"`
	Brand    string `json:"brand"`
	Android  string `json:"android"`
	SDK      string `json:"sdk"`
	ABI      string `json:"abi"`
	Screen   string `json:"screen"`
	Memory   string `json:"memory"`
	IP       string `json:"ip"`
	Endpoint string `json:"endpoint"`
}

// archiveMetadata keeps handshake keys at the root for existing UPAC v2 readers.
type archiveMetadata struct {
	protocol.Handshake
	Phone *PhoneInfo `json:"phone,omitempty"`
}

type File struct {
	Phone     *PhoneInfo
	Handshake protocol.Handshake
	Raw       [][]byte
	Screen    screenshots.Track
}

type header struct {
	Magic      [4]byte
	Version    uint16
	Flags      uint16
	FrameSize  uint16 // Reserved: retained for the retired UPAC v1 layout.
	Reserved   uint16
	ChunkCount uint32
	MetaLen    uint32
}

func (h header) writeTo(w io.Writer) error {
	var data [headerSize]byte
	copy(data[0:4], h.Magic[:])
	binary.LittleEndian.PutUint16(data[4:6], h.Version)
	binary.LittleEndian.PutUint16(data[6:8], h.Flags)
	binary.LittleEndian.PutUint16(data[8:10], h.FrameSize)
	binary.LittleEndian.PutUint16(data[10:12], h.Reserved)
	binary.LittleEndian.PutUint32(data[12:16], h.ChunkCount)
	binary.LittleEndian.PutUint32(data[16:20], h.MetaLen)
	return writeAll(w, data[:])
}

func readHeader(r io.Reader) (header, error) {
	var data [headerSize]byte
	if _, err := io.ReadFull(r, data[:]); err != nil {
		return header{}, err
	}
	var h header
	copy(h.Magic[:], data[0:4])
	h.Version = binary.LittleEndian.Uint16(data[4:6])
	h.Flags = binary.LittleEndian.Uint16(data[6:8])
	h.FrameSize = binary.LittleEndian.Uint16(data[8:10])
	h.Reserved = binary.LittleEndian.Uint16(data[10:12])
	h.ChunkCount = binary.LittleEndian.Uint32(data[12:16])
	h.MetaLen = binary.LittleEndian.Uint32(data[16:20])
	return h, nil
}

// SaveFile writes a native .upac archive.
func SaveFile(path string, file File) error {
	return SaveFileWithProgress(path, file, nil)
}

func SaveFileWithProgress(path string, file File, progress func(done, total int)) error {
	if len(file.Raw) == 0 {
		return ErrNoRawData
	}
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if err := encode(out, file, progress); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// Encode serializes a native archive to w.
func Encode(w io.Writer, file File) error {
	return encode(w, file, nil)
}

func encode(w io.Writer, file File, progress func(done, total int)) error {
	if len(file.Raw) == 0 {
		return ErrNoRawData
	}
	if len(file.Raw) > maxChunks {
		return fmt.Errorf("原始 PD3U 分块数 %d 超过上限 %d", len(file.Raw), maxChunks)
	}
	metadata, err := json.Marshal(archiveMetadata{Handshake: file.Handshake, Phone: file.Phone})
	if err != nil {
		return fmt.Errorf("编码握手信息失败：%w", err)
	}
	if len(metadata) > maxMetadataBytes {
		return fmt.Errorf("握手信息过大（%d 字节）", len(metadata))
	}
	var flags uint16
	var screenData []byte
	if len(file.Screen.Shots) > 0 {
		screenData, err = json.Marshal(file.Screen)
		if err != nil || len(screenData) > maxScreenBytes || len(file.Screen.Shots) > screenshots.MaxShots {
			return errors.New("截图数据无法保存或超过上限")
		}
		flags = screenFlag
	}
	if err := (header{Magic: Magic, Version: Version, Flags: flags, ChunkCount: uint32(len(file.Raw)), MetaLen: uint32(len(metadata))}).writeTo(w); err != nil {
		return err
	}
	if err := writeAll(w, metadata); err != nil {
		return err
	}
	totalRawBytes := 0
	for i, body := range file.Raw {
		if len(body) > unityprofiler.MaxPayloadSize {
			return fmt.Errorf("第 %d 个 PD3U 分块过大（%d 字节）", i, len(body))
		}
		if len(body) > maxRawBytes-totalRawBytes {
			return fmt.Errorf("原始 PD3U 数据超过上限 %d 字节", maxRawBytes)
		}
		var length [4]byte
		binary.LittleEndian.PutUint32(length[:], uint32(len(body)))
		if err := writeAll(w, length[:]); err != nil {
			return err
		}
		if err := writeAll(w, body); err != nil {
			return err
		}
		totalRawBytes += len(body)
		if progress != nil {
			progress(i+1, len(file.Raw))
		}
	}
	if flags&screenFlag != 0 {
		var length [4]byte
		binary.LittleEndian.PutUint32(length[:], uint32(len(screenData)))
		if err := writeAll(w, length[:]); err != nil {
			return err
		}
		return writeAll(w, screenData)
	}
	return nil
}

// LoadFile reads a native .upac archive.
func LoadFile(path string) (File, error) {
	in, err := os.Open(path)
	if err != nil {
		return File{}, err
	}
	defer in.Close()
	return Decode(in)
}

// Decode reads a native archive from r.
func Decode(r io.Reader) (File, error) {
	h, err := readHeader(r)
	if err != nil {
		return File{}, err
	}
	if h.Magic != Magic {
		return File{}, ErrBadMagic
	}
	if h.Version != Version {
		return File{}, fmt.Errorf("%w：%d", ErrUnsupportedVersion, h.Version)
	}
	if h.MetaLen > maxMetadataBytes {
		return File{}, fmt.Errorf("握手信息过大（%d 字节）", h.MetaLen)
	}
	if h.ChunkCount == 0 {
		return File{}, ErrNoRawData
	}
	if h.ChunkCount > maxChunks {
		return File{}, fmt.Errorf("原始 PD3U 分块数 %d 超过上限 %d", h.ChunkCount, maxChunks)
	}

	metadata := make([]byte, h.MetaLen)
	if _, err := io.ReadFull(r, metadata); err != nil {
		return File{}, fmt.Errorf("读取握手信息失败：%w", err)
	}
	var out File
	var meta archiveMetadata
	if err := json.Unmarshal(metadata, &meta); err != nil {
		return File{}, fmt.Errorf("握手信息损坏：%w", err)
	}
	out.Handshake, out.Phone = meta.Handshake, meta.Phone
	out.Raw = make([][]byte, h.ChunkCount)
	totalRawBytes := 0
	for i := range out.Raw {
		var length [4]byte
		if _, err := io.ReadFull(r, length[:]); err != nil {
			return File{}, fmt.Errorf("读取第 %d 个 PD3U 分块长度失败：%w", i, err)
		}
		n := binary.LittleEndian.Uint32(length[:])
		if n > unityprofiler.MaxPayloadSize {
			return File{}, fmt.Errorf("第 %d 个 PD3U 分块过大（%d 字节）", i, n)
		}
		if int(n) > maxRawBytes-totalRawBytes {
			return File{}, fmt.Errorf("原始 PD3U 数据超过上限 %d 字节", maxRawBytes)
		}
		out.Raw[i] = make([]byte, n)
		if _, err := io.ReadFull(r, out.Raw[i]); err != nil {
			return File{}, fmt.Errorf("读取第 %d 个 PD3U 分块失败：%w", i, err)
		}
		totalRawBytes += int(n)
	}
	if h.Flags&screenFlag != 0 {
		var length [4]byte
		if _, err := io.ReadFull(r, length[:]); err != nil {
			return File{}, err
		}
		size := binary.LittleEndian.Uint32(length[:])
		if size > maxScreenBytes {
			return File{}, errors.New("截图数据超过上限")
		}
		data := make([]byte, size)
		if _, err := io.ReadFull(r, data); err != nil {
			return File{}, err
		}
		if err := json.Unmarshal(data, &out.Screen); err != nil {
			return File{}, fmt.Errorf("截图轨道损坏：%w", err)
		}
		if len(out.Screen.Shots) > screenshots.MaxShots {
			return File{}, errors.New("截图数量超过上限")
		}
		for index, shot := range out.Screen.Shots {
			if len(shot.JPEG) == 0 || shot.Epoch <= 0 || (index > 0 && (shot.Epoch < out.Screen.Shots[index-1].Epoch || shot.TimeNS < out.Screen.Shots[index-1].TimeNS)) {
				return File{}, errors.New("截图轨道无效")
			}
		}
	}
	var trailing [1]byte
	if _, err := io.ReadFull(r, trailing[:]); err == nil {
		return File{}, errors.New("UPAC 存档末尾存在意外数据")
	} else if err != io.EOF {
		return File{}, err
	}
	return out, nil
}

func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}
