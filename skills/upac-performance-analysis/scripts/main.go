package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"unityperf/internal/protocol"
	"unityperf/internal/unityprofiler"
	"unityperf/internal/upac"
)

type frameRow struct {
	Position      int                       `json:"position"`
	ClockEpoch    int                       `json:"clockEpoch"`
	PlayerStartNS string                    `json:"playerStartNs"`
	PlayerEndNS   string                    `json:"playerEndNs"`
	FrameIndex    string                    `json:"frameIndex"`
	Metrics       protocol.FrameData        `json:"metrics"`
	Threads       []protocol.TimelineThread `json:"threads,omitempty"`
	ScreenDeltaMs *int64                    `json:"screenDeltaMs,omitempty"`
}

func main() {
	input := flag.String("file", "", "UPAC path")
	start := flag.Int("start", 0, "first zero-based decoded position")
	count := flag.Int("count", 0, "number of positions; 0 means all remaining")
	detail := flag.Bool("detail", false, "include thread sample trees")
	flag.Parse()
	if *input == "" {
		log.Fatal("-file is required")
	}
	capture, err := upac.LoadFile(*input)
	if err != nil {
		log.Fatal(err)
	}
	frames, err := unityprofiler.ReplayFrames(capture.Raw)
	if err != nil {
		log.Fatal(err)
	}
	if *start < 0 || *start >= len(frames) || *count < 0 || *count > len(frames)-*start {
		log.Fatalf("invalid range for %d decoded frames", len(frames))
	}
	end := len(frames)
	if *count > 0 {
		end = *start + *count
	}
	encoder := json.NewEncoder(os.Stdout)
	metadata := map[string]any{
		"kind": "capture", "handshake": capture.Handshake, "rawChunks": len(capture.Raw),
		"decodedFrames": len(frames), "screenShots": len(capture.Screen.Shots),
		"start": *start, "endExclusive": end,
	}
	if err := encoder.Encode(metadata); err != nil {
		log.Fatal(err)
	}
	for position := *start; position < end; position++ {
		frame := frames[position]
		row := frameRow{
			Position: position, ClockEpoch: frame.ClockEpoch, Metrics: frame.Summary,
			PlayerStartNS: fmt.Sprint(frame.PlayerStartNS), PlayerEndNS: fmt.Sprint(frame.PlayerEndNS),
			FrameIndex: fmt.Sprint(frame.Summary.FrameIndex),
		}
		if *detail {
			row.Threads = frame.TimelineThreads()
			if frame.PlayerEndNS > frame.PlayerStartNS {
				if _, delta, ok := capture.Screen.Nearest(frame.ClockEpoch, frame.PlayerEndNS); ok {
					row.ScreenDeltaMs = &delta
				}
			}
		}
		if err := encoder.Encode(row); err != nil {
			log.Fatal(err)
		}
	}
}
