package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SupportedUnityMajor 是当前支持的 Unity 大版本。
// unityperf 目前只支持 Unity 2022，其他版本暂不考虑。
const SupportedUnityMajor = "2022"

// Unity2022StreamVersion 是 Unity 2022 Profiler 数据流的格式版本戳。
// 改装引擎会自报别的版本号，因此兼容性以数据流格式为准。
const Unity2022StreamVersion uint32 = 0x20220328

const Tuanjie2022StreamVersion uint32 = 0x20230224

func IsSupportedProfilerVersion(version uint32) bool {
	return version == Unity2022StreamVersion || version == Tuanjie2022StreamVersion
}

// Handshake 是设备接入时上报的基本信息，Payload 为 JSON 编码。
type Handshake struct {
	DeviceName   string `json:"deviceName"`
	DeviceModel  string `json:"deviceModel"`
	OS           string `json:"os"`
	UnityVersion string `json:"unityVersion"`
	AppVersion   string `json:"appVersion"`
	// ProfilerStreamVersion 是 Player Profiler 数据流的格式版本，采集开始后才有值。
	ProfilerStreamVersion uint32 `json:"profilerStreamVersion"`
	// MainThreadID 是数据流头里的主线程 ID，打开无流头的存档时用来补全解码。
	MainThreadID int64 `json:"mainThreadId,omitempty"`
	// DeepProfilerSupported / ManagedCallstacksSupported 来自 Player 引擎信息能力位。
	DeepProfilerSupported      bool `json:"deepProfilerSupported"`
	ManagedCallstacksSupported bool `json:"managedCallstacksSupported"`
}

// IsSupportedUnityVersion 判断 Unity 版本号是否被支持（仅 2022.x）。
func IsSupportedUnityVersion(version string) bool {
	v := strings.TrimSpace(version)
	return v == SupportedUnityMajor || strings.HasPrefix(v, SupportedUnityMajor+".")
}

// Supported 判断该 Player 能否被采集。
//
// 判定优先看 Profiler 数据流格式版本：真机上的改装引擎会自报 2018.4.12f1 之类的
// 版本号，但数据流仍是 Unity 2022 格式，能正常解析。数据流版本未知时才退回看版本号。
func (h Handshake) Supported() bool {
	if h.ProfilerStreamVersion != 0 {
		return IsSupportedProfilerVersion(h.ProfilerStreamVersion)
	}
	return IsSupportedUnityVersion(h.UnityVersion)
}

// EncodeHandshake 将握手信息编码为 JSON 负载。
func EncodeHandshake(h *Handshake) ([]byte, error) {
	return json.Marshal(h)
}

// DecodeHandshake 解码握手负载，并校验 Unity 版本是否受支持。
func DecodeHandshake(payload []byte) (Handshake, error) {
	var h Handshake
	if err := json.Unmarshal(payload, &h); err != nil {
		return Handshake{}, fmt.Errorf("protocol: bad handshake payload: %w", err)
	}
	if !IsSupportedUnityVersion(h.UnityVersion) {
		return h, fmt.Errorf("protocol: unsupported unity version %q (only %s.x is supported)", h.UnityVersion, SupportedUnityMajor)
	}
	return h, nil
}
