package unityprofiler

import (
	"fmt"
	"strings"

	"unityperf/internal/profileframe"
)

const timelineFrameHistory = 120

func (d *Decoder) timelineFor(index uint32) *profileframe.Timeline {
	if timeline := d.timelines[index]; timeline != nil {
		return timeline
	}
	if len(d.timelineOrder) >= timelineFrameHistory {
		delete(d.timelines, d.timelineOrder[0])
		d.timelineOrder = d.timelineOrder[1:]
	}
	timeline := &profileframe.Timeline{}
	d.timelines[index] = timeline
	d.timelineOrder = append(d.timelineOrder, index)
	return timeline
}

func (d *Decoder) rollWorkerFrame(index uint32, ts int64, stream *threadStream) {
	if previous := stream.workerFrame; previous != nil {
		if ts > previous.startNS && (d.frame == nil || int32(d.frame.index-previous.index) <= timelineFrameHistory) {
			if root := d.snapshotOpenStack(stream.stack, previous, ts); root != nil {
				previous.roots = append(previous.roots, root)
			}
			if len(previous.roots) > 0 {
				name := strings.TrimPrefix(d.meta.threads[stream.id], "/")
				if name == "" {
					name = fmt.Sprintf("Thread %#x", stream.id)
				}
				samples := buildHierarchy(d, previous.roots, previous.startNS)
				d.timelineFor(previous.index).SetThread(stream.id, name, previous.startNS, samples)
			}
		}
	}
	resetOpenStack(stream, ts)
	stream.workerFrame = &frameAccum{index: index, startNS: ts}
}
