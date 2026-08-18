package unityprofiler

import (
	"encoding/binary"
	"errors"
	"fmt"

	"unityperfagent/internal/profileframe"
	"unityperfagent/internal/protocol"
)

// ErrNoStreamHeader 表示存档里没有 PD3U 流头，无法解码。
var ErrNoStreamHeader = errors.New("缺少 Profiler 数据流头（PD3U），无法解析。请重新采集并保存完整数据")

// ReplayProgress 在 Replay 过程中报告已处理分块数（done/total）。
type ReplayProgress func(done, total int)

// Replay 把保存的原始 Profiler 数据流重新送入解码器。保留摘要和 Hierarchy
// 的旧投影接口；新调用方应使用 ReplayFrames 获取统一内存模型。
func Replay(chunks [][]byte) (frames []protocol.FrameData, hierarchies [][]protocol.SampleNode, err error) {
	return ReplayWithProgress(chunks, nil)
}

// ReplayWithProgress 与 Replay 相同，并按分块回调进度。
func ReplayWithProgress(chunks [][]byte, progress ReplayProgress) (frames []protocol.FrameData, hierarchies [][]protocol.SampleNode, err error) {
	memoryFrames, err := ReplayFramesWithProgress(chunks, progress)
	if err != nil {
		return nil, nil, err
	}
	frames = make([]protocol.FrameData, len(memoryFrames))
	hierarchies = make([][]protocol.SampleNode, len(memoryFrames))
	for i, frame := range memoryFrames {
		frames[i] = frame.Summary
		hierarchies[i] = frame.MainThread
	}
	return frames, hierarchies, nil
}

// ReplayFrames normalizes saved PlayerConnection data into the same in-memory
// frame model used by live decoding and .data imports.
func ReplayFrames(chunks [][]byte) ([]profileframe.Frame, error) {
	return ReplayFramesWithProgress(chunks, nil)
}

// ReplayFramesWithProgress is ReplayFrames with a per-chunk progress callback.
func ReplayFramesWithProgress(chunks [][]byte, progress ReplayProgress) (frames []profileframe.Frame, err error) {
	report := func(done int) {
		if progress != nil {
			progress(done, len(chunks))
		}
	}
	if len(chunks) == 0 {
		return nil, errors.New("原始 Profiler 数据为空")
	}
	hdr, ok := firstStreamHeader(chunks)
	if !ok {
		return nil, ErrNoStreamHeader
	}
	h, err := ParseStreamHeader(hdr)
	if err != nil {
		return nil, fmt.Errorf("Profiler 数据流头无效：%w", err)
	}
	if !IsSupportedStreamVersion(h.FormatVersion) {
		return nil, fmt.Errorf("Profiler 数据流格式 %#x 不受支持（仅支持 Unity %s，格式 %#x）",
			h.FormatVersion, protocol.SupportedUnityMajor, Unity2022StreamVersion)
	}

	dec := NewDecoder(func(snap FrameSnapshot) {
		frames = append(frames, ToMemoryFrame(snap))
	})
	report(0)
	for i, c := range chunks {
		if ferr := dec.Feed(c); ferr != nil {
			st := dec.Stats()
			if st.Desynced {
				return nil, fmt.Errorf("Profiler 数据流解析失步（分块 %d）：%s", i, st.LastError)
			}
		}
		report(i + 1)
	}
	st := dec.Stats()
	if st.Desynced {
		return nil, fmt.Errorf("Profiler 数据流解析失步：%s", st.LastError)
	}
	if len(frames) == 0 {
		reason := st.LastError
		if reason == "" && st.DroppedChunks > 0 {
			reason = fmt.Sprintf("丢弃了 %d 个分块", st.DroppedChunks)
		}
		if reason == "" {
			reason = "未找到有效帧边界"
		}
		return nil, fmt.Errorf("未能从原始数据中解析出任何帧：%s", reason)
	}
	return frames, nil
}

func firstStreamHeader(chunks [][]byte) ([]byte, bool) {
	for _, c := range chunks {
		if len(c) >= 4 && binary.LittleEndian.Uint32(c[:4]) == streamMagic {
			return c, true
		}
	}
	return nil, false
}

// SampleProfilerCapture 生成一段含 Deep Profile 采样树的原始数据流，
// 布局与真机 PD3U 分块一致，供存档往返与 Replay 单测使用。
func SampleProfilerCapture() [][]byte {
	header := EncodeStreamHeader(StreamHeader{
		FormatVersion: Unity2022StreamVersion,
		MainThreadID:  fakeMainThreadID,
	}, 2022, 3, 47, 2, 1)

	var meta RecordWriter
	meta.CategoryDef(catScripts, "Scripts")
	meta.CategoryDef(catGC, "GC")
	meta.CategoryDef(catInternal, "Internal")
	meta.MarkerDef(mkMainThread, 0x08, catInternal, "Main Thread")
	meta.MarkerDef(mkScripts, 0, catScripts, "BehaviourUpdate")
	meta.MarkerDef(mkPlayerUpd, 0, catScripts, "PlayerController.Update")
	meta.MarkerDef(mkGCAlloc, 0, catGC, "GC.Alloc")
	meta.MethodDef(fakeMethodPlayerUpd, "Assembly-CSharp.dll!::PlayerController.Update()", "", 0)

	const ns = int64(1_000_000)
	var ev RecordWriter
	ev.FrameBoundary(1, 0)
	ev.Begin(mkMainThread, 0)
	ev.Begin(mkScripts, 0)
	ev.Begin(mkPlayerUpd, 0)
	ev.Begin(mkGCAlloc, ns)
	ev.Int64Meta(4096)
	ev.Callstack(fakeMethodPlayerUpd+0x40, fakeNativeAddr)
	ev.End(mkGCAlloc, ns+1000)
	ev.End(mkPlayerUpd, 4*ns)
	ev.End(mkScripts, 5*ns)
	ev.End(mkMainThread, 10*ns)
	ev.FrameBoundary(2, 10*ns)

	return [][]byte{
		header,
		EncodeChunk(0, metaThreadID, meta.Bytes()),
		EncodeChunk(0, fakeMainThreadID, ev.Bytes()),
	}
}
