package llm

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

func TestExtractCodexTexts(t *testing.T) {
	got := extractCodexTexts([]byte(`{"type":"item.completed","item":{"type":"agent_message","text":"你好"}}`))
	if strings.Join(got, "") != "你好" {
		t.Fatalf("got %v", got)
	}
	got = extractCodexTexts([]byte(`{"type":"agent_message_delta","delta":"Hel"}`))
	if strings.Join(got, "") != "Hel" {
		t.Fatalf("delta %v", got)
	}
	if extractCodexTexts([]byte(`{"type":"thread.started","thread_id":"x"}`)) != nil {
		t.Fatal("ignore thread.started")
	}
	if extractCodexTexts([]byte(`not json`)) != nil {
		t.Fatal("ignore plain")
	}
}

func TestExtractCodexTextsIncludesReasoningAndMCPProgress(t *testing.T) {
	reasoning := extractCodexTexts([]byte(`{"type":"item.completed","item":{"type":"reasoning","text":"先确认会话摘要。"}}`))
	if got := strings.Join(reasoning, ""); !strings.Contains(got, "【思考摘要】先确认会话摘要。") {
		t.Fatalf("reasoning = %q", got)
	}
	started := extractCodexTexts([]byte(`{"type":"item.started","item":{"type":"mcp_tool_call","server":"unity-perf-data","tool":"performance_session_summary"}}`))
	if got := strings.Join(started, ""); !strings.Contains(got, "【MCP 工具】正在调用 unity-perf-data.performance_session_summary。") {
		t.Fatalf("started = %q", got)
	}
	completed := extractCodexTexts([]byte(`{"type":"item.completed","item":{"type":"mcp_tool_call","server":"unity-perf-data","tool":"performance_session_summary"}}`))
	if got := strings.Join(completed, ""); !strings.Contains(got, "【MCP 工具】unity-perf-data.performance_session_summary 已返回结果。") {
		t.Fatalf("completed = %q", got)
	}
}

func TestCodexAdapterHandshakeAndPrompt(t *testing.T) {
	run := func(_ context.Context, _, prompt string, _ AgentProfile, emit func(string)) error {
		if !strings.Contains(prompt, "诊断") {
			t.Fatalf("prompt = %q", prompt)
		}
		emit("对象池")
		return nil
	}
	agent := newCodexACPAgent("", run)
	clientToAgentR, clientToAgentW := io.Pipe()
	agentToClientR, agentToClientW := io.Pipe()
	conn := acp.NewAgentSideConnection(agent, agentToClientW, clientToAgentR)
	agent.setConn(conn)
	t.Cleanup(func() {
		_ = clientToAgentW.Close()
		_ = agentToClientW.Close()
	})

	c := NewClient(Config{AgentCommand: "codex"})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := c.diagnoseOver(ctx, clientToAgentW, agentToClientR, `{"issues":["gc"]}`)
	if err != nil {
		t.Fatalf("diagnoseOver: %v", err)
	}
	if !strings.Contains(out, "对象池") {
		t.Fatalf("out = %q", out)
	}
}

func TestChatSessionStartsInProcessCodex(t *testing.T) {
	prev := runCodexExec
	t.Cleanup(func() { runCodexExec = prev })
	runCodexExec = func(_ context.Context, _, _ string, _ AgentProfile, emit func(string)) error {
		emit("in-process ok")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sess := NewChatSession(Config{AgentCommand: "codex"})
	if err := sess.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sess.Close()
	out, err := sess.Prompt(ctx, "hello", nil)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if !strings.Contains(out, "in-process ok") {
		t.Fatalf("out = %q", out)
	}
}

func TestChatSessionUpdateConfigAppliesToCodexExec(t *testing.T) {
	prev := runCodexExec
	t.Cleanup(func() { runCodexExec = prev })
	var got AgentProfile
	runCodexExec = func(_ context.Context, _, _ string, p AgentProfile, emit func(string)) error {
		got = p
		emit("ok")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sess := NewChatSession(Config{AgentCommand: "codex"})
	if err := sess.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sess.Close()
	sess.UpdateConfig(Config{
		ActiveID: "codex",
		Agents:   []AgentProfile{{ID: "codex", Command: "codex", Model: "gpt-5.3-codex", Effort: "high"}},
	})
	if _, err := sess.Prompt(ctx, "hello", nil); err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if got.Model != "gpt-5.3-codex" || got.Effort != "high" {
		t.Fatalf("profile = %+v", got)
	}
}
