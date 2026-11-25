package main

import (
	"sort"
	"strconv"
	"unityperf/internal/profileframe"
)

type timedFrame struct {
	duration int64
	position int
	index    uint64
}

// frameStats keeps the first captured position for ties, including the maximum.
func frameStats(frames []profileframe.Frame, start int, fps float64) []map[string]any {
	groups := make(map[int][]timedFrame)
	invalid := make(map[int]int)
	var epochs []int
	for i, f := range frames {
		epoch := f.ClockEpoch
		if _, exists := groups[epoch]; !exists {
			groups[epoch] = nil
			epochs = append(epochs, epoch)
		}
		if f.PlayerEndNS <= f.PlayerStartNS {
			invalid[epoch]++
			continue
		}
		groups[epoch] = append(groups[epoch], timedFrame{f.PlayerEndNS - f.PlayerStartNS, start + i, f.Summary.FrameIndex})
	}
	sort.Ints(epochs)
	result := make([]map[string]any, 0, len(epochs))
	for _, epoch := range epochs {
		samples := groups[epoch]
		sort.SliceStable(samples, func(i, j int) bool { return samples[i].duration < samples[j].duration })
		n := len(samples)
		row := map[string]any{"clockEpoch": epoch, "valid": n, "invalidClock": invalid[epoch]}
		if n > 0 {
			for label, pct := range map[string]int{"p50": 50, "p95": 95, "p99": 99, "max": 100} {
				duration := samples[(pct*n+99)/100-1].duration
				first := sort.Search(n, func(i int) bool { return samples[i].duration >= duration })
				sample := samples[first]
				row[label] = map[string]any{"ms": float64(duration) / 1e6, "position": sample.position, "frameIndex": strconv.FormatUint(sample.index, 10)}
			}
		}
		if fps > 0 {
			firstOver := sort.Search(n, func(i int) bool { return float64(samples[i].duration)*fps > 1e9 })
			row["targetFps"], row["overBudget"], row["overBudgetPct"] = fps, n-firstOver, nil
			if n > 0 {
				row["overBudgetPct"] = 100 * float64(n-firstOver) / float64(n)
			}
		}
		result = append(result, row)
	}
	return result
}
