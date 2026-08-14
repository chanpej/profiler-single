package unityprofiler

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

// PlayerConnection 报文格式（Editor 与 Player 双向一致，实测自 Unity 2022.3 Editor）：
//
//	magic(4) = 0x67A54E8F | messageId(16) | length(4) | body(length)
//
// messageId 是一组固定的 16 字节常量，Editor 与 Player 用它区分消息种类。

// ConnMagic 是 PlayerConnection 报文头魔数。
const ConnMagic uint32 = 0x67A54E8F

// ConnHeaderSize 是 magic(4)+messageId(16)+length(4)。
const ConnHeaderSize = 24

// MaxPayloadSize 限制单条报文，防止异常流撑爆内存。
const MaxPayloadSize = 16 << 20

// MessageID 是 PlayerConnection 消息种类标识。
type MessageID [16]byte

// String 以十六进制返回消息 ID。
func (id MessageID) String() string { return fmt.Sprintf("%x", id[:]) }

// Player → Editor 的消息 ID。
var (
	// MsgIDPlayerInfo 是 Player 连接后推送的引擎信息（版本、平台、能力位）。
	MsgIDPlayerInfo = MessageID{0x49, 0x3d, 0xea, 0x75, 0xac, 0xb4, 0xe4, 0x9d, 0x48, 0x80, 0x8b, 0x72, 0x1f, 0x08, 0x58, 0xd3}
	// MsgIDAnnounce 是 `[IP] ... [ProjectName] ...` 宣告文本，Editor 与 Player 都用它。
	MsgIDAnnounce = MessageID{0x15, 0x0d, 0x0f, 0x33, 0xdb, 0x48, 0xe4, 0x89, 0x38, 0xcf, 0xf9, 0xba, 0xd2, 0x2e, 0xab, 0x07}
	// MsgIDProfilerData 是 Profiler 原始数据流（PD3U 头 + 分块）。
	MsgIDProfilerData = MessageID{0x5c, 0xd8, 0x77, 0x81, 0xf4, 0xb4, 0xb4, 0x95, 0x3b, 0xff, 0xcf, 0xf6, 0x08, 0xa0, 0x1e, 0xe0}
)

// Editor → Player 的消息 ID。
var (
	// MsgIDProfilerSetup 在连接建立后立即发送，body 为 u32(0)。
	MsgIDProfilerSetup = MessageID{0x4c, 0xd8, 0x90, 0xf7, 0xf8, 0xae, 0x64, 0x43, 0x49, 0x8b, 0x0f, 0xb8, 0xb0, 0x55, 0x0d, 0xa5}
	// MsgIDProfilerAreas 用 u32 位掩码开启/关闭 ProfilerArea；0 表示停止采集。
	MsgIDProfilerAreas = MessageID{0x22, 0x75, 0x64, 0xd6, 0xe0, 0xe0, 0x74, 0xad, 0x98, 0x28, 0xc6, 0x0f, 0xe4, 0x86, 0x31, 0xc5}
	// MsgIDProfilerCategories 用 NUL 分隔的分类名列表指定需要的 ProfilerCategory。
	MsgIDProfilerCategories = MessageID{0x77, 0x4a, 0x10, 0xe6, 0xb6, 0x68, 0x24, 0xb0, 0xe8, 0xed, 0x34, 0xed, 0xe4, 0x66, 0x3a, 0x5f}
)

// EditorAreaMask 是 Unity 2022.3 Editor 打开 Profiler 时发送的 ProfilerArea 掩码
// （实测值：除 GPU 位以外全开）。
const EditorAreaMask uint32 = 0x7FFD

// EditorCategories 是 Editor 随掩码一起下发的 ProfilerCategory 名单（实测顺序）。
var EditorCategories = []string{
	"Video", "Physics2D", "Physics", "Render", "Virtual Texturing",
	"Audio", "Memory", "Lighting", "Gui",
}

// ErrBadMagic 表示报文头魔数不匹配。
var ErrBadMagic = errors.New("unityprofiler: message header is corrupted")

// ErrPayloadTooLarge 表示 length 超过 MaxPayloadSize。
var ErrPayloadTooLarge = errors.New("unityprofiler: payload too large")

// Message 是一条 PlayerConnection 报文。
type Message struct {
	ID   MessageID
	Body []byte
}

// WriteMessage 写入一条 PlayerConnection 报文。
func WriteMessage(w io.Writer, id MessageID, body []byte) error {
	if len(body) > MaxPayloadSize {
		return ErrPayloadTooLarge
	}
	hdr := make([]byte, ConnHeaderSize)
	binary.LittleEndian.PutUint32(hdr[0:4], ConnMagic)
	copy(hdr[4:20], id[:])
	binary.LittleEndian.PutUint32(hdr[20:24], uint32(len(body)))
	if _, err := w.Write(hdr); err != nil {
		return err
	}
	if len(body) == 0 {
		return nil
	}
	_, err := w.Write(body)
	return err
}

