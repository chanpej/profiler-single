package llm

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	acp "github.com/coder/acp-go-sdk"
)

const defaultEffort = "medium"

var effortIDs = map[string]struct{}{
	"low":    {},
	"medium": {},
	"high":   {},
	"xhigh":  {},
}

// NormalizeEffort 只允许 low/medium/high/xhigh，其余回落到 medium。
func NormalizeEffort(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	if _, ok := effortIDs[v]; ok {
		return v
	}
	return defaultEffort
}

func IsCursorCLI(p AgentProfile) bool {
	return p.ID == "cursor" || commandBase(p.Command) == "agent" || commandBase(p.Command) == "cursor-agent"
}

// IsCodexCLI 表示该 Agent 走 Codex CLI（可用 `codex debug models` 探测模型目录）。
func IsCodexCLI(p AgentProfile) bool {
	return p.ID == "codex" || commandBase(p.Command) == "codex"
}

func commandBase(command string) string {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(command)))
	base = strings.TrimSuffix(base, ".exe")
	base = strings.TrimSuffix(base, ".cmd")
	base = strings.TrimSuffix(base, ".bat")
	return base
}

// CursorModelSpec 生成 Cursor CLI `--model` 参数。始终带 fast=false，禁用 Fast 档。
func CursorModelSpec(model, effort string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	return model + "[effort=" + NormalizeEffort(effort) + ",fast=false]"
}

// ExecArgs 返回实际启动参数。Cursor 在 acp 前注入 --model；Codex 原样返回存储参数。
func (p AgentProfile) ExecArgs() []string {
	args := append([]string{}, p.Args...)
	if !IsCursorCLI(p) {
		return args
	}
	spec := CursorModelSpec(p.Model, p.Effort)
	if spec == "" {
		return args
	}
	return injectFlag(args, "--model", spec)
}

func injectFlag(args []string, flag, value string) []string {
	out := make([]string, 0, len(args)+2)
	skip := false
	for _, a := range args {
		if skip {
			skip = false
			continue
		}
		if a == flag {
			skip = true
			continue
		}
		if strings.HasPrefix(a, flag+"=") {
			continue
		}
		out = append(out, a)
	}
	for i, a := range out {
		if a == "acp" {
			return append(append(append([]string{}, out[:i]...), flag, value), out[i:]...)
		}
	}
	return append([]string{flag, value}, out...)
}

// CodexExecArgs 生成 `codex exec` 参数。不附加 --fast，禁用 Fast 档。
// CodexExecArgsWithMCP is the native Codex CLI launch form with explicitly
// supplied MCP servers. It is used for direct CLI validation without ACP.
func CodexExecArgsWithMCP(prompt string, p AgentProfile, servers []MCPServer) []string {
	p.mcpServers = append([]acp.McpServer(nil), servers...)
	return CodexExecArgs(prompt, p)
}

func CodexExecArgs(prompt string, p AgentProfile) []string {
	// Agent Chat runs non-interactively; default Codex approval prompts would
	// otherwise cancel MCP tool calls because no terminal UI can answer them.
	// The user has explicitly opted into full Agent permissions.
	args := []string{"exec", "--json", "--skip-git-repo-check", "--ephemeral", "--color", "never", "--dangerously-bypass-approvals-and-sandbox"}
	if m := strings.TrimSpace(p.Model); m != "" {
		args = append(args, "--model", m)
	}
	if e := strings.TrimSpace(p.Effort); e != "" {
		args = append(args, "-c", "model_reasoning_effort="+NormalizeEffort(e))
	}
	args = append(args, codexMCPConfigArgs(p.mcpServers)...)
	return append(args, prompt)
}

// codexMCPConfigArgs converts ACP stdio MCP declarations into Codex's native
// `mcp_servers` configuration. Agent Chat supplies it once when starting the
// persistent app-server; direct CLI validation uses the same conversion.
func codexMCPConfigArgs(servers []acp.McpServer) []string {
	var out []string
	for _, server := range servers {
		if server.Stdio == nil || strings.TrimSpace(server.Stdio.Name) == "" || strings.TrimSpace(server.Stdio.Command) == "" {
			continue
		}
		base := "mcp_servers." + server.Stdio.Name
		out = append(out, "-c", base+".command="+strconv.Quote(server.Stdio.Command))
		if len(server.Stdio.Args) > 0 {
			args, err := json.Marshal(server.Stdio.Args)
			if err == nil {
				out = append(out, "-c", base+".args="+string(args))
			}
		}
		for _, variable := range server.Stdio.Env {
			if strings.TrimSpace(variable.Name) == "" {
				continue
			}
			out = append(out, "-c", fmt.Sprintf("%s.env.%s=%s", base, variable.Name, strconv.Quote(variable.Value)))
		}
	}
	return out
}

func equalArgs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ShouldRestartChat 在 Agent 身份变化，或非进程内 Codex 的 model/effort 变化时需要重开会话。
func ShouldRestartChat(old, next Config) bool {
	a := old.Normalize().Launch()
	b := next.Normalize().Launch()
	if a.ID != b.ID || a.Command != b.Command || a.WorkDir != b.WorkDir || !equalArgs(a.Args, b.Args) {
		return true
	}
	if useInProcessCodex(b) {
		return false
	}
	return a.Model != b.Model || a.Effort != b.Effort
}
