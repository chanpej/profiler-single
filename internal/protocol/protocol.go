// Package protocol 实现 Profiler 模块快照的编解码，以及握手校验。
//
// FrameData 是 Unity Profiler 各模块计数器的固定长度快照（v2 164 字节，
// 兼容 v1 68 字节），作为 Editor Profiler 协议（internal/unityprofiler）
// 中 ProfilerFrame 报文的载荷。握手 JSON 在 Connect 应答中校验 Unity 2022。
package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

// Magic 是协议帧头的魔数。
var Magic = [4]byte{'U', 'P', 'A', '1'}

// MsgType 是消息类型。
type MsgType uint8

const (
	// MsgHandshake 握手消息，Payload 为设备信息 (UTF-8 字符串)。
	MsgHandshake MsgType = 1
	// MsgFrame 每帧性能数据。
	MsgFrame MsgType = 2
	// MsgPing 心跳。
	MsgPing MsgType = 3
)

// MaxPayloadSize 限制单条消息负载大小，防止异常数据导致内存暴涨。
const MaxPayloadSize = 1 << 20 // 1 MiB

// LegacyFramePayloadSize 是 v1 帧负载的固定字节数（仅基础字段）。
const LegacyFramePayloadSize = 8 + 8 + 4*8 + 4*5 // 68

// ExtendedFramePayloadSize 是 v2 帧负载的固定字节数。
const ExtendedFramePayloadSize = LegacyFramePayloadSize + 4*17 + 4*7 // 164

// FramePayloadSize 是 v3 FrameData 二进制编码后的固定字节数。
// v3 增加 Unity Memory 模块的 committed、资源统计和可用性掩码；解码仍兼容 v1/v2。
const FramePayloadSize = ExtendedFramePayloadSize + 4*10 // 204

// MemoryCounterMask 标记当前帧实际提供的 Memory 模块计数器。零值表示
// 来源未声明可用性（兼容旧协议）；前端可据此避免把缺失数据显示成 0。
const (
	MemoryCounterTotalUsed uint32 = 1 << iota
	MemoryCounterTotalReserved
	MemoryCounterGCUsed
	MemoryCounterGCReserved
	MemoryCounterGfxUsed
	MemoryCounterGfxReserved
	MemoryCounterAudioUsed
	MemoryCounterVideoUsed
	MemoryCounterProfilerUsed
	MemoryCounterProfilerReserved
	MemoryCounterSystemUsed
	MemoryCounterCommitted
	MemoryCounterTexture
	MemoryCounterMesh
	MemoryCounterMaterialCount
	MemoryCounterObjectCount
	MemoryCounterTextureCount
	MemoryCounterMeshCount
	MemoryCounterGameObjectCount
	MemoryCounterSceneObjectCount
)

// FrameData 是一帧的性能采样数据，字段按 Unity Profiler 模块分组。
type FrameData struct {
	FrameIndex   uint64  `json:"frameIndex"`
	TimestampMs  int64   `json:"timestampMs"`
	FPS          float32 `json:"fps"`
	CPUTimeMs    float32 `json:"cpuTimeMs"`
	GPUTimeMs    float32 `json:"gpuTimeMs"`
	MainThreadMs float32 `json:"mainThreadMs"`
	RenderMs     float32 `json:"renderMs"`
	GCAllocKB    float32 `json:"gcAllocKB"`
	UsedMemMB    float32 `json:"usedMemMB"`
	ReservedMB   float32 `json:"reservedMB"`
	DrawCalls    uint32  `json:"drawCalls"`
	SetPassCalls uint32  `json:"setPassCalls"`
	Batches      uint32  `json:"batches"`
	Triangles    uint32  `json:"triangles"`
	Vertices     uint32  `json:"vertices"`

	// ---- CPU Usage 模块：主线程耗时按 Unity Profiler 分类拆分（ms） ----
	CPURenderingMs float32 `json:"cpuRenderingMs"`
	CPUScriptsMs   float32 `json:"cpuScriptsMs"`
	CPUPhysicsMs   float32 `json:"cpuPhysicsMs"`
	CPUAnimationMs float32 `json:"cpuAnimationMs"`
	CPUGCMs        float32 `json:"cpuGcMs"`
	CPUVSyncMs     float32 `json:"cpuVsyncMs"`
	CPUUIMs        float32 `json:"cpuUiMs"`
	CPUOthersMs    float32 `json:"cpuOthersMs"`

	// ---- Memory 模块：分项内存（MB） ----
	GCUsedMB           float32 `json:"gcUsedMB"`
	GCReservedMB       float32 `json:"gcReservedMB"`
	GfxUsedMB          float32 `json:"gfxUsedMB"`
	GfxReservedMB      float32 `json:"gfxReservedMB"`
	AudioUsedMB        float32 `json:"audioUsedMB"`
	VideoUsedMB        float32 `json:"videoUsedMB"`
	ProfilerUsedMB     float32 `json:"profilerUsedMB"`
	ProfilerReservedMB float32 `json:"profilerReservedMB"`
	SystemUsedMB       float32 `json:"systemUsedMB"`
	// Unity Editor Memory 图表与 Simple 视图的补充计数器。
	CommittedMemMB   float32 `json:"committedMemMB"`
	TextureMemMB     float32 `json:"textureMemMB"`
	MeshMemMB        float32 `json:"meshMemMB"`
	MaterialCount    uint32  `json:"materialCount"`
	ObjectCount      uint32  `json:"objectCount"`
	TextureCount     uint32  `json:"textureCount"`
	MeshCount        uint32  `json:"meshCount"`
	GameObjectCount  uint32  `json:"gameObjectCount"`
	SceneObjectCount uint32  `json:"sceneObjectCount"`
	// MemoryCounterMask 仅用于展示可用性，区分真实 0 与采集源未提供。
	MemoryCounterMask uint32 `json:"memoryCounterMask"`

	// ---- Rendering / Audio / Physics / UI 模块计数器 ----
	ShadowCasters     uint32 `json:"shadowCasters"`
	PlayingSources    uint32 `json:"playingSources"`
	AudioVoices       uint32 `json:"audioVoices"`
	ActiveRigidbodies uint32 `json:"activeRigidbodies"`
	Colliders         uint32 `json:"colliders"`
	UIBatches         uint32 `json:"uiBatches"`
	UIVertices        uint32 `json:"uiVertices"`
}

