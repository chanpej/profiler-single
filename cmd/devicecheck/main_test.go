package main

import (
	"strings"
	"testing"
	"time"
)

func TestRunRequiresUSB(t *testing.T) {
	err := run("", "com.company.game", false, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "-usb") {
		t.Fatalf("err=%v, want -usb required", err)
	}
}
