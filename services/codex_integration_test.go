package services

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"unityperfagent/internal/llm"
	"unityperfagent/internal/mcp"
	"unityperfagent/internal/procutil"
)

const (
	mcpStandaloneTestEnv = "UPA_MCP_STANDALONE"
	codexIntegrationEnv  = "UPA_RUN_CODEX_INTEGRATION"
)

// TestMain makes a child copy of this test binary serve the stdio MCP transport
// when Codex launches it. Normal test runs do not set this environment flag.
func TestMain(m *testing.M) {
	if (len(os.Args) == 2 && os.Args[1] == "mcp") || os.Getenv(mcpStandaloneTestEnv) == "1" {
		err := mcp.Serve(context.Background(), os.Stdin, os.Stdout,
			os.Getenv("UNITY_PERF_MCP_ENDPOINT"), os.Getenv("UNITY_PERF_MCP_TOKEN"))
		if err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// TestCodexAnalyzesProfilerFixture invokes `codex exec` directly (without the
// ACP client/session layer) and verifies its native MCP call against the supplied
// capture. It is opt-in because it requires an authenticated Codex account and
// may consume API quota.
func TestCodexChatStreamsMCPProgress(t *testing.T) {
	if os.Getenv(codexIntegrationEnv) != "1" {
		t.Skip("set UPA_RUN_CODEX_INTEGRATION=1 to run authenticated Codex integration")
	}
	if _, err := exec.LookPath("codex"); err != nil {
		t.Skip("codex is not on PATH")
	}
	svc := newTestService()
	defer svc.AgentChatClose()
	defer svc.mcp.close()
	if _, err := svc.ImportCapture(filepath.Join("..", "test", "testdata", "perfagent-profiler.data")); err != nil {
		t.Fatalf("ImportCapture: %v", err)
	}
	svc.SetLLMConfig(llm.Config{
		ActiveID: "codex",
		Agents:   []llm.AgentProfile{{ID: "codex", Name: "Codex", Command: "codex"}},
	})
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	out, err := svc.AgentChatSend(ctx, "分析当前性能数据，给出优化建议。")
	if err != nil {
		t.Fatalf("AgentChatSend: %v", err)
	}
	for _, expected := range []string{"【MCP 工具】", "建议"} {
		if !strings.Contains(out, expected) {
			t.Fatalf("chat output missing %q: %s", expected, out)
		}
	}
}

func TestCodexAnalyzesProfilerFixture(t *testing.T) {
	if os.Getenv(codexIntegrationEnv) != "1" {
		t.Skip("set UPA_RUN_CODEX_INTEGRATION=1 to run authenticated Codex integration")
	}
	if _, err := exec.LookPath("codex"); err != nil {
		t.Skip("codex is not on PATH")
	}
	svc := newTestService()
	defer svc.mcp.close()
	if _, err := svc.ImportCapture(filepath.Join("..", "test", "testdata", "perfagent-profiler.data")); err != nil {
		t.Fatalf("ImportCapture: %v", err)
	}
	t.Setenv(mcpStandaloneTestEnv, "1")
	mcpServer, err := svc.mcp.config()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	args := llm.CodexExecArgsWithMCP(
		"立即调用 performance_session_summary 分析当前性能数据，并给出性能优化建议。不要回复计划，必须实际调用工具。",
		llm.AgentProfile{}, []llm.MCPServer{mcpServer},
	)
	cmd := procutil.CommandContext(ctx, "codex", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("codex exec failed: %v\noutput=%s\nlogs=%+v", err, out, svc.AgentLogs())
	}
	if !strings.Contains(string(out), "建议") {
		t.Fatalf("Codex did not provide optimization advice: %s", out)
	}
	var calledTool, receivedToolResult bool
	for _, entry := range svc.AgentLogs() {
		if entry.Source != "MCP" || entry.Method != "tools/call" {
			continue
		}
		if entry.Direction == "Agent → MCP" {
			calledTool = true
		}
		if entry.Direction == "MCP → Agent" && strings.Contains(entry.Payload, `"content"`) {
			receivedToolResult = true
		}
	}
	if !calledTool || !receivedToolResult {
		t.Fatalf("Codex MCP exchange incomplete: called=%t result=%t output=%s logs=%+v", calledTool, receivedToolResult, out, svc.AgentLogs())
	}
}
