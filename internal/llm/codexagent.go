package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"

	acp "github.com/coder/acp-go-sdk"

	"unityperfagent/internal/procutil"
)

// useInProcessCodex 表示用 acp-go-sdk 在进程内适配 Codex CLI（codex exec），
// 不再经过 npx / @agentclientprotocol/codex-acp。
func useInProcessCodex(p AgentProfile) bool {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(p.Command)))
	base = strings.TrimSuffix(base, ".exe")
	base = strings.TrimSuffix(base, ".cmd")
	base = strings.TrimSuffix(base, ".bat")
	if base != "codex" {
		return false
	}
	for _, a := range p.Args {
		al := strings.ToLower(a)
		if al == "acp" || strings.Contains(al, "codex-acp") {
			return false
		}
	}
	return true
}

type stubACPAgent struct{}

func (stubACPAgent) Authenticate(context.Context, acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, nil
}
func (stubACPAgent) Logout(context.Context, acp.LogoutRequest) (acp.LogoutResponse, error) {
	return acp.LogoutResponse{}, nil
}
func (stubACPAgent) Cancel(context.Context, acp.CancelNotification) error { return nil }
func (stubACPAgent) CloseSession(context.Context, acp.CloseSessionRequest) (acp.CloseSessionResponse, error) {
	return acp.CloseSessionResponse{}, nil
}
func (stubACPAgent) ListSessions(context.Context, acp.ListSessionsRequest) (acp.ListSessionsResponse, error) {
	return acp.ListSessionsResponse{}, nil
}
func (stubACPAgent) ResumeSession(context.Context, acp.ResumeSessionRequest) (acp.ResumeSessionResponse, error) {
	return acp.ResumeSessionResponse{}, nil
}
func (stubACPAgent) SetSessionConfigOption(context.Context, acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	return acp.SetSessionConfigOptionResponse{}, nil
}
func (stubACPAgent) SetSessionMode(context.Context, acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
	return acp.SetSessionModeResponse{}, nil
}

type codexACPAgent struct {
	stubACPAgent
	run     codexExecFn
	dir     string
	profile AgentProfile
	conn    *acp.AgentSideConnection
	mu      sync.Mutex
}

type codexExecFn func(ctx context.Context, dir, prompt string, profile AgentProfile, emit func(string)) error

func newCodexACPAgent(dir string, run codexExecFn) *codexACPAgent {
	if run == nil {
		run = runCodexExec
	}
	return &codexACPAgent{run: run, dir: dir}
}

func (a *codexACPAgent) setConn(c *acp.AgentSideConnection) {
	a.mu.Lock()
	a.conn = c
	a.mu.Unlock()
}

func (a *codexACPAgent) setProfile(p AgentProfile) {
	a.mu.Lock()
	a.profile = p
	if p.WorkDir != "" {
		a.dir = p.WorkDir
	}
	a.mu.Unlock()
}

func (a *codexACPAgent) setDir(dir string) {
	a.mu.Lock()
	if dir != "" {
		a.dir = dir
	}
	a.mu.Unlock()
}

func (a *codexACPAgent) Initialize(context.Context, acp.InitializeRequest) (acp.InitializeResponse, error) {
	return acp.InitializeResponse{
		ProtocolVersion: acp.ProtocolVersionNumber,
		AgentInfo:       &acp.Implementation{Name: "UnityPerfAgent-Codex", Version: "1.0.0"},
	}, nil
}

func (a *codexACPAgent) NewSession(_ context.Context, p acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	if p.Cwd != "" {
		a.dir = p.Cwd
	}
	return acp.NewSessionResponse{SessionId: acp.SessionId("codex-sess")}, nil
}

func (a *codexACPAgent) Prompt(ctx context.Context, p acp.PromptRequest) (acp.PromptResponse, error) {
	prompt := promptText(p)
	if prompt == "" {
		return acp.PromptResponse{}, fmt.Errorf("llm: empty prompt")
	}
	a.mu.Lock()
	conn := a.conn
	dir := a.dir
	run := a.run
	profile := a.profile
	a.mu.Unlock()
	if conn == nil || run == nil {
		return acp.PromptResponse{}, fmt.Errorf("llm: codex adapter not ready")
	}
	err := run(ctx, dir, prompt, profile, func(chunk string) {
		_ = conn.SessionUpdate(ctx, acp.SessionNotification{
			SessionId: p.SessionId,
			Update:    acp.UpdateAgentMessageText(chunk),
		})
	})
	if err != nil {
		return acp.PromptResponse{}, err
	}
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
}

func promptText(p acp.PromptRequest) string {
	var b strings.Builder
	for _, block := range p.Prompt {
		if block.Text != nil {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(block.Text.Text)
		}
	}
	return strings.TrimSpace(b.String())
}

var runCodexExec codexExecFn = defaultRunCodexExec

func defaultRunCodexExec(ctx context.Context, dir, prompt string, profile AgentProfile, emit func(string)) error {
	exe, argv, err := ResolveLaunch("codex", CodexExecArgs(prompt, profile))
	if err != nil {
		return err
	}
	cmd := procutil.CommandContext(ctx, exe, argv...)
	if dir != "" {
		cmd.Dir = dir
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("llm: codex stdout: %w", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("llm: start codex: %w", err)
	}
	got := scanCodexJSONL(stdout, emit)
	waitErr := cmd.Wait()
	if got == 0 {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = "codex exec 没有返回文本"
		}
		if waitErr != nil {
			return fmt.Errorf("llm: %s (%v)", msg, waitErr)
		}
		return fmt.Errorf("llm: %s", msg)
	}
	if waitErr != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

func scanCodexJSONL(r io.Reader, emit func(string)) int {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	n := 0
	for sc.Scan() {
		for _, text := range extractCodexTexts(sc.Bytes()) {
			if text == "" {
				continue
			}
			emit(text)
			n++
		}
	}
	return n
}

func extractCodexTexts(line []byte) []string {
	line = bytes.TrimSpace(line)
	if len(line) == 0 || line[0] != '{' {
		return nil
	}
	var ev map[string]any
	if err := json.Unmarshal(line, &ev); err != nil {
		return nil
	}
	var out []string
	if t := stringField(ev, "delta"); t != "" && looksLikeMessage(ev) {
		out = append(out, t)
	}
	if t := stringField(ev, "text"); t != "" && looksLikeMessage(ev) {
		out = append(out, t)
	}
	if item, ok := ev["item"].(map[string]any); ok {
		kind := strings.ToLower(stringField(item, "type") + " " + stringField(item, "item_type"))
		if strings.Contains(kind, "agent_message") || strings.Contains(kind, "message") {
			if t := stringField(item, "text"); t != "" {
				out = append(out, t)
			}
			if t := stringField(item, "delta"); t != "" {
				out = append(out, t)
			}
		}
	}
	return out
}

func looksLikeMessage(ev map[string]any) bool {
	typ := strings.ToLower(stringField(ev, "type"))
	if typ == "" {
		return true
	}
	return strings.Contains(typ, "message") || strings.Contains(typ, "delta") || strings.Contains(typ, "item.")
}

func stringField(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

var _ acp.Agent = (*codexACPAgent)(nil)
