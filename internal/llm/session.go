package llm

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	acp "github.com/coder/acp-go-sdk"

	"unityperfagent/internal/procutil"
)

const chatSystemHint = `你是 Unity 移动游戏性能优化专家，目标平台为 Unity 2022（其他版本不需要考虑）。请用中文简洁回答。你可以使用 UnityPerfAgent MCP 工具查询当前抓取的帧、CPU Hierarchy、统计与规则分析；涉及具体性能结论时应先查询数据，不要臆测。

这是一个持久会话：后续用户消息可能是对前文的追问，应结合已有上下文，仅查询完成当前请求所需的数据。用户指定帧号时，优先查询该帧及其 CPU Hierarchy；除非用户明确要求总体概览或上下文确实不足，否则不要重新进行全量性能概览。每次收到实际用户请求后，必须立即调用完成任务所需的 MCP 工具并在同一回合给出结论、数据证据以及按优先级排序的 Unity 2022 优化建议；不要只回复确认、复述请求或给出执行计划。`

// ChatSession 是与 ACP Agent 的长连接多轮会话。
// 一次 Start 对应一个 Agent 子进程与一条 ACP session，后续 Prompt 复用同一会话。
type ChatSession struct {
	cfg        Config
	cmd        *exec.Cmd
	cancel     context.CancelFunc
	stdin      io.Closer
	conn       *acp.ClientSideConnection
	sessionID  acp.SessionId
	sink       *updateSink
	stderr     syncBuf
	applyCfg   func(Config)
	models     []AgentModel
	mcpServers []acp.McpServer
	mu         sync.Mutex
	closeOnce  sync.Once
	first      bool
}

type syncBuf struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// NewChatSession 创建尚未启动的会话。可选 MCP Server 会在 Start 时暴露给 Agent。
func NewChatSession(cfg Config, mcpServers ...MCPServer) *ChatSession {
	return &ChatSession{cfg: cfg, mcpServers: append([]acp.McpServer(nil), mcpServers...), sink: &updateSink{}, first: true}
}

// Start 拉起 ACP Agent 子进程并完成 initialize / authenticate / session/new。
func (s *ChatSession) Start(ctx context.Context) error {
	launch := s.cfg.Launch()
	if useInProcessCodex(launch) {
		return s.startInProcessCodex(ctx)
	}
	exe, argv, err := ResolveLaunch(launch.Command, launch.ExecArgs())
	if err != nil {
		return err
	}
	procCtx, cancel := context.WithCancel(context.Background())
	cmd := procutil.CommandContext(procCtx, exe, argv...)
	if launch.WorkDir != "" {
		cmd.Dir = launch.WorkDir
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("llm: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		_ = stdin.Close()
		return fmt.Errorf("llm: stdout pipe: %w", err)
	}
	cmd.Stderr = &s.stderr
	if err := cmd.Start(); err != nil {
		cancel()
		_ = stdin.Close()
		return fmt.Errorf("llm: start agent %q: %w", exe, err)
	}
	s.cmd = cmd
	s.cancel = cancel
	s.stdin = stdin
	if err := s.startOver(ctx, stdin, stdout); err != nil {
		hint := s.stderrHint()
		s.Close()
		if hint != "" {
			return fmt.Errorf("%w\n%s", err, hint)
		}
		return err
	}
	return nil
}

func (s *ChatSession) startInProcessCodex(ctx context.Context) error {
	clientToAgentR, clientToAgentW := io.Pipe()
	agentToClientR, agentToClientW := io.Pipe()
	agent := newCodexACPAgent(s.cfg.cwd(), createCodexServer)
	agent.setProfile(s.cfg.Launch())
	conn := acp.NewAgentSideConnection(agent, agentToClientW, clientToAgentR)
	agent.setConn(conn)
	s.applyCfg = func(cfg Config) {
		agent.setProfile(cfg.Launch())
		agent.setDir(cfg.cwd())
	}
	s.stdin = clientToAgentW
	s.cancel = func() {
		_ = clientToAgentW.Close()
		_ = agentToClientW.Close()
		_ = clientToAgentR.Close()
		_ = agentToClientR.Close()
	}
	if err := s.startOver(ctx, clientToAgentW, agentToClientR); err != nil {
		s.Close()
		return err
	}
	// Codex app-server receives chatSystemHint as thread/start baseInstructions,
	// so every turn can forward the user's message without prompt wrapping.
	s.mu.Lock()
	s.first = false
	s.mu.Unlock()
	return nil
}

func (s *ChatSession) stderrHint() string {
	t := strings.TrimSpace(s.stderr.String())
	if t == "" {
		return ""
	}
	if len(t) > 600 {
		t = t[len(t)-600:]
	}
	return t
}

// startOver 在给定传输通道上完成 ACP 握手并创建 session。
func (s *ChatSession) startOver(ctx context.Context, agentStdin io.Writer, agentStdout io.Reader) error {
	conn, sess, err := acpHandshake(ctx, s.sink, agentStdin, agentStdout, s.cfg.cwd(), s.mcpServers)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.conn = conn
	s.sessionID = sess.SessionId
	s.models = ModelsFromACPConfig(sess.ConfigOptions)
	s.first = true
	s.mu.Unlock()
	return nil
}

// Models 返回握手时 Agent 通过 ACP configOptions 声明的模型列表。
func (s *ChatSession) Models() []AgentModel {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]AgentModel(nil), s.models...)
}

