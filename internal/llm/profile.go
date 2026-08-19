package llm

import (
	"strings"

	acp "github.com/coder/acp-go-sdk"
)

// AgentProfile 是一个可切换的 ACP Agent 启动配置。
type AgentProfile struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	WorkDir string   `json:"workDir"`
	Model   string   `json:"model,omitempty"`
	Effort  string   `json:"effort,omitempty"`

	// mcpServers is process-only session state used by the in-process Codex
	// adapter. It is deliberately not persisted in frontend configuration.
	mcpServers []acp.McpServer
}

// DefaultAgents 返回内置预设：Codex 与 Cursor CLI。
//
// Codex：本进程用 acp-go-sdk 适配持久 `codex app-server`（不经过 npx / Node ACP 适配包）。
//
// Cursor CLI：官方 ACP 入口 `agent acp`
//
//	https://cursor.com/docs/cli/acp
func DefaultAgents() []AgentProfile {
	return []AgentProfile{
		{
			ID:      "codex",
			Name:    "Codex",
			Command: "codex",
		},
		{
			ID:      "cursor",
			Name:    "Cursor CLI",
			Command: "agent",
			Args:    []string{"acp"},
		},
	}
}

// Normalize 补齐默认 Agent 列表，并把遗留的单命令配置保留为「自定义」。
func (c Config) Normalize() Config {
	if len(c.Agents) == 0 {
		c.Agents = DefaultAgents()
		if strings.TrimSpace(c.AgentCommand) != "" {
			custom := AgentProfile{
				ID:      "custom",
				Name:    "自定义",
				Command: c.AgentCommand,
				Args:    c.AgentArgs,
				WorkDir: c.WorkDir,
			}
			c.Agents = append([]AgentProfile{custom}, c.Agents...)
			if c.ActiveID == "" {
				c.ActiveID = custom.ID
			}
		}
	}
	if c.ActiveID == "" || c.find(c.ActiveID).ID == "" {
		c.ActiveID = c.Agents[0].ID
	}
	p := c.Launch()
	c.AgentCommand = p.Command
	c.AgentArgs = p.Args
	c.WorkDir = p.WorkDir
	return c
}

func (c Config) find(id string) AgentProfile {
	for _, a := range c.Agents {
		if a.ID == id {
			return a
		}
	}
	return AgentProfile{}
}

// Launch 返回当前应启动的 Agent 配置。
func (c Config) Launch() AgentProfile {
	if p := c.find(c.ActiveID); strings.TrimSpace(p.Command) != "" || p.ID != "" {
		if strings.TrimSpace(p.Command) != "" {
			return p
		}
	}
	if strings.TrimSpace(c.AgentCommand) != "" {
		return AgentProfile{
			ID:      "legacy",
			Name:    "自定义",
			Command: c.AgentCommand,
			Args:    c.AgentArgs,
			WorkDir: c.WorkDir,
		}
	}
	if p := c.find(c.ActiveID); p.ID != "" {
		return p
	}
	if len(c.Agents) > 0 {
		return c.Agents[0]
	}
	return AgentProfile{}
}

// WithActive 切换当前 Agent，并同步遗留字段。
func (c Config) WithActive(id string) Config {
	c.ActiveID = id
	return c.Normalize()
}
