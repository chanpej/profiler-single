// Package capture 读取 Unity Editor Profiler 保存的 .data 文件。
package capture

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"

	"unityperfagent/internal/profileframe"
	"unityperfagent/internal/protocol"
)

const (
	unity2022DataVersion uint32 = 0x20220328
	blockHeaderSize             = 28
	frameEndMarker       uint32 = 0xAFAFAFAF
	fileEndMarker        uint32 = 0xDEADFEED
	maxFrameBodyBytes           = 128 << 20
	maxMarkerDefs               = 100_000
	maxThreadsPerFrame          = 512
	maxSamplesPerThread         = 1_000_000
	maxStringBytes              = 1 << 20
	maxGCAllocBytes      uint32 = 1 << 30
	bytesPerKB                  = 1024
)

var (
	ErrUnsupportedFormat  = errors.New("不是 Unity 2022 Profiler .data 文件")
	ErrUnsupportedVersion = errors.New("Profiler .data 格式版本不受支持")
)

// File 是从 Unity Editor Profiler .data 文件读取的会话。
type File struct {
	Handshake protocol.Handshake
	// Frames are the normalized in-memory representation used by both offline
	// captures and the live PlayerConnection decoder.
	Frames []profileframe.Frame
}

type blockHeader struct {
	formatVersion uint32
	bodySize      uint32
	unityMajor    uint32
	unityMinor    uint32
	unityPatch    uint32
	releaseType   uint32
	releaseNumber uint32
}

type markerInfo struct {
	name       string
	categoryID uint16
}

type diskSample struct {
	markerID     uint32
	totalNS      float32
	startNS      uint64
	children     int32
	gcAllocBytes uint32
}

type reader struct {
	b   []byte
	pos int
	err error
}

func (r *reader) need(n int) bool {
	if r.err != nil {
		return false
	}
	if n < 0 || len(r.b)-r.pos < n {
		r.err = io.ErrUnexpectedEOF
		return false
	}
	return true
}
func (r *reader) skip(n int) {
	if r.need(n) {
		r.pos += n
	}
}
func (r *reader) u32() uint32 {
	if !r.need(4) {
		return 0
	}
	v := binary.LittleEndian.Uint32(r.b[r.pos:])
	r.pos += 4
	return v
}
func (r *reader) i32() int32 { return int32(r.u32()) }
func (r *reader) u64() uint64 {
	if !r.need(8) {
		return 0
	}
	v := binary.LittleEndian.Uint64(r.b[r.pos:])
	r.pos += 8
	return v
}
func (r *reader) f32() float32 { return math.Float32frombits(r.u32()) }
func (r *reader) str() string {
	if r.err != nil {
		return ""
	}
	start := r.pos
	limit := start + maxStringBytes
	if limit > len(r.b) {
		limit = len(r.b)
	}
	for i := start; i < limit; i++ {
		if r.b[i] != 0 {
			continue
		}
		n := i - start
		advance := ((n / 4) + 1) * 4
		if !r.need(advance) {
			return ""
		}
		r.pos += advance
		return string(r.b[start:i])
	}
	r.err = errors.New("Profiler .data 字符串未终止")
	return ""
}

func readBlockHeader(b []byte) blockHeader {
	le := binary.LittleEndian
	return blockHeader{
		formatVersion: le.Uint32(b[0:]), bodySize: le.Uint32(b[4:]),
		unityMajor: le.Uint32(b[8:]), unityMinor: le.Uint32(b[12:]),
		unityPatch: le.Uint32(b[16:]), releaseType: le.Uint32(b[20:]), releaseNumber: le.Uint32(b[24:]),
	}
}

// Progress reports .data bytes fully read and parsed. total is zero when the
// source length is unknown.
type Progress func(done, total int64)

// LoadFile 从 path 读取 Unity 2022 Editor Profiler 的 .data 文件。
func LoadFile(path string) (File, error) {
	return LoadFileWithProgress(path, nil)
}

