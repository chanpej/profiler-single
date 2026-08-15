package llm

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf16"
)

const sampleCatalog = `{
  "models": [
    {"slug":"gpt-5.6-sol","display_name":"GPT-5.6-Sol","visibility":"list"},
    {"slug":"gpt-5.4","display_name":"GPT-5.4","visibility":"list"},
    {"slug":"codex-auto-review","display_name":"Codex Auto Review","visibility":"hide"}
  ]
}`

func TestParseCodexModelCatalog(t *testing.T) {
	got, err := ParseCodexModelCatalog([]byte(sampleCatalog))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (hide filtered): %+v", len(got), got)
	}
	if got[0].ID != "gpt-5.6-sol" || got[0].Name != "GPT-5.6-Sol" {
		t.Fatalf("first = %+v", got[0])
	}
	if got[1].ID != "gpt-5.4" {
		t.Fatalf("second = %+v", got[1])
	}
}

func TestParseCodexModelCatalogArrayAndBOM(t *testing.T) {
	got, err := ParseCodexModelCatalog([]byte(`[{"slug":"o3","name":"o3","visibility":"list"}]`))
	if err != nil || len(got) != 1 || got[0].ID != "o3" {
		t.Fatalf("array = %+v err=%v", got, err)
	}
	u16 := utf16.Encode([]rune(sampleCatalog))
	buf := []byte{0xFF, 0xFE}
	for _, c := range u16 {
		buf = append(buf, byte(c), byte(c>>8))
	}
	got, err = ParseCodexModelCatalog(buf)
	if err != nil || len(got) != 2 {
		t.Fatalf("utf16 = %+v err=%v", got, err)
	}
	prefixed := bytes.Join([][]byte{[]byte("log line\n"), []byte(sampleCatalog), []byte("\n")}, nil)
	got, err = ParseCodexModelCatalog(prefixed)
	if err != nil || len(got) != 2 {
		t.Fatalf("prefixed = %+v err=%v", got, err)
	}
}

func TestListCodexModelsPrefersMergedThenBundled(t *testing.T) {
	prev := runCodexDebug
	t.Cleanup(func() { runCodexDebug = prev })
	var calls []bool
	runCodexDebug = func(_ context.Context, _ string, bundled bool) ([]byte, error) {
		calls = append(calls, bundled)
		if !bundled {
			return nil, errors.New("network")
		}
		return []byte(sampleCatalog), nil
	}
	got, err := ListCodexModels(context.Background(), AgentProfile{Command: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "gpt-5.6-sol" {
		t.Fatalf("got %+v", got)
	}
	if len(calls) != 2 || calls[0] || !calls[1] {
		t.Fatalf("calls = %v", calls)
	}
}

func TestListCodexModelsUsesMergedWhenValid(t *testing.T) {
	prev := runCodexDebug
	t.Cleanup(func() { runCodexDebug = prev })
	runCodexDebug = func(_ context.Context, _ string, bundled bool) ([]byte, error) {
		if bundled {
			t.Fatal("should not fall back")
		}
		return []byte(sampleCatalog), nil
	}
	got, err := ListCodexModels(context.Background(), AgentProfile{Command: "codex"})
	if err != nil || len(got) != 2 {
		t.Fatalf("got %+v err=%v", got, err)
	}
}

func TestParseCodexModelCatalogEmpty(t *testing.T) {
	if _, err := ParseCodexModelCatalog([]byte(`{"models":[]}`)); err == nil {
		t.Fatal("expected error")
	}
	if _, err := ParseCodexModelCatalog(nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestIsCodexCLI(t *testing.T) {
	if !IsCodexCLI(AgentProfile{ID: "codex", Command: "codex"}) {
		t.Fatal("codex id")
	}
	if IsCodexCLI(AgentProfile{ID: "cursor", Command: "agent"}) {
		t.Fatal("cursor is not codex")
	}
	if strings.Contains(sampleCatalog, "gpt-5.3-codex") {
		t.Fatal("sample must not rely on hardcoded old slugs")
	}
}
