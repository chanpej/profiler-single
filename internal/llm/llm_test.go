package llm

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

// fakeAgent 是进程内的假 ACP Agent，实现 acp.Agent 接口。
// Prompt 时把收到的用户消息记录下来，并按配置流式返回回复文本。
type fakeAgent struct {
	conn       *acp.AgentSideConnection
	replyParts []string
	lastPrompt string
}

func (a *fakeAgent) Initialize(ctx context.Context, p acp.InitializeRequest) (acp.InitializeResponse, error) {
	return acp.InitializeResponse{
		ProtocolVersion: acp.ProtocolVersionNumber,
		AgentInfo:       &acp.Implementation{Name: "fake-agent", Version: "0.0.1"},
		AuthMethods: []acp.AuthMethod{
			{Agent: &acp.AuthMethodAgent{Id: "cursor_login", Name: "Cursor Login"}},
		},
	}, nil
}

func (a *fakeAgent) NewSession(ctx context.Context, p acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	return acp.NewSessionResponse{SessionId: acp.SessionId("sess-1")}, nil
}

func (a *fakeAgent) Prompt(ctx context.Context, p acp.PromptRequest) (acp.PromptResponse, error) {
	if len(p.Prompt) > 0 && p.Prompt[0].Text != nil {
		a.lastPrompt = p.Prompt[0].Text.Text
	}
	for _, part := range a.replyParts {
		if err := a.conn.SessionUpdate(ctx, acp.SessionNotification{
			SessionId: p.SessionId,
			Update:    acp.UpdateAgentMessageText(part),
		}); err != nil {
			return acp.PromptResponse{}, err
		}
	}
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
}

func (a *fakeAgent) Authenticate(ctx context.Context, p acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, nil
}
func (a *fakeAgent) Logout(ctx context.Context, p acp.LogoutRequest) (acp.LogoutResponse, error) {
	return acp.LogoutResponse{}, nil
}
func (a *fakeAgent) Cancel(ctx context.Context, p acp.CancelNotification) error { return nil }
func (a *fakeAgent) CloseSession(ctx context.Context, p acp.CloseSessionRequest) (acp.CloseSessionResponse, error) {
	return acp.CloseSessionResponse{}, nil
}
func (a *fakeAgent) ListSessions(ctx context.Context, p acp.ListSessionsRequest) (acp.ListSessionsResponse, error) {
	return acp.ListSessionsResponse{}, nil
}
func (a *fakeAgent) ResumeSession(ctx context.Context, p acp.ResumeSessionRequest) (acp.ResumeSessionResponse, error) {
	return acp.ResumeSessionResponse{}, nil
}
func (a *fakeAgent) SetSessionConfigOption(ctx context.Context, p acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	return acp.SetSessionConfigOptionResponse{}, nil
}
func (a *fakeAgent) SetSessionMode(ctx context.Context, p acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
	return acp.SetSessionModeResponse{}, nil
}

const fakeAgentEnv = "UPA_FAKE_ACP_AGENT"

