// profilerproxy transparently relays a Unity Editor Profiler connection and
// records the Player-to-Editor PD3U messages for offline .data research.
//
// Usage:
//
//	go run ./cmd/profilerproxy -listen 127.0.0.1:55018 -target 127.0.0.1:55000 -out session.uppc
//
// In Unity Editor, use Profiler > Attach to Player > Enter IP and enter the
// listen address. Save the same Profiler session as a .data file before
// disconnecting. The .uppc and .data files then describe the same session.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"unityperfagent/internal/profilerproxy"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:55018", "proxy listen address for Unity Editor")
	target := flag.String("target", "", "Unity Player address, usually an adb-forwarded local port")
	output := flag.String("out", "profiler-session.uppc", "output PD3U capture path")
	flag.Parse()
	if *target == "" {
		fmt.Fprintln(os.Stderr, "-target is required, for example 127.0.0.1:55000")
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Printf("Profiler proxy listening at %s\n", *listen)
	fmt.Printf("Target Player: %s\n", *target)
	fmt.Printf("In Unity Editor, choose Attach to Player > Enter IP and enter %s.\n", *listen)
	fmt.Printf("PD3U capture will be written to %s when the Editor disconnects.\n", *output)
	if err := profilerproxy.Run(ctx, profilerproxy.Config{
		ListenAddr:  *listen,
		TargetAddr:  *target,
		CapturePath: *output,
	}); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "profiler proxy:", err)
		os.Exit(1)
	}
}
