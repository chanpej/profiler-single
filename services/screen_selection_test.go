package services

import (
	"context"
	"strings"
	"testing"
	"unityperf/internal/adbforward"
)

func TestScreenSelectionMatchesPlayer(t *testing.T) {
	const ip = "192.168.1.5"
	tests := []struct {
		name, devices, want string
		addresses           map[string]string
	}{
		{"usb and wifi on same phone", "USB123 device\n192.168.1.5:38811 device", "192.168.1.5:38811", nil},
		{"one usb connection", "USB123 device", "USB123", nil},
		{"several phones", "USB123 device\nUSB987 device", "USB987", map[string]string{"USB123": "192.168.1.3", "USB987": ip}},
		{"no match", "USB123 device\nUSB987 device", "", nil},
		{"ambiguous IP", "USB123 device\nUSB987 device", "", map[string]string{"USB123": ip, "USB987": ip}},
		{"no phone", "", "", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var run adbforward.Runner = func(_ context.Context, args ...string) (string, error) {
				if args[0] == "devices" {
					return tc.devices, nil
				}
				return strings.Repeat("\n", 6) + "9: wlan0 inet " + tc.addresses[args[1]] + "/24", nil
			}
			got, err := selectScreenDevice(context.Background(), run, ip)
			if got != tc.want || (err != nil) != (tc.want == "") {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
}