// TestMain 支持把测试二进制自身作为 ACP Agent 子进程运行，
// 用于覆盖 Diagnose 真实拉起子进程的完整链路。
func TestMain(m *testing.M) {
	if os.Getenv(fakeAgentEnv) == "1" {
		agent := &fakeAgent{replyParts: []string{"诊断结论：", "GC 分配过高，建议使用对象池。"}}
		conn := acp.NewAgentSideConnection(agent, os.Stdout, os.Stdin)
		agent.conn = conn
		<-conn.Done()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// startInProcAgent 用内存管道把假 Agent 与客户端对接。
func startInProcAgent(t *testing.T, agent *fakeAgent) (io.Writer, io.Reader) {
	t.Helper()
	clientToAgentR, clientToAgentW := io.Pipe()
	agentToClientR, agentToClientW := io.Pipe()
	conn := acp.NewAgentSideConnection(agent, agentToClientW, clientToAgentR)
	agent.conn = conn
	t.Cleanup(func() {
		clientToAgentW.Close()
		agentToClientW.Close()
	})
	return clientToAgentW, agentToClientR
}

func TestDiagnoseOverInProcAgent(t *testing.T) {
	agent := &fakeAgent{replyParts: []string{"诊断结论：", "DrawCall 过高，建议合批。"}}
	w, r := startInProcAgent(t, agent)

	c := NewClient(Config{AgentCommand: "in-proc"})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := c.diagnoseOver(ctx, w, r, `{"frameCount":100,"issues":[]}`)
	if err != nil {
		t.Fatalf("diagnoseOver: %v", err)
	}
	if out != "诊断结论：DrawCall 过高，建议合批。" {
		t.Fatalf("out = %q", out)
	}
	if !strings.Contains(agent.lastPrompt, `"frameCount":100`) {
		t.Fatalf("report not forwarded to agent: %q", agent.lastPrompt)
	}
	if !strings.Contains(agent.lastPrompt, "Unity 2022") {
		t.Fatalf("prompt should pin Unity 2022: %q", agent.lastPrompt)
	}
}

func TestDiagnoseOverEmptyReply(t *testing.T) {
	agent := &fakeAgent{replyParts: nil}
	w, r := startInProcAgent(t, agent)

	c := NewClient(Config{AgentCommand: "in-proc"})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := c.diagnoseOver(ctx, w, r, "{}"); err == nil {
		t.Fatal("expected error for empty agent reply")
	}
}

func TestDiagnoseNotConfigured(t *testing.T) {
	c := NewClient(Config{})
	if _, err := c.Diagnose(context.Background(), "{}"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
	if c.Configured() {
		t.Fatal("Configured should be false")
	}
	if NewClient(Config{AgentCommand: "   "}).Configured() {
		t.Fatal("blank command should not count as configured")
	}
}

func TestDiagnoseSpawnsAgentProcess(t *testing.T) {
	// 把测试二进制自身作为 ACP Agent 子进程拉起（见 TestMain）。
	t.Setenv(fakeAgentEnv, "1")

	c := NewClient(Config{AgentCommand: os.Args[0]})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := c.Diagnose(ctx, `{"frameCount":42}`)
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if !strings.Contains(out, "对象池") {
		t.Fatalf("out = %q", out)
	}
}

func TestDiagnoseBadCommand(t *testing.T) {
	c := NewClient(Config{AgentCommand: "definitely-not-a-real-command-upa"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.Diagnose(ctx, "{}"); err == nil {
		t.Fatal("expected error for missing agent command")
	}
}

func TestChatSessionPromptOverInProcAgent(t *testing.T) {
	agent := &fakeAgent{replyParts: []string{"你好，", "这是回复。"}}
	w, r := startInProcAgent(t, agent)

	sess := NewChatSession(Config{AgentCommand: "in-proc"})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sess.startOver(ctx, w, r); err != nil {
		t.Fatalf("startOver: %v", err)
	}

	var chunks []string
	out, err := sess.Prompt(ctx, "卡顿原因？", func(c string) { chunks = append(chunks, c) })
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if out != "你好，这是回复。" {
		t.Fatalf("out = %q", out)
	}
	if strings.Join(chunks, "") != out {
		t.Fatalf("chunks = %q, out = %q", chunks, out)
	}
	if !strings.Contains(agent.lastPrompt, "Unity 2022") {
		t.Fatalf("first prompt should include system hint: %q", agent.lastPrompt)
	}

	agent.replyParts = []string{"第二轮。"}
	out2, err := sess.Prompt(ctx, "还有呢？", nil)
	if err != nil {
		t.Fatalf("second Prompt: %v", err)
	}
	if out2 != "第二轮。" {
		t.Fatalf("out2 = %q", out2)
	}
	if strings.Contains(agent.lastPrompt, chatSystemHint) {
		t.Fatalf("second prompt should not repeat system hint: %q", agent.lastPrompt)
	}
}

func TestChatSessionStartNotConfigured(t *testing.T) {
	sess := NewChatSession(Config{})
	if err := sess.Start(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

func TestChatSessionPromptBeforeStart(t *testing.T) {
	sess := NewChatSession(Config{AgentCommand: "x"})
	if _, err := sess.Prompt(context.Background(), "hi", nil); err == nil {
		t.Fatal("expected error before Start")
	}
}

func TestChatSessionEmptyPrompt(t *testing.T) {
	sess := NewChatSession(Config{AgentCommand: "x"})
	if _, err := sess.Prompt(context.Background(), "  ", nil); err == nil {
		t.Fatal("expected error for empty prompt")
	}
}

func TestChatSessionSpawnsAgentProcess(t *testing.T) {
	t.Setenv(fakeAgentEnv, "1")

	sess := NewChatSession(Config{AgentCommand: os.Args[0]})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := sess.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sess.Close()

	out, err := sess.Prompt(ctx, "请诊断", nil)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if !strings.Contains(out, "对象池") {
		t.Fatalf("out = %q", out)
	}
}

func TestDefaultAgentsOfficialACP(t *testing.T) {
	agents := DefaultAgents()
	if len(agents) != 2 {
		t.Fatalf("len = %d", len(agents))
	}
	if agents[0].ID != "codex" || agents[0].Command != "codex" {
		t.Fatalf("codex = %+v", agents[0])
	}
	if len(agents[0].Args) != 0 {
		t.Fatalf("codex args = %v", agents[0].Args)
	}
	if agents[1].ID != "cursor" || agents[1].Command != "agent" || strings.Join(agents[1].Args, " ") != "acp" {
		t.Fatalf("cursor = %+v", agents[1])
	}
}

func TestNormalizePreservesLegacyAndDefaults(t *testing.T) {
	got := Config{AgentCommand: "gemini", AgentArgs: []string{"--experimental-acp"}}.Normalize()
	if got.ActiveID != "custom" || got.AgentCommand != "gemini" {
		t.Fatalf("got = %+v", got)
	}
	if len(got.Agents) != 3 {
		t.Fatalf("agents = %d", len(got.Agents))
	}
	codex := got.WithActive("codex")
	if codex.AgentCommand != "codex" {
		t.Fatalf("switch codex command = %q", codex.AgentCommand)
	}
}

func TestProbeMissingCommand(t *testing.T) {
	r := Probe(context.Background(), Config{AgentCommand: "definitely-not-a-real-command-upa"})
	if r.OK {
		t.Fatal("expected probe failure")
	}
	if !strings.Contains(r.Message, "找不到可执行文件") {
		t.Fatalf("msg = %q", r.Message)
	}
}

func TestProbeEmptyCommand(t *testing.T) {
	r := Probe(context.Background(), Config{Agents: []AgentProfile{{ID: "x", Name: "x"}}})
	if r.OK || !strings.Contains(r.Message, "未配置") {
		t.Fatalf("probe = %+v", r)
	}
}

func TestProbeCancelledContextFailsFast(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := Probe(ctx, Config{AgentCommand: os.Args[0]})
	if r.OK {
		t.Fatalf("probe = %+v", r)
	}
}

func TestProbeCodexGoAdapter(t *testing.T) {
	if _, err := exec.LookPath("codex"); err != nil {
		t.Skip("PATH 上没有 codex")
	}
	prev := runCodexExec
	t.Cleanup(func() { runCodexExec = prev })
	runCodexExec = func(context.Context, string, string, AgentProfile, func(string)) error {
		t.Fatal("probe 不应调用 codex exec")
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r := Probe(ctx, Config{ActiveID: "codex", Agents: DefaultAgents()})
	if !r.OK {
		t.Fatalf("codex probe: %+v", r)
	}
}

func TestProbeSpawnsAgentProcess(t *testing.T) {
	t.Setenv(fakeAgentEnv, "1")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r := Probe(ctx, Config{AgentCommand: os.Args[0]})
	if !r.OK {
		t.Fatalf("probe: %+v", r)
	}
}