// LoadFileWithProgress loads an offline capture and reports byte-level decode
// progress after every frame body. It keeps the UI responsive for large files.
func LoadFileWithProgress(path string, progress Progress) (File, error) {
	r, err := os.Open(path)
	if err != nil {
		return File{}, err
	}
	defer r.Close()
	var total int64
	if info, err := r.Stat(); err == nil {
		total = info.Size()
	}
	return DecodeWithProgress(r, total, progress)
}

// Decode 读取 Unity 2022 离线帧、主线程采样树和 CPU 分类耗时。
func Decode(src io.Reader) (File, error) {
	return DecodeWithProgress(src, 0, nil)
}

// DecodeWithProgress decodes an offline capture and reports completed bytes
// after each frame is available in the common in-memory frame model.
func DecodeWithProgress(src io.Reader, total int64, progress Progress) (File, error) {
	br := bufio.NewReaderSize(src, 1<<20)
	var done int64
	report := func() {
		if progress != nil {
			progress(done, total)
		}
	}
	markers := make(map[uint32]markerInfo)
	var out File
	var headerBuf [blockHeaderSize]byte
	seen := false

	for {
		if _, err := io.ReadFull(br, headerBuf[:4]); err != nil {
			if errors.Is(err, io.EOF) && seen {
				return File{}, errors.New("Profiler .data 缺少文件结束标记")
			}
			return File{}, fmt.Errorf("读取 Profiler .data 帧头失败：%w", err)
		}
		done += 4
		if binary.LittleEndian.Uint32(headerBuf[:4]) == fileEndMarker {
			if _, err := br.ReadByte(); err != io.EOF {
				if err == nil {
					return File{}, errors.New("Profiler .data 文件结束标记后存在意外数据")
				}
				return File{}, fmt.Errorf("读取 Profiler .data 文件结束标记失败：%w", err)
			}
			if !seen {
				return File{}, errors.New("Profiler .data 不含任何帧")
			}
			report()
			return out, nil
		}
		if binary.LittleEndian.Uint32(headerBuf[:4]) != unity2022DataVersion {
			return File{}, fmt.Errorf("%w：%#x（仅支持 Unity 2022，格式 %#x）", ErrUnsupportedVersion, binary.LittleEndian.Uint32(headerBuf[:4]), unity2022DataVersion)
		}
		if _, err := io.ReadFull(br, headerBuf[4:]); err != nil {
			return File{}, fmt.Errorf("读取 Profiler .data 帧头失败：%w", err)
		}
		done += blockHeaderSize - 4
		h := readBlockHeader(headerBuf[:])
		if h.unityMajor != 2022 {
			return File{}, fmt.Errorf("%w：Unity %d.%d.%d", ErrUnsupportedFormat, h.unityMajor, h.unityMinor, h.unityPatch)
		}
		if h.bodySize < 4 || h.bodySize > maxFrameBodyBytes {
			return File{}, fmt.Errorf("Profiler .data 帧体大小无效：%d", h.bodySize)
		}
		body := make([]byte, h.bodySize)
		if _, err := io.ReadFull(br, body); err != nil {
			return File{}, fmt.Errorf("读取 Profiler .data 帧体失败：%w", err)
		}
		done += int64(len(body))
		if binary.LittleEndian.Uint32(body[len(body)-4:]) != frameEndMarker {
			return File{}, errors.New("Profiler .data 帧结束标记无效")
		}
		if !seen {
			out.Handshake.UnityVersion = unityVersion(h)
			out.Handshake.ProfilerStreamVersion = unity2022DataVersion
			seen = true
		} else if out.Handshake.UnityVersion != unityVersion(h) {
			return File{}, errors.New("Profiler .data 内的 Unity 版本不一致")
		}

		frame, hierarchy, zeroDuration, err := parseFrameBody(body, markers)
		if err != nil {
			return File{}, fmt.Errorf("解析第 %d 个 Profiler .data 帧失败：%w", len(out.Frames), err)
		}
		if !zeroDuration {
			// Unity Editor displays an ordinal frame position, not the Player's
			// internal frame ID embedded in the capture.
			frame.FrameIndex = uint64(len(out.Frames) + 1)
			out.Frames = append(out.Frames, profileframe.New(frame, hierarchy))
		}
		report()
	}
}