// ErrBadMagic 表示帧头魔数不匹配。
var ErrBadMagic = errors.New("protocol: bad magic")

// ErrPayloadTooLarge 表示负载超过 MaxPayloadSize。
var ErrPayloadTooLarge = errors.New("protocol: payload too large")

// EncodeFrame 将 FrameData 编码为固定长度的二进制负载（v2，164 字节）。
func EncodeFrame(f *FrameData) []byte {
	buf := make([]byte, FramePayloadSize)
	le := binary.LittleEndian
	off := 0
	le.PutUint64(buf[off:], f.FrameIndex)
	off += 8
	le.PutUint64(buf[off:], uint64(f.TimestampMs))
	off += 8
	for _, v := range []float32{
		f.FPS, f.CPUTimeMs, f.GPUTimeMs, f.MainThreadMs, f.RenderMs, f.GCAllocKB, f.UsedMemMB, f.ReservedMB,
	} {
		le.PutUint32(buf[off:], math.Float32bits(v))
		off += 4
	}
	for _, v := range []uint32{f.DrawCalls, f.SetPassCalls, f.Batches, f.Triangles, f.Vertices} {
		le.PutUint32(buf[off:], v)
		off += 4
	}
	// v2 扩展：CPU 分类耗时。
	for _, v := range []float32{
		f.CPURenderingMs, f.CPUScriptsMs, f.CPUPhysicsMs, f.CPUAnimationMs,
		f.CPUGCMs, f.CPUVSyncMs, f.CPUUIMs, f.CPUOthersMs,
	} {
		le.PutUint32(buf[off:], math.Float32bits(v))
		off += 4
	}
	// v2 扩展：内存分项。
	for _, v := range []float32{
		f.GCUsedMB, f.GCReservedMB, f.GfxUsedMB, f.GfxReservedMB,
		f.AudioUsedMB, f.VideoUsedMB, f.ProfilerUsedMB, f.ProfilerReservedMB, f.SystemUsedMB,
	} {
		le.PutUint32(buf[off:], math.Float32bits(v))
		off += 4
	}
	// v2 扩展：渲染/音频/物理/UI 计数器。
	for _, v := range []uint32{
		f.ShadowCasters, f.PlayingSources, f.AudioVoices,
		f.ActiveRigidbodies, f.Colliders, f.UIBatches, f.UIVertices,
	} {
		le.PutUint32(buf[off:], v)
		off += 4
	}
	// v3 扩展：Unity Editor Memory 图表、Simple 视图与对象统计。
	for _, v := range []float32{f.CommittedMemMB, f.TextureMemMB, f.MeshMemMB} {
		le.PutUint32(buf[off:], math.Float32bits(v))
		off += 4
	}
	for _, v := range []uint32{
		f.MaterialCount, f.ObjectCount, f.TextureCount, f.MeshCount,
		f.GameObjectCount, f.SceneObjectCount, f.MemoryCounterMask,
	} {
		le.PutUint32(buf[off:], v)
		off += 4
	}
	return buf
}

