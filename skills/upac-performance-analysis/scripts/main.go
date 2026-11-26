package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
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
	ScreenJPEG    []byte                    `json:"screenJpegBase64,omitempty"`
	ScreenDeltaMs *int64                    `json:"screenDeltaMs,omitempty"`
}

func main() {
	input := flag.String("file", "", "UPAC path")
	start := flag.Int("start", 0, "first zero-based decoded position")
	count := flag.Int("count", 0, "number of positions; 0 means all remaining")
	detail := flag.Bool("detail", false, "include thread sample trees")
	stats := flag.Bool("stats", false, "summarize frame times per clock epoch")
	fps := flag.Float64("fps", 0, "target FPS for -stats; omit for no budget")
	image := flag.Bool("image", false, "include matched screenshot JPEG in detail rows")
	metadataOnly := flag.Bool("metadata", false, "print capture handshake and recorded phone information")
	flag.Parse()
	if math.IsNaN(*fps) || math.IsInf(*fps, 0) || *fps < 0 || (*fps != 0 && !*stats) || (*stats && (*detail || *image)) {
		log.Fatal("use -fps with -stats and a finite positive target; -detail and -image are extraction options")
	}
	if *input == "" {
		log.Fatal("-file is required")
	}
	capture, err := upac.LoadFile(*input)
	if err != nil {
		log.Fatal(err)
	}
	if *metadataOnly {
		if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"rawChunks": len(capture.Raw), "handshake": capture.Handshake, "phone": capture.Phone, "memory": capture.Memory}); err != nil {
			log.Fatal(err)
		}
		return
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
	if *stats {
		if err := encoder.Encode(frameStats(frames[*start:end], *start, *fps)); err != nil {
			log.Fatal(err)
		}
		return
	}
	metadata := map[string]any{
		"kind": "capture", "phone": capture.Phone, "memory": capture.Memory, "handshake": capture.Handshake, "rawChunks": len(capture.Raw),
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
		if *detail || *image {
			row.Threads = frame.TimelineThreads()
			if frame.PlayerEndNS > frame.PlayerStartNS {
				if shot, delta, ok := capture.Screen.Nearest(frame.ClockEpoch, frame.PlayerEndNS); ok {
					row.ScreenDeltaMs = &delta
					if *image {
						row.ScreenJPEG = shot.JPEG
					}
				}
			}
		}
		if err := encoder.Encode(row); err != nil {
			log.Fatal(err)
		}
	}
}