func parseFrameBody(body []byte, markers map[uint32]markerInfo) (protocol.FrameData, []protocol.SampleNode, bool, error) {
	r := &reader{b: body[:len(body)-4]}
	frameID := r.i32()
	duplicateID := r.i32()
	startNS := r.u64()
	cpuUS := r.i32()
	gpuUS := r.i32()
	r.i32() // gathered data
	if r.err != nil {
		return protocol.FrameData{}, nil, false, r.err
	}
	if frameID != duplicateID {
		return protocol.FrameData{}, nil, false, fmt.Errorf("帧编号不一致：%d != %d", frameID, duplicateID)
	}
	if cpuUS == 0 {
		return protocol.FrameData{}, nil, true, nil
	}
	if err := skipStatsAndAuxiliary(r); err != nil {
		return protocol.FrameData{}, nil, false, err
	}
	if err := readMarkers(r, markers); err != nil {
		return protocol.FrameData{}, nil, false, err
	}
	mainSamples, err := readMainThreadSamples(r, markers)
	if err != nil {
		return protocol.FrameData{}, nil, false, err
	}

	cpuMS := float32(cpuUS) / 1_000
	frame := protocol.FrameData{FrameIndex: uint64(frameID), TimestampMs: int64(startNS / 1_000_000), FPS: float32(1_000_000 / float64(cpuUS)), CPUTimeMs: cpuMS, MainThreadMs: cpuMS, GPUTimeMs: float32(gpuUS) / 1_000}
	if len(mainSamples) == 0 {
		frame.CPUOthersMs = cpuMS
		return frame, nil, false, nil
	}
	hierarchy, err := buildForest(mainSamples, markers, startNS)
	if err != nil {
		return protocol.FrameData{}, nil, false, err
	}
	frame.GCAllocKB = rollUpGCAlloc(hierarchy)
	if len(hierarchy) > 0 && hierarchy[0].TotalMs > 0 {
		frame.CPUTimeMs, frame.MainThreadMs = hierarchy[0].TotalMs, hierarchy[0].TotalMs
	}
	accumulateCPU(&frame, hierarchy)
	known := frame.CPURenderingMs + frame.CPUScriptsMs + frame.CPUPhysicsMs + frame.CPUAnimationMs + frame.CPUGCMs + frame.CPUVSyncMs + frame.CPUUIMs
	if other := frame.MainThreadMs - known; other > 0 {
		frame.CPUOthersMs = other
	}
	return frame, hierarchy, false, nil
}

func skipStatsAndAuxiliary(r *reader) error {
	// MemoryStats: 33 fields, one unused value, id/value pairs ending with -1, then 16 platform values.
	r.skip(33 * 4)
	r.skip(4)
	for id := r.i32(); r.err == nil && id != -1; id = r.i32() {
		r.skip(4)
	}
	r.skip(16 * 4)
	// Remaining fixed AllProfilerStats for 0x20220328.
	r.skip(1060)
	for _, size := range []int{64, 44} {
		n := int(r.i32())
		if n < 0 || n > maxThreadsPerFrame {
			return errors.New("辅助数据数量无效")
		}
		r.skip(n * size)
	}
	names := int(r.i32())
	if names < 0 {
		return errors.New("音频名称长度无效")
	}
	r.skip(names)
	n := int(r.i32())
	if n < 0 || n > maxThreadsPerFrame {
		return errors.New("UI Canvas 数量无效")
	}
	r.skip(n * 52)
	skipNamesBlob(r)
	n = int(r.i32())
	if n < 0 || n > maxThreadsPerFrame {
		return errors.New("事件标记数量无效")
	}
	r.skip(n * 12)
	skipNamesBlob(r)
	n = int(r.i32())
	if n < 0 || n > maxSamplesPerThread {
		return errors.New("UI Batch 数量无效")
	}
	r.skip(n * 4)
	return r.err
}
func skipNamesBlob(r *reader) {
	n := int(r.i32())
	if n < 0 {
		r.err = errors.New("名称表长度无效")
		return
	}
	r.skip(n)
	if rem := r.pos % 4; rem != 0 {
		r.skip(4 - rem)
	}
}

