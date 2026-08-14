// Package collector 负责性能帧数据的会话级存储与统计。
//
// 存储使用固定容量的环形缓冲区，追加为 O(1)，按范围读取为 O(n)，
// 可支撑长时间采集而内存占用有上界，配合前端虚拟化列表按需取数。
package collector

import (
	"math"
	"sort"
	"sync"

	"unityperfagent/internal/protocol"
)

// DefaultCapacity 是默认可保留的帧数（约 30 分钟 @60fps）。
const DefaultCapacity = 108000

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
	mu    sync.RWMutex
	buf   []protocol.FrameData
	head  int // 下一个写入位置
	size  int // 当前存储帧数
	total uint64
}

// NewSession 创建容量为 capacity 的会话；capacity <= 0 时使用 DefaultCapacity。
func NewSession(capacity int) *Session {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &Session{buf: make([]protocol.FrameData, capacity)}
}

// Append 追加一帧数据。
func (s *Session) Append(f protocol.FrameData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf[s.head] = f
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

// Clear 清空会话。
func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.head, s.size, s.total = 0, 0, 0
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