// DecodeFrame 从二进制负载解码 FrameData。
// 兼容 v1（68 字节）、v2（164 字节）及 v3（204 字节）；旧负载新增字段保持零值。
func DecodeFrame(payload []byte) (FrameData, error) {
	if len(payload) != FramePayloadSize && len(payload) != ExtendedFramePayloadSize && len(payload) != LegacyFramePayloadSize {
		return FrameData{}, fmt.Errorf("protocol: frame payload size %d, want %d (v3), %d (v2) or %d (v1)",
			len(payload), FramePayloadSize, ExtendedFramePayloadSize, LegacyFramePayloadSize)
	}
	le := binary.LittleEndian
	var f FrameData
	off := 0
	f.FrameIndex = le.Uint64(payload[off:])
	off += 8
	f.TimestampMs = int64(le.Uint64(payload[off:]))
	off += 8
	floats := []*float32{&f.FPS, &f.CPUTimeMs, &f.GPUTimeMs, &f.MainThreadMs, &f.RenderMs, &f.GCAllocKB, &f.UsedMemMB, &f.ReservedMB}
	for _, p := range floats {
		*p = math.Float32frombits(le.Uint32(payload[off:]))
		off += 4
	}
	uints := []*uint32{&f.DrawCalls, &f.SetPassCalls, &f.Batches, &f.Triangles, &f.Vertices}
	for _, p := range uints {
		*p = le.Uint32(payload[off:])
		off += 4
	}
	if len(payload) == LegacyFramePayloadSize {
		return f, nil
	}
	cpuFloats := []*float32{
		&f.CPURenderingMs, &f.CPUScriptsMs, &f.CPUPhysicsMs, &f.CPUAnimationMs,
		&f.CPUGCMs, &f.CPUVSyncMs, &f.CPUUIMs, &f.CPUOthersMs,
	}
	for _, p := range cpuFloats {
		*p = math.Float32frombits(le.Uint32(payload[off:]))
		off += 4
	}
	memFloats := []*float32{
		&f.GCUsedMB, &f.GCReservedMB, &f.GfxUsedMB, &f.GfxReservedMB,
		&f.AudioUsedMB, &f.VideoUsedMB, &f.ProfilerUsedMB, &f.ProfilerReservedMB, &f.SystemUsedMB,
	}
	for _, p := range memFloats {
		*p = math.Float32frombits(le.Uint32(payload[off:]))
		off += 4
	}
	counters := []*uint32{
		&f.ShadowCasters, &f.PlayingSources, &f.AudioVoices,
		&f.ActiveRigidbodies, &f.Colliders, &f.UIBatches, &f.UIVertices,
	}
	for _, p := range counters {
		*p = le.Uint32(payload[off:])
		off += 4
	}
	if len(payload) == ExtendedFramePayloadSize {
		return f, nil
	}
	memoryFloats := []*float32{&f.CommittedMemMB, &f.TextureMemMB, &f.MeshMemMB}
	for _, p := range memoryFloats {
		*p = math.Float32frombits(le.Uint32(payload[off:]))
		off += 4
	}
	memoryCounts := []*uint32{
		&f.MaterialCount, &f.ObjectCount, &f.TextureCount, &f.MeshCount,
		&f.GameObjectCount, &f.SceneObjectCount, &f.MemoryCounterMask,
	}
	for _, p := range memoryCounts {
		*p = le.Uint32(payload[off:])
		off += 4
	}
	return f, nil
}

// WriteMessage 向 w 写入一条完整协议消息。
func WriteMessage(w io.Writer, t MsgType, payload []byte) error {
	if len(payload) > MaxPayloadSize {
		return ErrPayloadTooLarge
	}
	header := make([]byte, 9)
	copy(header[:4], Magic[:])
	header[4] = byte(t)
	binary.LittleEndian.PutUint32(header[5:], uint32(len(payload)))
	if _, err := w.Write(header); err != nil {
		return err
	}
	if len(payload) > 0 {
		if _, err := w.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

// ReadMessage 从 r 读取一条完整协议消息。
func ReadMessage(r io.Reader) (MsgType, []byte, error) {
	header := make([]byte, 9)
	if _, err := io.ReadFull(r, header); err != nil {
		return 0, nil, err
	}
	if [4]byte(header[:4]) != Magic {
		return 0, nil, ErrBadMagic
	}
	t := MsgType(header[4])
	n := binary.LittleEndian.Uint32(header[5:])
	if n > MaxPayloadSize {
		return 0, nil, ErrPayloadTooLarge
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return t, payload, nil
}