func readMarkers(r *reader, markers map[uint32]markerInfo) error {
	n := int(r.i32())
	if n < 0 || n > maxMarkerDefs {
		return errors.New("marker 数量无效")
	}
	for range n {
		id := r.u32()
		name := r.str()
		groupFlags := r.u32()
		meta := int(r.i32())
		if meta < 0 || meta > maxMarkerDefs {
			return errors.New("marker 元数据数量无效")
		}
		markers[id] = markerInfo{name: name, categoryID: uint16(groupFlags >> 16)}
		for range meta {
			r.u32()
			r.str()
		}
	}
	return r.err
}

func readMainThreadSamples(r *reader, markers map[uint32]markerInfo) ([]diskSample, error) {
	threads := int(r.i32())
	if threads < 0 || threads > maxThreadsPerFrame {
		return nil, errors.New("线程数量无效")
	}
	for i := 0; i < threads; i++ {
		r.u64()
		r.str()
		name := r.str()
		count := int(r.i32())
		if count < 0 || count > maxSamplesPerThread {
			return nil, errors.New("采样数量无效")
		}
		if name != "Main Thread" {
			return nil, fmt.Errorf("Main Thread 必须是第一个线程，实际为 %q", name)
		}
		samples := make([]diskSample, count)
		for j := range samples {
			samples[j] = diskSample{markerID: r.u32(), totalNS: r.f32(), startNS: r.u64(), children: r.i32()}
		}
		if r.err != nil {
			return nil, r.err
		}
		readGCAllocMetadata(r.b, r.pos, samples, markers)
		return samples, nil
	}
	return nil, errors.New("找不到 Main Thread")
}

// readGCAllocMetadata 读取 Unity 2022 离线帧中的采样元数据。GC.Alloc 的 Size
// 记录以连续的 (main-thread sample index, uint32 bytes) 对编码在采样表之后。
// 帧体还包含其它线程和其它元数据；用完整、无重复的 GC.Alloc 索引序列定位该记录
// 段，避免把后续线程字段误当成分配数据。
func readGCAllocMetadata(body []byte, metadataStart int, samples []diskSample, markers map[uint32]markerInfo) {
	gcIndexes := make(map[int]struct{})
	for i, sample := range samples {
		if markers[sample.markerID].name == "GC.Alloc" {
			gcIndexes[i] = struct{}{}
		}
	}
	if len(gcIndexes) == 0 || metadataStart+8 > len(body) {
		return
	}

	best := map[int]uint32{}
	for alignment := 0; alignment < 8; alignment++ {
		run := map[int]uint32{}
		for pos := metadataStart + alignment; pos+8 <= len(body); pos += 8 {
			index := int(int32(binary.LittleEndian.Uint32(body[pos:])))
			size := binary.LittleEndian.Uint32(body[pos+4:])
			if _, wanted := gcIndexes[index]; wanted && size <= maxGCAllocBytes {
				if _, duplicate := run[index]; !duplicate {
					run[index] = size
					continue
				}
			}
			if len(run) > len(best) {
				best = run
			}
			run = map[int]uint32{}
		}
		if len(run) > len(best) {
			best = run
		}
	}
	// 一个 GC.Alloc 采样必须恰有一条 Size 记录；不接受不完整的匹配，防止损坏
	// 帧或未知后续格式导致错误的 GC 归因。
	if len(best) != len(gcIndexes) {
		return
	}
	for index, size := range best {
		samples[index].gcAllocBytes = size
	}
}

