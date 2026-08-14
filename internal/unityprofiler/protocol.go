// Package unityprofiler 实现 Unity 2022 Editor 采集真机 Profiler 数据的 PlayerConnection 协议。
//
// 协议分三层，均由 Unity 2022.3 Editor 与 Android Player 之间的实际报文逆向确认：
//
//	1. 报文层（wire.go）：magic 0x67A54E8F + messageId(16) + length(4) + body
//	2. 数据流层（rawstream.go）：Profiler 原始流 "PD3U" 头 + 按线程多路复用的分块
//	3. 记录层（rawstream.go）：元数据流给出 marker / counter / 分类 / 线程定义，
//	   主线程流给出帧边界、采样区间与计数器取值
//
// 连接角色与 Editor 的「Attach to Player」相同：本工具作为 Profiler 客户端连到
// Development Build Player 的端口（USB 下经 adb forward 到 localabstract:Unity-<包名>）。
package unityprofiler

import (
	"fmt"
	"strings"
)

// DefaultPort 是 Unity Player TCP 监听端口。
const DefaultPort = 55000

// Player 默认在此闭区间内选一个空闲端口做 Profiler / PlayerConnection。
const (
	PlayerPortMin = 55000
	PlayerPortMax = 55016
)

// 组播发现（与 Editor / Rider / VS 调试器相同）。
const MulticastGroup = "225.0.0.222"

// MulticastPorts 是 Player 宣告使用的 UDP 端口。
var MulticastPorts = []int{54997, 34997, 57997, 58997}

// PlayerInfo 是宣告报文解析出的 Development Player（对应 Editor Attach to Player 一行）。
type PlayerInfo struct {
	IP          string `json:"ip"`
	Port        int    `json:"port"`
	Flags       int    `json:"flags"`
	GUID        string `json:"guid"`
	ID          string `json:"id"`
	Debug       bool   `json:"debug"`
	PackageName string `json:"packageName"`
	ProjectName string `json:"projectName"`
	// Version 是 PlayerConnection 协议版本字段（如 1048832），不是 Unity 编辑器版本。
	Version string `json:"version"`
	// Transport 为 "tcp"（局域网 Direct Connection）或 "adb"（USB adb forward）。
	Transport string `json:"transport"`
	Serial    string `json:"serial"`
}

// PlayerConnection Flags（与 Rider / Editor 发现协议一致）。
const (
	FlagRequestImmediateConnect = 1 << 0
	FlagSupportsProfile         = 1 << 1
)

// SupportsProfile 表示 Player 宣告支持 Profiler Attach（与 [Debug] 脚本调试无关）。
func (p PlayerInfo) SupportsProfile() bool {
	return p.Flags&FlagSupportsProfile != 0
}

// ParseAnnouncement 解析 Player 宣告文本。
// 例：
//
//	[IP] 10.0.1.152 [Port] 55000 [Flags] 3 [Guid] 2575380029 [EditorId] 4264788666
//	[Version] 1048832 [Id] AndroidPlayer(Pixel):56000 [Debug] 1 [PackageName] AndroidPlayer
//	[ProjectName] MyGame
func ParseAnnouncement(s string) (PlayerInfo, error) {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "[Port]") || !strings.Contains(s, "[IP]") {
		return PlayerInfo{}, fmt.Errorf("unityprofiler: not a player announcement")
	}
	fields := parseTagged(s)
	p := PlayerInfo{
		IP:          fields["IP"],
		GUID:        fields["Guid"],
		ID:          fields["Id"],
		PackageName: fields["PackageName"],
		ProjectName: fields["ProjectName"],
		Version:     fields["Version"],
		Debug:       strings.HasPrefix(fields["Debug"], "1"),
		Transport:   "tcp",
	}
	if v := fields["Port"]; v != "" {
		fmt.Sscanf(v, "%d", &p.Port)
	}
	if v := fields["Flags"]; v != "" {
		fmt.Sscanf(v, "%d", &p.Flags)
	}
	if p.Port <= 0 || p.IP == "" {
		return PlayerInfo{}, fmt.Errorf("unityprofiler: incomplete announcement")
	}
	return p, nil
}

func parseTagged(s string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(s, "[") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		end := strings.IndexByte(part, ']')
		if end < 0 {
			continue
		}
		out[part[:end]] = strings.TrimSpace(strings.TrimRight(part[end+1:], "\x00"))
	}
	return out
}

// FormatAnnouncement 生成宣告文本（本工具自报身份，以及模拟 Player 时使用）。
func FormatAnnouncement(p PlayerInfo) string {
	debug := "0"
	if p.Debug {
		debug = "1"
	}
	id := p.ID
	if id == "" {
		id = "AndroidPlayer(UnityPerfAgent)"
	}
	pkg := p.PackageName
	if pkg == "" {
		pkg = "AndroidPlayer"
	}
	return fmt.Sprintf("[IP] %s [Port] %d [Flags] %d [Guid] %s [EditorId] 0 [Version] 1048832 [Id] %s [Debug] %s [PackageName] %s [ProjectName] %s",
		p.IP, p.Port, p.Flags, p.GUID, id, debug, pkg, p.ProjectName)
}