// ReadMessage 读取一条完整 PlayerConnection 报文。
func ReadMessage(r io.Reader) (Message, error) {
	hdr := make([]byte, ConnHeaderSize)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return Message{}, err
	}
	if got := binary.LittleEndian.Uint32(hdr[0:4]); got != ConnMagic {
		return Message{}, fmt.Errorf("%w (got %08x)", ErrBadMagic, got)
	}
	var msg Message
	copy(msg.ID[:], hdr[4:20])
	n := binary.LittleEndian.Uint32(hdr[20:24])
	if n > MaxPayloadSize {
		return Message{}, ErrPayloadTooLarge
	}
	if n > 0 {
		msg.Body = make([]byte, n)
		if _, err := io.ReadFull(r, msg.Body); err != nil {
			return Message{}, err
		}
	}
	return msg, nil
}

// PlayerEngineInfo 是 MsgIDPlayerInfo 的解析结果。
type PlayerEngineInfo struct {
	// Protocol 是报文自身的版本字段（实测为 2）。
	Protocol uint32
	// Platform 是 RuntimePlatform 枚举值（Android 为 11）。
	Platform uint32
	// Version 是 Player 自报的引擎版本，如 2018.4.12f1。
	Version string
	// DeepProfilerSupported / ManagedCallstacksSupported 对应 Editor 连接详情里的两项能力。
	DeepProfilerSupported      bool
	ManagedCallstacksSupported bool
}

// PlayerInfoBodySize 是 MsgIDPlayerInfo 的固定长度。
const PlayerInfoBodySize = 31

// ParsePlayerEngineInfo 解析 Player 引擎信息报文。
//
// 布局：protocol(4) + 3 个能力字节 + platform(4) + major(4) + minor(4) + patch(4) +
// releaseType(4) + releaseNumber(4)。
func ParsePlayerEngineInfo(body []byte) (PlayerEngineInfo, error) {
	if len(body) < PlayerInfoBodySize {
		return PlayerEngineInfo{}, fmt.Errorf("unityprofiler: player info body %d bytes, want %d", len(body), PlayerInfoBodySize)
	}
	le := binary.LittleEndian
	info := PlayerEngineInfo{
		Protocol:                   le.Uint32(body[0:4]),
		DeepProfilerSupported:      body[4] != 0,
		ManagedCallstacksSupported: body[5] != 0,
		Platform:                   le.Uint32(body[7:11]),
	}
	info.Version = formatEngineVersion(
		le.Uint32(body[11:15]), le.Uint32(body[15:19]), le.Uint32(body[19:23]),
		le.Uint32(body[23:27]), le.Uint32(body[27:31]),
	)
	return info, nil
}

// EncodePlayerEngineInfo 生成 MsgIDPlayerInfo 报文体（供假 Player 使用）。
func EncodePlayerEngineInfo(platform uint32, major, minor, patch, releaseType, releaseNumber uint32) []byte {
	body := make([]byte, PlayerInfoBodySize)
	le := binary.LittleEndian
	le.PutUint32(body[0:4], 2)
	le.PutUint32(body[7:11], platform)
	le.PutUint32(body[11:15], major)
	le.PutUint32(body[15:19], minor)
	le.PutUint32(body[19:23], patch)
	le.PutUint32(body[23:27], releaseType)
	le.PutUint32(body[27:31], releaseNumber)
	return body
}

// releaseTypeLetters 把引擎版本的 releaseType 还原为字母（2 = f，正式版）。
var releaseTypeLetters = [...]byte{'a', 'b', 'f', 'p', 'x'}

func formatEngineVersion(major, minor, patch, releaseType, releaseNumber uint32) string {
	letter := byte('f')
	if releaseType < uint32(len(releaseTypeLetters)) {
		letter = releaseTypeLetters[releaseType]
	}
	return fmt.Sprintf("%d.%d.%d%c%d", major, minor, patch, letter, releaseNumber)
}

// AnnouncementFromBody 从宣告报文体中截出 `[IP] ...` 文本。
func AnnouncementFromBody(body []byte) string {
	s := string(bytes.TrimRight(body, "\x00"))
	i := strings.Index(s, "[IP]")
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(s[i:])
}

// EncodeAreaMask 生成 MsgIDProfilerAreas 报文体。
func EncodeAreaMask(mask uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, mask)
	return b
}

// EncodeCategories 生成 MsgIDProfilerCategories 报文体：每个名字以 NUL 结尾。
func EncodeCategories(names []string) []byte {
	var buf bytes.Buffer
	for _, n := range names {
		buf.WriteString(n)
		buf.WriteByte(0)
	}
	return buf.Bytes()
}

// ParseCategories 解析 MsgIDProfilerCategories 报文体。
func ParseCategories(body []byte) []string {
	var out []string
	for _, part := range bytes.Split(body, []byte{0}) {
		if len(part) > 0 {
			out = append(out, string(part))
		}
	}
	return out
}