// rollUpGCAlloc 把直接挂在 GC.Alloc 采样上的 Size 向父节点累计，使每一行的
// GC Alloc 与 Unity Hierarchy 一样表示该采样子树的总分配量。
func rollUpGCAlloc(nodes []protocol.SampleNode) float32 {
	var total float32
	for i := range nodes {
		node := &nodes[i]
		node.GCAllocKB += rollUpGCAlloc(node.Children)
		total += node.GCAllocKB
	}
	return total
}

func buildForest(samples []diskSample, markers map[uint32]markerInfo, frameStartNS uint64) ([]protocol.SampleNode, error) {
	next := 0
	var roots []protocol.SampleNode
	for next < len(samples) {
		node, err := buildNode(samples, &next, markers, frameStartNS)
		if err != nil {
			return nil, err
		}
		roots = append(roots, node)
	}
	return roots, nil
}
func buildNode(samples []diskSample, next *int, markers map[uint32]markerInfo, frameStartNS uint64) (protocol.SampleNode, error) {
	if *next >= len(samples) {
		return protocol.SampleNode{}, io.ErrUnexpectedEOF
	}
	raw := samples[*next]
	*next++
	if raw.children < 0 || int(raw.children) > len(samples)-*next {
		return protocol.SampleNode{}, fmt.Errorf("无效子节点数：%d", raw.children)
	}
	info := markers[raw.markerID]
	total := raw.totalNS / 1_000_000
	start := float32(0)
	if raw.startNS >= frameStartNS {
		start = float32(float64(raw.startNS-frameStartNS) / 1_000_000)
	}
	node := protocol.SampleNode{Name: info.name, Category: categoryName(info.categoryID), TotalMs: total, StartMs: start, Calls: 1, GCAllocKB: float32(raw.gcAllocBytes) / bytesPerKB, Children: make([]protocol.SampleNode, 0, raw.children)}
	var childTotal float32
	for range raw.children {
		child, err := buildNode(samples, next, markers, frameStartNS)
		if err != nil {
			return protocol.SampleNode{}, err
		}
		childTotal += child.TotalMs
		node.Children = append(node.Children, child)
	}
	if node.TotalMs > childTotal {
		node.SelfMs = node.TotalMs - childTotal
	}
	return node, nil
}

func accumulateCPU(frame *protocol.FrameData, nodes []protocol.SampleNode) {
	for _, n := range nodes {
		switch n.Category {
		case "Render":
			frame.CPURenderingMs += n.SelfMs
		case "Scripts":
			frame.CPUScriptsMs += n.SelfMs
		case "Physics", "Physics2D":
			frame.CPUPhysicsMs += n.SelfMs
		case "Animation":
			frame.CPUAnimationMs += n.SelfMs
		case "GC":
			frame.CPUGCMs += n.SelfMs
		case "VSync":
			frame.CPUVSyncMs += n.SelfMs
		case "Gui", "UI Layout", "UI Render", "UI Details":
			frame.CPUUIMs += n.SelfMs
		}
		accumulateCPU(frame, n.Children)
	}
}

func categoryName(id uint16) string {
	switch id {
	case 0:
		return "Render"
	case 1:
		return "Scripts"
	case 4:
		return "Gui"
	case 5:
		return "Physics"
	case 6:
		return "Animation"
	case 17:
		return "GC"
	case 18:
		return "VSync"
	case 26:
		return "UI Layout"
	case 27:
		return "UI Render"
	case 33:
		return "Physics2D"
	case 35:
		return "UI Details"
	default:
		return ""
	}
}

func unityVersion(h blockHeader) string {
	release := map[uint32]string{0: "a", 1: "b", 2: "f", 3: "c"}[h.releaseType]
	if release == "" {
		release = "?"
	}
	return fmt.Sprintf("%d.%d.%d%s%d", h.unityMajor, h.unityMinor, h.unityPatch, release, h.releaseNumber)
}
func IsFiniteFPS(v float32) bool { return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) }
