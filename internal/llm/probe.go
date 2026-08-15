package llm

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ProbeResult 是 ACP Agent 连接检测结果。
type ProbeResult struct {
	OK      bool   `json:"ok"`
	Name    string `json:"name"`
	Message string `json:"message"`
}

const probeTimeout = 20 * time.Second

// Probe 拉起 Agent 并完成 ACP initialize / authenticate / session/new，随后立即关闭。
func Probe(ctx context.Context, cfg Config) ProbeResult {
	cfg = cfg.Normalize()
	p := cfg.Launch()
	name := p.Name
	if name == "" {
		name = p.Command
	}
	if strings.TrimSpace(p.Command) == "" {
		return ProbeResult{OK: false, Name: name, Message: "未配置 Agent 命令"}
	}
	if _, err := exec.LookPath(p.Command); err != nil {
		return ProbeResult{OK: false, Name: name, Message: fmt.Sprintf("找不到可执行文件 %q", p.Command)}
	}
	if err := ctx.Err(); err != nil {
		return ProbeResult{OK: false, Name: name, Message: err.Error()}
	}
	probeCtx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	stop := context.AfterFunc(ctx, cancel)
	defer stop()

	one := Config{ActiveID: p.ID, Agents: []AgentProfile{p}, AgentCommand: p.Command, AgentArgs: p.Args, WorkDir: p.WorkDir}
	sess := NewChatSession(one)
	errCh := make(chan error, 1)
	go func() {
		errCh <- sess.Start(probeCtx)
	}()
	select {
	case err := <-errCh:
		if err != nil {
			return ProbeResult{OK: false, Name: name, Message: err.Error()}
		}
		sess.Close()
		return ProbeResult{OK: true, Name: name, Message: "ACP 握手成功（initialize / session/new）"}
	case <-probeCtx.Done():
		sess.Close()
		select {
		case err := <-errCh:
			if err != nil {
				return ProbeResult{OK: false, Name: name, Message: err.Error()}
			}
		case <-time.After(2 * time.Second):
		}
		return ProbeResult{OK: false, Name: name, Message: "ACP 握手超时（20s）。Cursor CLI 请确认已执行 agent login，命令为 agent acp。"}
	}
}
