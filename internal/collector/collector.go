// Package collector 负责性能帧数据的会话级存储与统计。
//
// 存储使用固定容量的环形缓冲区，追加为 O(1)，按范围读取为 O(n)，
// 可支撑长时间采集而内存占用有上界，配合前端虚拟化列表按需取数。
package collector

import (
	"encoding/binary"
	"math"
	"sort"
	"sync"

	"unityperfagent/internal/protocol"
)

const (
	// DefaultCapacity 是默认可保留的帧数（约 30 分钟 @60fps）。
	DefaultCapacity = 108000
	// maxRawBytes 限制原始 Profiler 流在内存中的上限（1 GiB）。
	maxRawBytes = 1 << 30
)

// Stats 是当前会话的聚合统计。
type Stats struct {
	FrameCount   int     `json:"frameCount"`
	AvgFPS       float64 `json:"avgFps"`
	MinFPS       float64 `json:"minFps"`
	MaxFPS       float64 `json:"maxFps"`
	AvgCPUMs     float64 `json:"avgCpuMs"`
	P95CPUMs     float64 `json:"p95CpuMs"`
	MaxCPUMs     float64 `json:"maxCpuMs"`
	AvgDrawCalls float64 `json:"avgDrawCalls"`
	MaxDrawCalls uint32  `json:"maxDrawCalls"`
	TotalGCKB    float64 `json:"totalGcKb"`
	LastUsedMB   float64 `json:"lastUsedMb"`
}

// Session 是一次采集会话，线程安全。
type Session struct {
	mu       sync.RWMutex
	buf      []protocol.FrameData
	hier     [][]protocol.SampleNode
	raw      [][]byte
	rawBytes int
	header   []byte // 流头报文体，清空帧时保留，导出时补到原始流前面
	head     int    // 下一个写入位置
	size     int // 当前存储帧数
	total    uint64
}

// NewSession 创建容量为 capacity 的会话；capacity <= 0 时使用 DefaultCapacity。
func NewSession(capacity int) *Session {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &Session{
		buf:  make([]protocol.FrameData, capacity),
		hier: make([][]protocol.SampleNode, capacity),
	}
}

// Append 追加一帧数据。
func (s *Session) Append(f protocol.FrameData) {
	s.AppendDetail(f, nil)
}

// AppendDetail 追加一帧及其 CPU Hierarchy。
func (s *Session) AppendDetail(f protocol.FrameData, hierarchy []protocol.SampleNode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf[s.head] = f
	s.hier[s.head] = hierarchy
	s.head = (s.head + 1) % len(s.buf)
	if s.size < len(s.buf) {
		s.size++
	}
	s.total++
}

// Len 返回当前保留的帧数。
func (s *Session) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.size
}

// Total 返回累计接收帧数（含被环形缓冲淘汰的帧）。
func (s *Session) Total() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.total
}

// Clear 清空帧数据。原始 Profiler 流保留，避免丢掉流头后无法重新分析。
func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.head, s.size, s.total = 0, 0, 0
	for i := range s.hier {
		s.hier[i] = nil
	}
}

// ClearAll 清空帧数据与原始 Profiler 流（重新连接时使用）。
func (s *Session) ClearAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.head, s.size, s.total = 0, 0, 0
	for i := range s.hier {
		s.hier[i] = nil
	}
	s.raw, s.rawBytes = nil, 0
	s.header = nil
}

// Replace 用 frames 覆盖当前会话。超出容量时保留末尾（最新）帧。
func (s *Session) Replace(frames []protocol.FrameData) {
	s.ReplaceDetail(frames, nil)
}

// ReplaceDetail 用帧数据及其 CPU Hierarchy 覆盖当前会话。
// hierarchies 可比 frames 短；超出容量时与 frames 一并保留末尾。
func (s *Session) ReplaceDetail(frames []protocol.FrameData, hierarchies [][]protocol.SampleNode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.head, s.size, s.total = 0, 0, 0
	for i := range s.hier {
		s.hier[i] = nil
	}
	s.raw, s.rawBytes = nil, 0
	s.header = nil
	if len(frames) == 0 {
		return
	}
	start := 0
	if len(frames) > len(s.buf) {
		start = len(frames) - len(s.buf)
	}
	for i := start; i < len(frames); i++ {
		s.buf[s.head] = frames[i]
		if i < len(hierarchies) {
			s.hier[s.head] = hierarchies[i]
		}
		s.head = (s.head + 1) % len(s.buf)
		if s.size < len(s.buf) {
			s.size++
		}
		s.total++
	}
}

// Range 返回按时间顺序排列的第 offset 起 limit 帧（offset 0 为最旧帧）。
// 越界部分会被裁剪；结果是副本，可安全在锁外使用。
func (s *Session) Range(offset, limit int) []protocol.FrameData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if offset < 0 {
		offset = 0
	}
	if offset >= s.size || limit <= 0 {
		return nil
	}
	if offset+limit > s.size {
		limit = s.size - offset
	}
	out := make([]protocol.FrameData, limit)
	start := s.head - s.size + len(s.buf)
	for i := 0; i < limit; i++ {
		out[i] = s.buf[(start+offset+i)%len(s.buf)]
	}
	return out
}

