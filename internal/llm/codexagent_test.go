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
	server := &fakeCodexServer{reply: "对象池"}
	agent := newCodexACPAgent("", func() codexServer { return server })
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
	if len(server.prompts) != 1 || !strings.Contains(server.prompts[0], "诊断") {
		t.Fatalf("prompts = %#v", server.prompts)
	}
}

func TestChatSessionStartsInProcessCodex(t *testing.T) {
	server := &fakeCodexServer{reply: "in-process ok"}
	prev := createCodexServer
	t.Cleanup(func() { createCodexServer = prev })
	createCodexServer = func() codexServer { return server }

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
	if _, err := sess.Prompt(ctx, "分析 frame 30", nil); err != nil {
		t.Fatalf("follow-up Prompt: %v", err)
	}
	if server.starts != 1 || len(server.prompts) != 2 || server.prompts[0] != "hello" || server.prompts[1] != "分析 frame 30" {
		t.Fatalf("starts=%d prompts=%#v", server.starts, server.prompts)
	}
}

func TestChatSessionUpdateConfigAppliesToCodexAppServer(t *testing.T) {
	server := &fakeCodexServer{reply: "ok"}
	prev := createCodexServer
	t.Cleanup(func() { createCodexServer = prev })
	createCodexServer = func() codexServer { return server }

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
	if server.lastProfile.Model != "gpt-5.3-codex" || server.lastProfile.Effort != "high" {
		t.Fatalf("profile = %+v", server.lastProfile)
	}
}

type fakeCodexServer struct {
	starts      int
	prompts     []string
	lastProfile AgentProfile
	reply       string
	closed      bool
}

func (s *fakeCodexServer) Start(_ context.Context, _ string, profile AgentProfile) error {
	s.starts++
	s.lastProfile = profile
	return nil
}

func (s *fakeCodexServer) Prompt(_ context.Context, text string, profile AgentProfile, emit func(string)) error {
	s.prompts = append(s.prompts, text)
	s.lastProfile = profile
	if emit != nil {
		emit(s.reply)
	}
	return nil
}

func (s *fakeCodexServer) Close() {
	s.closed = true
}
