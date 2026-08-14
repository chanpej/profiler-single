// Package llm 通过 ACP（Agent Client Protocol）接入外部 LLM Agent，
// 把 analyzer 的结构化报告交给 Agent 生成自动化的性能诊断结论。
//
// UnityPerfAgent 作为 ACP Client，通过 stdio 启动并连接任意支持 ACP 的
// Agent（如 `gemini --experimental-acp`、`claude-code-acp` 等），
// 走 initialize → session/new → session/prompt 流程，流式收集回复文本。
package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	acp "github.com/coder/acp-go-sdk"

	"unityperfagent/internal/procutil"
)

// Config 是 ACP Agent 的接入配置。
type Config struct {
	// AgentCommand 是 ACP Agent 的可执行命令，例如 "gemini"。
	AgentCommand string `json:"agentCommand"`
	// AgentArgs 是附加参数，例如 ["--experimental-acp"]。
	AgentArgs []string `json:"agentArgs"`
	// WorkDir 是 Agent 会话的工作目录，留空时使用当前目录。
	WorkDir string `json:"workDir"`
}

// ErrNotConfigured 表示尚未配置 ACP Agent 命令。
var ErrNotConfigured = errors.New("llm: not configured (missing acp agent command)")

// Client 通过 ACP 与外部 LLM Agent 通信。
type Client struct {
	cfg Config
}

// NewClient 创建客户端。
func NewClient(cfg Config) *Client { return &Client{cfg: cfg} }

// Configured 返回配置是否可用。
func (c *Client) Configured() bool { return strings.TrimSpace(c.cfg.AgentCommand) != "" }

const promptTemplate = `你是 Unity 移动游戏性能优化专家，目标平台为 Unity 2022（其他版本不需要考虑）。
下面是一份来自真机采集的性能分析报告（JSON），请：
1. 用中文给出简明的诊断结论；
2. 按优先级列出最多 5 条可执行的优化建议，结合 Unity 2022 的特性（如 URP、Burst、Incremental GC）；
3. 若数据不足请直接说明。

性能分析报告：
%s`

// Diagnose 启动配置的 ACP Agent 子进程，发送报告并返回诊断文本。
func (c *Client) Diagnose(ctx context.Context, reportJSON string) (string, error) {
	if !c.Configured() {
		return "", ErrNotConfigured
	}
	// 隐藏 Agent 子进程的控制台窗口（Windows 桌面程序下避免弹出黑窗）。
	cmd := procutil.CommandContext(ctx, c.cfg.AgentCommand, c.cfg.AgentArgs...)
	if c.cfg.WorkDir != "" {
		cmd.Dir = c.cfg.WorkDir
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("llm: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("llm: stdout pipe: %w", err)
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("llm: start agent %q: %w", c.cfg.AgentCommand, err)
	}
	defer func() {
		stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	return c.diagnoseOver(ctx, stdin, stdout, reportJSON)
}

// diagnoseOver 在给定的传输通道上执行一次完整的 ACP 诊断回合。
// 独立出来便于在测试中用内存管道对接假 Agent。
func (c *Client) diagnoseOver(ctx context.Context, agentStdin io.Writer, agentStdout io.Reader, reportJSON string) (string, error) {
	sink := &updateSink{}
	conn := acp.NewClientSideConnection(sink, agentStdin, agentStdout)

	if _, err := conn.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientInfo:      &acp.Implementation{Name: "UnityPerfAgent", Version: "1.0.0"},
	}); err != nil {
		return "", fmt.Errorf("llm: acp initialize: %w", err)
	}

	cwd := c.cfg.WorkDir
	if cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		}
	}
	sess, err := conn.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        cwd,
		McpServers: []acp.McpServer{},
	})
	if err != nil {
		return "", fmt.Errorf("llm: acp session/new: %w", err)
	}

	resp, err := conn.Prompt(ctx, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock(fmt.Sprintf(promptTemplate, reportJSON))},
	})
	if err != nil {
		return "", fmt.Errorf("llm: acp session/prompt: %w", err)
	}

	text := sink.text()
	if text == "" {
		return "", fmt.Errorf("llm: agent returned no message (stop reason: %s)", resp.StopReason)
	}
	return text, nil
}

// updateSink 实现 acp.Client 接口，收集 Agent 的流式回复文本。
// 性能诊断是只读任务，文件系统/终端/权限请求一律拒绝。
type updateSink struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (s *updateSink) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.TrimSpace(s.buf.String())
}

// SessionUpdate 收集 agent_message_chunk 中的文本内容。
func (s *updateSink) SessionUpdate(ctx context.Context, n acp.SessionNotification) error {
	if n.Update.AgentMessageChunk != nil {
		if t := n.Update.AgentMessageChunk.Content.Text; t != nil {
			s.mu.Lock()
			s.buf.WriteString(t.Text)
			s.mu.Unlock()
		}
	}
	return nil
}

// RequestPermission 拒绝一切工具授权请求（诊断任务无需 Agent 改动任何东西）。
func (s *updateSink) RequestPermission(ctx context.Context, p acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	return acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeCancelled()}, nil
}

var errCapabilityDisabled = errors.New("llm: capability not supported by UnityPerfAgent")

func (s *updateSink) ReadTextFile(ctx context.Context, p acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	return acp.ReadTextFileResponse{}, errCapabilityDisabled
}

func (s *updateSink) WriteTextFile(ctx context.Context, p acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	return acp.WriteTextFileResponse{}, errCapabilityDisabled
}

func (s *updateSink) CreateTerminal(ctx context.Context, p acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	return acp.CreateTerminalResponse{}, errCapabilityDisabled
}

func (s *updateSink) KillTerminal(ctx context.Context, p acp.KillTerminalRequest) (acp.KillTerminalResponse, error) {
	return acp.KillTerminalResponse{}, errCapabilityDisabled
}

func (s *updateSink) TerminalOutput(ctx context.Context, p acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	return acp.TerminalOutputResponse{}, errCapabilityDisabled
}

func (s *updateSink) ReleaseTerminal(ctx context.Context, p acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	return acp.ReleaseTerminalResponse{}, errCapabilityDisabled
}

func (s *updateSink) WaitForTerminalExit(ctx context.Context, p acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	return acp.WaitForTerminalExitResponse{}, errCapabilityDisabled
}
