package unityprofiler

import (
	"time"

	"unityperfagent/internal/profileframe"
	"unityperfagent/internal/protocol"
)

// 计数器键为 `ProfilerCategory/计数器名`，与 Unity Profiler 各模块的图表一一对应。
const (
	ctrCPUTotal     = "Render/CPU Total Frame Time"
	ctrCPUMain      = "Render/CPU Main Thread Frame Time"
	ctrCPURender    = "Render/CPU Render Thread Frame Time"
	ctrGPUFrame     = "Render/GPU Frame Time"
	ctrDrawCalls    = "Render/Draw Calls Count"
	ctrSetPassCalls = "Render/SetPass Calls Count"
	ctrBatches      = "Render/Batches Count"
	ctrTriangles    = "Render/Triangles Count"
	ctrVertices     = "Render/Vertices Count"
	ctrShadowCaster = "Render/Shadow Casters Count"

	ctrTotalUsed        = "Memory/Total Used Memory"
	ctrTotalReserved    = "Memory/Total Reserved Memory"
	ctrGCUsed           = "Memory/GC Used Memory"
	ctrGCReserved       = "Memory/GC Reserved Memory"
	ctrGfxUsed          = "Memory/Gfx Used Memory"
	ctrGfxReserved      = "Memory/Gfx Reserved Memory"
	ctrAudioUsed        = "Memory/Audio Used Memory"
	ctrVideoUsed        = "Memory/Video Used Memory"
	ctrProfilerUsed     = "Memory/Profiler Used Memory"
	ctrProfilerReserved = "Memory/Profiler Reserved Memory"
	ctrSystemUsed       = "Memory/System Used Memory"
	ctrGCAllocInFrame   = "Memory/GC Allocated In Frame"

	ctrPlayingSources = "Audio/Playing Audio Sources"
	ctrAudioVoices    = "Audio/Total Audio Voices"
	// Unity 2022 的物理计数器按刚体类型拆开上报，活动刚体数取两者之和；
	// ctrActiveRigidbodies 是旧版名字，仅作兜底。
	ctrActiveDynamicBodies   = "Physics/Active Dynamic Bodies"
	ctrActiveKinematicBodies = "Physics/Active Kinematic Bodies"
	ctrActiveRigidbodies     = "Physics/Active Rigidbodies"
	ctrStaticColliders       = "Physics/Static Colliders"
	ctrUIBatches             = "UI Details/Batches Count"
	ctrUIVertices            = "UI Details/Vertices Count"
)

const (
	bytesPerMB = 1024 * 1024
	bytesPerKB = 1024
	nsPerMS    = 1e6
)

// cpuCategoryField 把 ProfilerCategory 归并到 CPU 占用图的各个分段。
var cpuCategoryField = map[string]func(*protocol.FrameData, float32){
	"Render":     func(f *protocol.FrameData, v float32) { f.CPURenderingMs += v },
	"Scripts":    func(f *protocol.FrameData, v float32) { f.CPUScriptsMs += v },
	"Physics":    func(f *protocol.FrameData, v float32) { f.CPUPhysicsMs += v },
	"Physics2D":  func(f *protocol.FrameData, v float32) { f.CPUPhysicsMs += v },
	"Animation":  func(f *protocol.FrameData, v float32) { f.CPUAnimationMs += v },
	"GC":         func(f *protocol.FrameData, v float32) { f.CPUGCMs += v },
	"VSync":      func(f *protocol.FrameData, v float32) { f.CPUVSyncMs += v },
	"Gui":        func(f *protocol.FrameData, v float32) { f.CPUUIMs += v },
	"UI Layout":  func(f *protocol.FrameData, v float32) { f.CPUUIMs += v },
	"UI Render":  func(f *protocol.FrameData, v float32) { f.CPUUIMs += v },
	"UI Details": func(f *protocol.FrameData, v float32) { f.CPUUIMs += v },
}

// ToMemoryFrame normalizes a live raw-stream snapshot into the same in-memory
// frame model used by offline .data imports.
func ToMemoryFrame(snap FrameSnapshot) profileframe.Frame {
	return profileframe.New(ToFrameData(snap), snap.Hierarchy)
}

