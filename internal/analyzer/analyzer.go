// Package analyzer 对采集到的帧数据做规则化性能诊断，
// 输出结构化 Report，可直接展示，也可作为 LLM 诊断的输入。
package analyzer

import (
	"fmt"
	"sort"

	"unityperf/internal/protocol"
)

// Severity 是问题严重程度。
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Issue 是一个诊断出的性能问题。
type Issue struct {
	Rule       string   `json:"rule"`
	Severity   Severity `json:"severity"`
	Title      string   `json:"title"`
	Detail     string   `json:"detail"`
	FrameIndex uint64   `json:"frameIndex,omitempty"`
}

// Report 是一次分析的完整结果。
type Report struct {
	FrameCount int     `json:"frameCount"`
	Issues     []Issue `json:"issues"`
	Summary    string  `json:"summary"`
}

// 阈值配置（针对移动端 Unity 2022 游戏的经验值）。
const (
	spikeFactor        = 2.0  // 帧耗时超过中位数的倍数即视为卡顿
	minSpikeMs         = 33.4 // 低于该耗时不算卡顿（~30fps 一帧）
	gcSpikeKB          = 256  // 单帧 GC 分配超过该值视为 GC 尖峰
	highDrawCalls      = 500  // DrawCall 警戒线
	criticalDrawCalls  = 1000 // DrawCall 严重线
	lowFPSThreshold    = 30   // 平均 FPS 低于该值告警
	memGrowthMBPerMin  = 30   // 每分钟内存增长超过该值视为疑似泄漏
	maxReportedSpikes  = 20   // 最多报告的卡顿帧数量
	minFramesToAnalyze = 30   // 少于该帧数不做趋势分析
)

// Analyze 对帧序列执行全部规则并生成报告。
func Analyze(frames []protocol.FrameData) Report {
	r := Report{FrameCount: len(frames), Issues: []Issue{}}
	if len(frames) == 0 {
		r.Summary = "没有可分析的帧数据。"
		return r
	}
	r.Issues = append(r.Issues, detectFrameSpikes(frames)...)
	r.Issues = append(r.Issues, detectGCSpikes(frames)...)
	r.Issues = append(r.Issues, detectLowFPS(frames)...)
	r.Issues = append(r.Issues, detectHighDrawCalls(frames)...)
	r.Issues = append(r.Issues, detectMemoryGrowth(frames)...)
	r.Summary = summarize(&r)
	return r
}

// detectFrameSpikes 找出耗时显著高于中位数的卡顿帧。
func detectFrameSpikes(frames []protocol.FrameData) []Issue {
	times := make([]float64, len(frames))
	for i := range frames {
		times[i] = float64(frames[i].CPUTimeMs)
	}
	med := median(times)
	threshold := med * spikeFactor
	if threshold < minSpikeMs {
		threshold = minSpikeMs
	}
	var issues []Issue
	for i := range frames {
		if float64(frames[i].CPUTimeMs) > threshold {
			sev := SeverityWarning
			if float64(frames[i].CPUTimeMs) > threshold*2 {
				sev = SeverityCritical
			}
			issues = append(issues, Issue{
				Rule:       "frame_spike",
				Severity:   sev,
				Title:      "帧耗时尖峰",
				Detail:     fmt.Sprintf("第 %d 帧 CPU 耗时 %.1fms，超过阈值 %.1fms（中位数 %.1fms）", frames[i].FrameIndex, frames[i].CPUTimeMs, threshold, med),
				FrameIndex: frames[i].FrameIndex,
			})
			if len(issues) >= maxReportedSpikes {
				break
			}
		}
	}
	return issues
}

// detectGCSpikes 找出单帧 GC 分配过大的帧。
func detectGCSpikes(frames []protocol.FrameData) []Issue {
	var issues []Issue
	for i := range frames {
		if frames[i].GCAllocKB > gcSpikeKB {
			issues = append(issues, Issue{
				Rule:       "gc_spike",
				Severity:   SeverityWarning,
				Title:      "GC 分配尖峰",
				Detail:     fmt.Sprintf("第 %d 帧 GC 分配 %.0fKB，超过阈值 %dKB，可能触发 GC 卡顿", frames[i].FrameIndex, frames[i].GCAllocKB, gcSpikeKB),
				FrameIndex: frames[i].FrameIndex,
			})
			if len(issues) >= maxReportedSpikes {
				break
			}
		}
	}
	return issues
}

// detectLowFPS 检查整体平均帧率。
func detectLowFPS(frames []protocol.FrameData) []Issue {
	var sum float64
	for i := range frames {
		sum += float64(frames[i].FPS)
	}
	avg := sum / float64(len(frames))
	if avg >= lowFPSThreshold {
		return nil
	}
	return []Issue{{
		Rule:     "low_fps",
		Severity: SeverityCritical,
		Title:    "平均帧率过低",
		Detail:   fmt.Sprintf("平均 FPS 仅 %.1f，低于 %d 的可玩性下限", avg, lowFPSThreshold),
	}}
}

// detectHighDrawCalls 检查 DrawCall 峰值。
func detectHighDrawCalls(frames []protocol.FrameData) []Issue {
	var peak uint32
	var peakFrame uint64
	for i := range frames {
		if frames[i].DrawCalls > peak {
			peak = frames[i].DrawCalls
			peakFrame = frames[i].FrameIndex
		}
	}
	if peak <= highDrawCalls {
		return nil
	}
	sev := SeverityWarning
	if peak > criticalDrawCalls {
		sev = SeverityCritical
	}
	return []Issue{{
		Rule:       "high_drawcalls",
		Severity:   sev,
		Title:      "DrawCall 过高",
		Detail:     fmt.Sprintf("峰值 DrawCall 达 %d（第 %d 帧），建议合批或剔除优化", peak, peakFrame),
		FrameIndex: peakFrame,
	}}
}

// detectMemoryGrowth 用首尾采样估算内存增长速率，识别疑似泄漏。
func detectMemoryGrowth(frames []protocol.FrameData) []Issue {
	if len(frames) < minFramesToAnalyze {
		return nil
	}
	first, last := frames[0], frames[len(frames)-1]
	durMs := last.TimestampMs - first.TimestampMs
	if durMs <= 0 {
		return nil
	}
	growthMB := float64(last.UsedMemMB - first.UsedMemMB)
	perMin := growthMB / (float64(durMs) / 60000.0)
	if perMin <= memGrowthMBPerMin {
		return nil
	}
	return []Issue{{
		Rule:     "memory_growth",
		Severity: SeverityWarning,
		Title:    "内存持续增长",
		Detail:   fmt.Sprintf("内存以约 %.1fMB/分钟 增长（%.0fMB → %.0fMB），疑似泄漏或缓存未释放", perMin, first.UsedMemMB, last.UsedMemMB),
	}}
}

func summarize(r *Report) string {
	if len(r.Issues) == 0 {
		return fmt.Sprintf("分析了 %d 帧，未发现明显性能问题。", r.FrameCount)
	}
	counts := map[Severity]int{}
	for _, is := range r.Issues {
		counts[is.Severity]++
	}
	return fmt.Sprintf("分析了 %d 帧，发现 %d 个问题（严重 %d，警告 %d，提示 %d）。",
		r.FrameCount, len(r.Issues), counts[SeverityCritical], counts[SeverityWarning], counts[SeverityInfo])
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	c := make([]float64, len(v))
	copy(c, v)
	sort.Float64s(c)
	mid := len(c) / 2
	if len(c)%2 == 1 {
		return c[mid]
	}
	return (c[mid-1] + c[mid]) / 2
}
