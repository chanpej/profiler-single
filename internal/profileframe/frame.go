// Package profileframe defines the common in-memory model consumed by the
// Profiler UI. Both the live PlayerConnection stream and offline .data captures
// are normalized into Frame before reaching the session store.
package profileframe

import "unityperfagent/internal/protocol"

// Frame is one fully decoded Unity Profiler frame.
//
// Summary carries the module counters exposed to the frame list and charts.
// MainThread contains the unmerged CPU sample forest for the Main Thread; the
// frontend performs Unity-style hierarchy merging only when rendering it.
type Frame struct {
	Summary    protocol.FrameData
	MainThread []protocol.SampleNode
}

// New combines a frame summary and its Main Thread samples into the common
// in-memory representation.
func New(summary protocol.FrameData, mainThread []protocol.SampleNode) Frame {
	return Frame{Summary: summary, MainThread: mainThread}
}