// ToFrameData projects a raw snapshot to the summary consumed by the frame list
// and charts. CPU Usage and Hierarchy both use the Main Thread root duration.
func ToFrameData(snap FrameSnapshot) protocol.FrameData {
	f := protocol.FrameData{
		FrameIndex:  uint64(snap.Index),
		TimestampMs: time.Now().UnixMilli(),
	}

	frameMs := float32(float64(snap.DurationNS) / nsPerMS)
	rootMs := float32(float64(snap.RootNS) / nsPerMS)
	f.CPUTimeMs = rootMs
	if f.CPUTimeMs <= 0 {
		f.CPUTimeMs = pickMs(snap, ctrCPUTotal, frameMs)
	}
	f.MainThreadMs = rootMs
	if f.MainThreadMs <= 0 {
		f.MainThreadMs = pickMs(snap, ctrCPUMain, f.CPUTimeMs)
	}
	f.RenderMs = pickMs(snap, ctrCPURender, 0)
	f.GPUTimeMs = pickMs(snap, ctrGPUFrame, 0)
	if frameMs > 0 {
		f.FPS = 1000 / frameMs
	}

	f.DrawCalls = pickCount(snap, ctrDrawCalls)
	f.SetPassCalls = pickCount(snap, ctrSetPassCalls)
	f.Batches = pickCount(snap, ctrBatches)
	f.Triangles = pickCount(snap, ctrTriangles)
	f.Vertices = pickCount(snap, ctrVertices)
	f.ShadowCasters = pickCount(snap, ctrShadowCaster)
	f.PlayingSources = pickCount(snap, ctrPlayingSources)
	f.AudioVoices = pickCount(snap, ctrAudioVoices)
	f.ActiveRigidbodies = pickCount(snap, ctrActiveDynamicBodies) +
		pickCount(snap, ctrActiveKinematicBodies) +
		pickCount(snap, ctrActiveRigidbodies)
	f.Colliders = pickCount(snap, ctrStaticColliders)
	f.UIBatches = pickCount(snap, ctrUIBatches)
	f.UIVertices = pickCount(snap, ctrUIVertices)

	f.UsedMemMB = pickMB(snap, ctrTotalUsed)
	f.ReservedMB = pickMB(snap, ctrTotalReserved)
	f.GCUsedMB = pickMB(snap, ctrGCUsed)
	f.GCReservedMB = pickMB(snap, ctrGCReserved)
	f.GfxUsedMB = pickMB(snap, ctrGfxUsed)
	f.GfxReservedMB = pickMB(snap, ctrGfxReserved)
	f.AudioUsedMB = pickMB(snap, ctrAudioUsed)
	f.VideoUsedMB = pickMB(snap, ctrVideoUsed)
	f.ProfilerUsedMB = pickMB(snap, ctrProfilerUsed)
	f.ProfilerReservedMB = pickMB(snap, ctrProfilerReserved)
	f.SystemUsedMB = pickMB(snap, ctrSystemUsed)
	if v, ok := snap.Counters[ctrGCAllocInFrame]; ok {
		f.GCAllocKB = float32(v / bytesPerKB)
	}

	var known float32
	for cat, ns := range snap.CategoryNS {
		ms := float32(float64(ns) / nsPerMS)
		if set, ok := cpuCategoryField[cat]; ok {
			set(&f, ms)
			known += ms
		}
	}
	total := float32(float64(snap.RootNS) / nsPerMS)
	if others := total - known; others > 0 {
		f.CPUOthersMs = others
	}
	return f
}

func pickMs(snap FrameSnapshot, key string, fallback float32) float32 {
	if v, ok := snap.Counters[key]; ok {
		return float32(v / nsPerMS)
	}
	return fallback
}

func pickMB(snap FrameSnapshot, key string) float32 {
	if v, ok := snap.Counters[key]; ok {
		return float32(v / bytesPerMB)
	}
	return 0
}

func pickCount(snap FrameSnapshot, key string) uint32 {
	if v, ok := snap.Counters[key]; ok && v > 0 {
		return uint32(v)
	}
	return 0
}
