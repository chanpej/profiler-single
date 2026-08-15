package llm

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	acp "github.com/coder/acp-go-sdk"

	"unityperfagent/internal/procutil"
)

const cursorModelsTimeout = 20 * time.Second

type cursorListFn func(ctx context.Context, command string) ([]byte, error)
type cursorACPListFn func(ctx context.Context, p AgentProfile) ([]AgentModel, error)

var runCursorList = defaultRunCursorList
var listCursorACP = defaultListCursorACP

func defaultRunCursorList(ctx context.Context, command string) ([]byte, error) {
	exe, argv, err := ResolveLaunch(command, []string{"--list-models"})
	if err != nil {
		return nil, err
	}
	cmd := procutil.CommandContext(ctx, exe, argv...)
	out, err := cmd.Output()
	if err != nil {
		return out, err
	}
	return out, nil
}

func defaultListCursorACP(ctx context.Context, p AgentProfile) ([]AgentModel, error) {
	cfg := Config{
		ActiveID:     p.ID,
		Agents:       []AgentProfile{{ID: p.ID, Name: p.Name, Command: p.Command, Args: p.Args, WorkDir: p.WorkDir}},
		AgentCommand: p.Command,
		AgentArgs:    p.Args,
		WorkDir:      p.WorkDir,
	}
	sess := NewChatSession(cfg)
	defer sess.Close()
	if err := sess.Start(ctx); err != nil {
		return nil, err
	}
	return sess.Models(), nil
}

// ListCursorModels 探测 Cursor 当前账号可用的模型。
// 优先 `agent --list-models`（官方、按账号）；为空或失败时回退 ACP session/new 的 configOptions。
func ListCursorModels(ctx context.Context, p AgentProfile) ([]AgentModel, error) {
	command := strings.TrimSpace(p.Command)
	if command == "" {
		command = "agent"
	}
	if ctx == nil {
		ctx = context.Background()
	}
	probeCtx, cancel := context.WithTimeout(ctx, cursorModelsTimeout)
	defer cancel()

	out, err := runCursorList(probeCtx, command)
	if models := ParseCursorModelList(decodeCatalogBytes(out)); len(models) > 0 {
		return models, nil
	}
	acpModels, acpErr := listCursorACP(probeCtx, p)
	if len(acpModels) > 0 {
		return acpModels, nil
	}
	if err != nil {
		return nil, fmt.Errorf("llm: 探测 Cursor 模型失败: %w", err)
	}
	if acpErr != nil {
		return nil, fmt.Errorf("llm: 探测 Cursor 模型失败: %w", acpErr)
	}
	return nil, fmt.Errorf("llm: Cursor 模型列表为空")
}

// ParseCursorModelList 解析 `agent --list-models` 文本：`id - Display Name`。
func ParseCursorModelList(raw []byte) []AgentModel {
	text := string(bytesAsText(raw))
	out := make([]AgentModel, 0, 32)
	seen := map[string]struct{}{}
	for _, line := range strings.Split(text, "\n") {
		id, name, ok := splitCursorModelLine(strings.TrimSpace(line))
		if !ok {
			continue
		}
		id = stripModelParams(id)
		if id == "" || isFastModelID(id) {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if name == "" {
			name = id
		}
		out = append(out, AgentModel{ID: id, Name: name})
	}
	return out
}

func splitCursorModelLine(line string) (id, name string, ok bool) {
	if line == "" || strings.EqualFold(line, "available models") {
		return "", "", false
	}
	i := strings.Index(line, " - ")
	if i <= 0 {
		return "", "", false
	}
	id = strings.TrimSpace(line[:i])
	name = strings.TrimSpace(line[i+3:])
	if id == "" || strings.IndexFunc(id, unicode.IsSpace) >= 0 {
		return "", "", false
	}
	return id, name, true
}

func isFastModelID(id string) bool {
	s := strings.ToLower(strings.TrimSpace(id))
	if strings.Contains(s, "fast=true") {
		return true
	}
	if strings.HasSuffix(s, "-fast") || strings.Contains(s, "-fast[") || strings.Contains(s, "-fast-") {
		return true
	}
	return false
}

func stripModelParams(id string) string {
	id = strings.TrimSpace(id)
	if i := strings.IndexByte(id, '['); i > 0 {
		return strings.TrimSpace(id[:i])
	}
	return id
}

func bytesAsText(raw []byte) []byte {
	return decodeCatalogBytes(raw)
}

// ModelsFromACPConfig 从 session/new 的 configOptions 提取 category=model 的选项。
func ModelsFromACPConfig(opts []acp.SessionConfigOption) []AgentModel {
	out := make([]AgentModel, 0)
	seen := map[string]struct{}{}
	for _, o := range opts {
		if o.Select == nil {
			continue
		}
		if !isACPModelSelect(o.Select) {
			continue
		}
		for _, opt := range flattenSelectOptions(o.Select.Options) {
			id := stripModelParams(string(opt.Value))
			if id == "" || isFastModelID(id) || isFastModelID(string(opt.Value)) {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			name := strings.TrimSpace(opt.Name)
			if name == "" {
				name = id
			}
			out = append(out, AgentModel{ID: id, Name: name})
		}
	}
	return out
}

func isACPModelSelect(sel *acp.SessionConfigOptionSelect) bool {
	if sel == nil {
		return false
	}
	if sel.Category != nil && *sel.Category == acp.SessionConfigOptionCategoryModel {
		return true
	}
	return strings.EqualFold(string(sel.Id), "model")
}

func flattenSelectOptions(opts acp.SessionConfigSelectOptions) []acp.SessionConfigSelectOption {
	if opts.Ungrouped != nil {
		return *opts.Ungrouped
	}
	if opts.Grouped != nil {
		var out []acp.SessionConfigSelectOption
		for _, g := range *opts.Grouped {
			out = append(out, g.Options...)
		}
		return out
	}
	return nil
}