// At 返回从最旧帧起第 index 帧。越界时 ok 为 false。
func (s *Session) At(index int) (protocol.FrameData, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if index < 0 || index >= s.size {
		return protocol.FrameData{}, false
	}
	start := s.head - s.size + len(s.buf)
	return s.buf[(start+index)%len(s.buf)], true
}

// HierarchyAt 返回第 index 帧的 CPU Hierarchy。无数据时 ok 为 false。
func (s *Session) HierarchyAt(index int) ([]protocol.SampleNode, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if index < 0 || index >= s.size {
		return nil, false
	}
	start := s.head - s.size + len(s.buf)
	h := s.hier[(start+index)%len(s.buf)]
	if len(h) == 0 {
		return nil, true
	}
	out := make([]protocol.SampleNode, len(h))
	copy(out, h)
	return out, true
}

// Hierarchies 返回与 Range(0, Len()) 对齐的各帧 CPU Hierarchy。
func (s *Session) Hierarchies() [][]protocol.SampleNode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.size == 0 {
		return nil
	}
	out := make([][]protocol.SampleNode, s.size)
	start := s.head - s.size + len(s.buf)
	for i := 0; i < s.size; i++ {
		h := s.hier[(start+i)%len(s.buf)]
		if len(h) == 0 {
			continue
		}
		cp := make([]protocol.SampleNode, len(h))
		copy(cp, h)
		out[i] = cp
	}
	return out
}

// AppendRaw 追加一条原始 Profiler 报文体（流头或分块），供存档原样写出。
func (s *Session) AppendRaw(body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appendRawLocked(body)
}

func (s *Session) appendRawLocked(body []byte) {
	if isProfilerStreamHeader(body) {
		s.header = make([]byte, len(body))
		copy(s.header, body)
	}
	if s.rawBytes >= maxRawBytes {
		return
	}
	if s.rawBytes+len(body) > maxRawBytes {
		return
	}
	cp := make([]byte, len(body))
	copy(cp, body)
	s.raw = append(s.raw, cp)
	s.rawBytes += len(cp)
}

// SetRaw 用原始 Profiler 流覆盖当前存档缓冲。
func (s *Session) SetRaw(chunks [][]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.raw, s.rawBytes, s.header = nil, 0, nil
	for _, c := range chunks {
		s.appendRawLocked(c)
	}
}

// Raw 返回已保存的原始 Profiler 报文体（与接收顺序一致）。
// 若会话期间确实收到过流头，导出时保证它位于分块列表开头；不会伪造流头。
func (s *Session) Raw() [][]byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.raw) == 0 && len(s.header) == 0 {
		return nil
	}
	if len(s.header) > 0 && (len(s.raw) == 0 || !isProfilerStreamHeader(s.raw[0])) {
		out := make([][]byte, 0, len(s.raw)+1)
		h := make([]byte, len(s.header))
		copy(h, s.header)
		out = append(out, h)
		out = append(out, s.raw...)
		return out
	}
	out := make([][]byte, len(s.raw))
	copy(out, s.raw)
	return out
}

// RawLen 返回已保存的原始分块数（含会话中实际收到的流头）。
func (s *Session) RawLen() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := len(s.raw)
	if len(s.header) > 0 && (n == 0 || !isProfilerStreamHeader(s.raw[0])) {
		n++
	}
	return n
}

func isProfilerStreamHeader(b []byte) bool {
	const pd3u = 0x55334450
	return len(b) >= 4 && binary.LittleEndian.Uint32(b[:4]) == pd3u
}

// Latest 返回最近的 n 帧（按时间顺序）。
func (s *Session) Latest(n int) []protocol.FrameData {
	s.mu.RLock()
	size := s.size
	s.mu.RUnlock()
	if n > size {
		n = size
	}
	return s.Range(size-n, n)
}

// Stats 计算当前会话统计信息。
func (s *Session) Stats() Stats {
	frames := s.Range(0, s.Len())
	st := Stats{FrameCount: len(frames)}
	if len(frames) == 0 {
		return st
	}
	st.MinFPS = math.MaxFloat64
	cpu := make([]float64, 0, len(frames))
	var sumFPS, sumCPU, sumDraw float64
	for i := range frames {
		f := &frames[i]
		fps := float64(f.FPS)
		sumFPS += fps
		if fps < st.MinFPS {
			st.MinFPS = fps
		}
		if fps > st.MaxFPS {
			st.MaxFPS = fps
		}
		c := float64(f.CPUTimeMs)
		sumCPU += c
		cpu = append(cpu, c)
		if c > st.MaxCPUMs {
			st.MaxCPUMs = c
		}
		sumDraw += float64(f.DrawCalls)
		if f.DrawCalls > st.MaxDrawCalls {
			st.MaxDrawCalls = f.DrawCalls
		}
		st.TotalGCKB += float64(f.GCAllocKB)
	}
	n := float64(len(frames))
	st.AvgFPS = sumFPS / n
	st.AvgCPUMs = sumCPU / n
	st.AvgDrawCalls = sumDraw / n
	st.LastUsedMB = float64(frames[len(frames)-1].UsedMemMB)
	sort.Float64s(cpu)
	st.P95CPUMs = percentile(cpu, 0.95)
	return st
}

// percentile 计算已排序切片的分位数（线性插值）。
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	rank := p * float64(len(sorted)-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return sorted[lo]
	}
	frac := rank - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}
