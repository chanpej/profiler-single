package llm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveLaunchMissing(t *testing.T) {
	_, _, err := ResolveLaunch("definitely-not-a-real-command-upa", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveLaunchLookPath(t *testing.T) {
	goBin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	got, args, err := ResolveLaunch(goBin, []string{"acp"})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != filepath.Base(goBin) {
		t.Fatalf("got %q want %q", got, goBin)
	}
	if len(args) != 1 || args[0] != "acp" {
		t.Fatalf("args = %v", args)
	}
}

func TestPreferAuthID(t *testing.T) {
	if got := preferAuthID([]string{"other", "cursor_login", "x"}); got != "cursor_login" {
		t.Fatalf("got %q", got)
	}
	if got := preferAuthID([]string{"only"}); got != "only" {
		t.Fatalf("got %q", got)
	}
	if preferAuthID(nil) != "" {
		t.Fatal("empty")
	}
}

func TestUseInProcessCodex(t *testing.T) {
	if !useInProcessCodex(AgentProfile{Command: "codex"}) {
		t.Fatal("codex should use in-process adapter")
	}
	if !useInProcessCodex(AgentProfile{Command: `C:\npm\codex.cmd`}) {
		t.Fatal("codex.cmd should use in-process adapter")
	}
	if useInProcessCodex(AgentProfile{Command: "agent", Args: []string{"acp"}}) {
		t.Fatal("cursor stays external ACP")
	}
	if useInProcessCodex(AgentProfile{Command: "npx", Args: []string{"-y", "@agentclientprotocol/codex-acp"}}) {
		t.Fatal("legacy npx adapter stays external")
	}
}
