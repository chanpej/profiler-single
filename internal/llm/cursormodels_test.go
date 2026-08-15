package llm

import (
	"context"
	"errors"
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

const sampleCursorList = `Available models

auto - Auto (default)
composer-2.5 - Composer 2.5 (current)
composer-2.5-fast - Composer 2.5 Fast
cursor-grok-4.6-high - Cursor Grok 4.6
cursor-grok-4.6-high-fast - Cursor Grok 4.6 Fast
gpt-5.2 - GPT-5.2
`

func TestParseCursorModelListDropsFast(t *testing.T) {
	got := ParseCursorModelList([]byte(sampleCursorList))
	ids := make([]string, 0, len(got))
	for _, m := range got {
		ids = append(ids, m.ID)
		if strings.Contains(strings.ToLower(m.ID), "fast") {
			t.Fatalf("fast variant leaked: %+v", m)
		}
	}
	want := "auto composer-2.5 cursor-grok-4.6-high gpt-5.2"
	if strings.Join(ids, " ") != want {
		t.Fatalf("ids = %q want %q", ids, want)
	}
	if got[1].Name != "Composer 2.5 (current)" {
		t.Fatalf("name = %q", got[1].Name)
	}
}

func TestParseCursorModelListStripsParams(t *testing.T) {
	got := ParseCursorModelList([]byte("composer-2.5[effort=high,fast=false] - Composer 2.5\n"))
	if len(got) != 1 || got[0].ID != "composer-2.5" {
		t.Fatalf("got %+v", got)
	}
}

func TestModelsFromACPConfig(t *testing.T) {
	cat := acp.SessionConfigOptionCategoryModel
	ungrouped := acp.SessionConfigSelectOptionsUngrouped{
		{Name: "Composer 2.5", Value: acp.SessionConfigValueId("composer-2.5")},
		{Name: "Composer Fast", Value: acp.SessionConfigValueId("composer-2.5-fast")},
		{Name: "Grok", Value: acp.SessionConfigValueId("cursor-grok-4.6[fast=true]")},
		{Name: "GPT-5.2", Value: acp.SessionConfigValueId("gpt-5.2")},
	}
	opts := []acp.SessionConfigOption{{
		Select: &acp.SessionConfigOptionSelect{
			Category: &cat,
			Id:       acp.SessionConfigId("model"),
			Name:     "Model",
			Options:  acp.SessionConfigSelectOptions{Ungrouped: &ungrouped},
		},
	}}
	got := ModelsFromACPConfig(opts)
	if len(got) != 2 || got[0].ID != "composer-2.5" || got[1].ID != "gpt-5.2" {
		t.Fatalf("got %+v", got)
	}
}

func TestListCursorModelsPrefersCLI(t *testing.T) {
	prevList, prevACP := runCursorList, listCursorACP
	t.Cleanup(func() {
		runCursorList = prevList
		listCursorACP = prevACP
	})
	listCursorACP = func(context.Context, AgentProfile) ([]AgentModel, error) {
		t.Fatal("CLI 成功时不应回退 ACP")
		return nil, nil
	}
	runCursorList = func(context.Context, string) ([]byte, error) {
		return []byte(sampleCursorList), nil
	}
	got, err := ListCursorModels(context.Background(), AgentProfile{Command: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 3 || got[1].ID != "composer-2.5" {
		t.Fatalf("got %+v", got)
	}
}

func TestListCursorModelsFallsBackToACP(t *testing.T) {
	prevList, prevACP := runCursorList, listCursorACP
	t.Cleanup(func() {
		runCursorList = prevList
		listCursorACP = prevACP
	})
	runCursorList = func(context.Context, string) ([]byte, error) {
		return nil, errors.New("cli missing")
	}
	listCursorACP = func(context.Context, AgentProfile) ([]AgentModel, error) {
		return []AgentModel{{ID: "composer-2.5", Name: "Composer 2.5"}}, nil
	}
	got, err := ListCursorModels(context.Background(), AgentProfile{Command: "agent"})
	if err != nil || len(got) != 1 || got[0].ID != "composer-2.5" {
		t.Fatalf("got %+v err=%v", got, err)
	}
}

func TestIsCursorCLI(t *testing.T) {
	if !IsCursorCLI(AgentProfile{ID: "cursor", Command: "agent"}) {
		t.Fatal("cursor")
	}
	if IsCursorCLI(AgentProfile{ID: "codex", Command: "codex"}) {
		t.Fatal("codex is not cursor")
	}
}