// UpdateConfig 更新当前会话使用的 Agent 配置（进程内 Codex 的下个 turn 立即生效）。
func (s *ChatSession) UpdateConfig(cfg Config) {
	s.mu.Lock()
	s.cfg = cfg
	apply := s.applyCfg
	s.mu.Unlock()
	if apply != nil {
		apply(cfg)
	}
}

func (c Config) cwd() string {
	if p := c.Launch(); p.WorkDir != "" {
		return p.WorkDir
	}
	if c.WorkDir != "" {
		return c.WorkDir
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return ""
}

// Prompt 向当前 ACP session 发送一轮用户消息，并通过 onChunk 流式回调文本。
func (s *ChatSession) Prompt(ctx context.Context, text string, onChunk func(string)) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("llm: empty prompt")
	}
	s.mu.Lock()
	conn := s.conn
	id := s.sessionID
	first := s.first
	s.mu.Unlock()
	if conn == nil {
		return "", fmt.Errorf("llm: chat session not started")
	}
	if first {
		// ACP has no system-role content. Attach the fixed session instructions
		// once to the first prompt; later user messages remain untouched.
		text = chatSystemHint + "\n\n用户首个请求：\n" + text
		s.mu.Lock()
		s.first = false
		s.mu.Unlock()
	}
	s.sink.reset(onChunk)
	resp, err := conn.Prompt(ctx, acp.PromptRequest{
		SessionId: id,
		Prompt:    []acp.ContentBlock{acp.TextBlock(text)},
	})
	if err != nil {
		return "", fmt.Errorf("llm: acp session/prompt: %w", err)
	}
	out := s.sink.text()
	if out == "" {
		return "", fmt.Errorf("llm: agent returned no message (stop reason: %s)", resp.StopReason)
	}
	return out, nil
}

// Close 结束 ACP 会话并杀掉 Agent 子进程。可重复调用。
func (s *ChatSession) Close() {
	s.closeOnce.Do(s.closeOnceFn)
}

func (s *ChatSession) closeOnceFn() {
	s.mu.Lock()
	conn := s.conn
	id := s.sessionID
	s.conn = nil
	s.mu.Unlock()
	if conn != nil && id != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, _ = conn.CloseSession(ctx, acp.CloseSessionRequest{SessionId: id})
		cancel()
	}
	if s.stdin != nil {
		_ = s.stdin.Close()
		s.stdin = nil
	}
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		done := make(chan struct{})
		go func() {
			_ = s.cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
		s.cmd = nil
	}
}
