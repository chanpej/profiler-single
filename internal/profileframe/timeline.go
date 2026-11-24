package profileframe

import (
	"sort"
	"strconv"
	"sync"

	"unityperf/internal/protocol"
)

type threadSamples struct {
	name    string
	startNS int64
	samples []protocol.SampleNode
}

type Timeline struct {
	mu      sync.RWMutex
	threads map[int64]threadSamples
}

func (timeline *Timeline) SetThread(id int64, name string, startNS int64, samples []protocol.SampleNode) {
	timeline.mu.Lock()
	defer timeline.mu.Unlock()
	if timeline.threads == nil {
		timeline.threads = make(map[int64]threadSamples)
	}
	timeline.threads[id] = threadSamples{name: name, startNS: startNS, samples: samples}
}

func (frame Frame) TimelineThreads() []protocol.TimelineThread {
	var threads []protocol.TimelineThread
	if len(frame.MainThread) > 0 {
		threads = append(threads, protocol.TimelineThread{ID: "main", Name: "Main Thread", Samples: frame.MainThread})
	}
	if frame.Timeline == nil {
		return threads
	}
	frame.Timeline.mu.RLock()
	defer frame.Timeline.mu.RUnlock()
	firstWorker := len(threads)
	for id, thread := range frame.Timeline.threads {
		threads = append(threads, protocol.TimelineThread{
			ID: strconv.FormatInt(id, 10), Name: thread.name,
			OffsetMs: float64(thread.startNS-frame.PlayerStartNS) / 1e6, Samples: thread.samples,
		})
	}
	workers := threads[firstWorker:]
	sort.Slice(workers, func(left, right int) bool {
		if workers[left].Name == workers[right].Name {
			return workers[left].ID < workers[right].ID
		}
		return workers[left].Name < workers[right].Name
	})
	return threads
}
