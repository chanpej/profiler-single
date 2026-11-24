package services

import (
	"fmt"
	"sort"

	"unityperf/internal/protocol"
)

type HeavySample struct {
	Name     string         `json:"name"`
	Category string         `json:"category"`
	Calls    uint64         `json:"calls"`
	TotalMs  float64        `json:"totalMs"`
	SelfMs   float64        `json:"selfMs"`
	Children []*HeavySample `json:"children,omitempty"`
}

type HeavyThread struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Roots     []*HeavySample `json:"roots"`
	Functions []*HeavySample `json:"functions"`
}

type HeavyProfile struct {
	FirstFrame uint64        `json:"firstFrame"`
	LastFrame  uint64        `json:"lastFrame"`
	FrameCount int           `json:"frameCount"`
	Threads    []HeavyThread `json:"threads"`
}

type heavyKey struct {
	name, category string
}

type heavyBranch struct {
	stats HeavySample
	kids  map[heavyKey]*heavyBranch
}

func (branch *heavyBranch) add(samples []protocol.SampleNode, functions map[heavyKey]*HeavySample) {
	if len(samples) > 0 && branch.kids == nil {
		branch.kids = make(map[heavyKey]*heavyBranch)
	}
	for _, sample := range samples {
		key := heavyKey{sample.Name, sample.Category}
		child := branch.kids[key]
		if child == nil {
			child = &heavyBranch{stats: HeavySample{Name: sample.Name, Category: sample.Category}}
			branch.kids[key] = child
		}
		function := functions[key]
		if function == nil {
			function = &HeavySample{Name: sample.Name, Category: sample.Category}
			functions[key] = function
		}
		for _, stats := range []*HeavySample{&child.stats, function} {
			stats.Calls += uint64(max(1, sample.Calls))
			stats.TotalMs += float64(sample.TotalMs)
			stats.SelfMs += float64(sample.SelfMs)
		}
		child.add(sample.Children, functions)
	}
}

func sortHeavySamples(samples []*HeavySample) {
	sort.Slice(samples, func(left, right int) bool {
		first, second := samples[left], samples[right]
		if first.TotalMs != second.TotalMs {
			return first.TotalMs > second.TotalMs
		}
		if first.Name != second.Name {
			return first.Name < second.Name
		}
		return first.Category < second.Category
	})
}

func (branch *heavyBranch) finish() []*HeavySample {
	children := make([]*HeavySample, 0, len(branch.kids))
	for _, child := range branch.kids {
		child.stats.Children = child.finish()
		children = append(children, &child.stats)
	}
	sortHeavySamples(children)
	return children
}

func (p *PerfService) FrameHeavyProfile(start, end int) (HeavyProfile, error) {
	if start < 0 || end < start || end >= p.session.Len() {
		return HeavyProfile{}, fmt.Errorf("所选帧范围已超出保留数据")
	}
	frames := p.session.RangeFrames(start, end-start+1)
	if len(frames) != end-start+1 {
		return HeavyProfile{}, fmt.Errorf("所选帧范围已超出保留数据")
	}
	profile := HeavyProfile{
		FirstFrame: frames[0].Summary.FrameIndex,
		LastFrame:  frames[len(frames)-1].Summary.FrameIndex,
		FrameCount: len(frames),
	}
	type threadAggregate struct {
		name      string
		root      heavyBranch
		functions map[heavyKey]*HeavySample
	}
	threads := make(map[string]*threadAggregate)
	for _, frame := range frames {
		for _, thread := range frame.TimelineThreads() {
			id := thread.ID
			if id != "main" {
				id = fmt.Sprintf("%d:%s", frame.ClockEpoch, id)
			}
			aggregate := threads[id]
			if aggregate == nil {
				aggregate = &threadAggregate{name: thread.Name, functions: make(map[heavyKey]*HeavySample)}
				threads[id] = aggregate
			}
			aggregate.root.add(thread.Samples, aggregate.functions)
		}
	}
	for id, thread := range threads {
		functions := make([]*HeavySample, 0, len(thread.functions))
		for _, function := range thread.functions {
			functions = append(functions, function)
		}
		sortHeavySamples(functions)
		profile.Threads = append(profile.Threads, HeavyThread{ID: id, Name: thread.name, Roots: thread.root.finish(), Functions: functions})
	}
	sort.Slice(profile.Threads, func(left, right int) bool {
		first, second := profile.Threads[left], profile.Threads[right]
		if (first.ID == "main") != (second.ID == "main") {
			return first.ID == "main"
		}
		if first.Name != second.Name {
			return first.Name < second.Name
		}
		return first.ID < second.ID
	})
	return profile, nil
}
