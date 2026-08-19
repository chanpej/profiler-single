package llm

import (
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

func TestCursorModelSpecDisablesFast(t *testing.T) {
	got := CursorModelSpec("composer-2.5", "high")
	if got != "composer-2.5[effort=high,fast=false]" {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(got, "fast=true") {
		t.Fatal("fast must stay disabled")
	}
	if CursorModelSpec("  ", "high") != "" {
		t.Fatal("empty model should skip spec")
	}
	if got := CursorModelSpec("grok-4.6", "nope"); !strings.Contains(got, "effort=medium") {
		t.Fatalf("invalid effort should fall back: %q", got)
	}
}

func TestCursorExecArgsInjectsModelBeforeACP(t *testing.T) {
	p := AgentProfile{
		ID:      "cursor",
		Command: "agent",
		Args:    []string{"acp"},
		Model:   "composer-2.5",
		Effort:  "high",
	}
	got := p.ExecArgs()
	want := []string{"--model", "composer-2.5[effort=high,fast=false]", "acp"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("got %v want %v", got, want)
	}
	if strings.Join(p.Args, " ") != "acp" {
		t.Fatal("stored args must stay unchanged")
	}
}

func TestCursorExecArgsReplacesExistingModel(t *testing.T) {
	p := AgentProfile{
		Command: "cursor-agent",
		Args:    []string{"--model", "old", "acp"},
		Model:   "gpt-5.4",
		Effort:  "low",
	}
	got := strings.Join(p.ExecArgs(), " ")
	if got != "--model gpt-5.4[effort=low,fast=false] acp" {
		t.Fatalf("got %q", got)
	}
}

func TestCodexExecArgsNeverUsesFast(t *testing.T) {
	p := AgentProfile{Model: "gpt-5.3-codex", Effort: "xhigh"}
	got := CodexExecArgs("hello", p)
	joined := strings.Join(got, " ")
	if strings.Contains(joined, "--fast") {
		t.Fatalf("codex must not enable fast: %v", got)
	}
	if !strings.Contains(joined, "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatalf("Codex must receive the user-approved full permission flag: %q", joined)
	}
	if joined != "exec --json --skip-git-repo-check --ephemeral --color never --dangerously-bypass-approvals-and-sandbox --model gpt-5.3-codex -c model_reasoning_effort=xhigh hello" {
		t.Fatalf("got %q", joined)
	}
}

func TestCodexExecArgsIncludesSessionMCP(t *testing.T) {
	p := AgentProfile{mcpServers: []acp.McpServer{{Stdio: &acp.McpServerStdio{
		Name: "unity-perf-data", Command: "UnityPerfAgent.exe", Args: []string{"mcp"},
		Env: []acp.EnvVariable{{Name: "UNITY_PERF_MCP_TOKEN", Value: "secret"}},
	}}}}
	joined := strings.Join(CodexExecArgs("analyze", p), " ")
	for _, expected := range []string{"mcp_servers.unity-perf-data.command=\"UnityPerfAgent.exe\"", "mcp_servers.unity-perf-data.args=[\"mcp\"]", "UNITY_PERF_MCP_TOKEN=\"secret\""} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing %q in %q", expected, joined)
		}
	}
}

func TestShouldRestartChat(t *testing.T) {
	codex := Config{ActiveID: "codex", Agents: DefaultAgents()}.Normalize()
	sameModel := codex
	sameModel.Agents[0].Model = "gpt-5.3-codex"
	if ShouldRestartChat(codex, sameModel) {
		t.Fatal("in-process Codex model change should not restart")
	}
	cursor := Config{ActiveID: "cursor", Agents: DefaultAgents()}.Normalize()
	next := cursor
	next.Agents = append([]AgentProfile{}, cursor.Agents...)
	next.Agents[1].Model = "composer-2.5"
	if !ShouldRestartChat(cursor, next) {
		t.Fatal("Cursor model change should restart")
	}
	if !ShouldRestartChat(codex, cursor) {
		t.Fatal("switching agent should restart")
	}
}
