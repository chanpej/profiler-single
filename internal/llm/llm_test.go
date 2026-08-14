package llm

import (
	"context"
	"errors"
	"io"
	"os"
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
