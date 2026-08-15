package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf16"

	"unityperfagent/internal/procutil"
)

// AgentModel 是可供选择的模型项。
type AgentModel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

const codexModelsTimeout = 20 * time.Second

type codexDebugFn func(ctx context.Context, command string, bundled bool) ([]byte, error)

var runCodexDebug = defaultRunCodexDebug

func defaultRunCodexDebug(ctx context.Context, command string, bundled bool) ([]byte, error) {
	args := []string{"debug", "models"}
	if bundled {
		args = append(args, "--bundled")
	}
	exe, argv, err := ResolveLaunch(command, args)
	if err != nil {
		return nil, err
	}
	cmd := procutil.CommandContext(ctx, exe, argv...)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListCodexModels 探测 Codex 当前支持的模型目录（优先合并目录，失败则回退捆绑目录）。
func ListCodexModels(ctx context.Context, p AgentProfile) ([]AgentModel, error) {
	command := strings.TrimSpace(p.Command)
	if command == "" {
		command = "codex"
	}
	if ctx == nil {
		ctx = context.Background()
	}
	probeCtx, cancel := context.WithTimeout(ctx, codexModelsTimeout)
	defer cancel()

	out, err := runCodexDebug(probeCtx, command, false)
	models, parseErr := ParseCodexModelCatalog(out)
	if parseErr == nil && len(models) > 0 {
		return models, nil
	}
	bundled, bErr := runCodexDebug(probeCtx, command, true)
	if bErr == nil {
		if m, pErr := ParseCodexModelCatalog(bundled); pErr == nil && len(m) > 0 {
			return m, nil
		}
	}
	if err != nil {
		return nil, fmt.Errorf("llm: 探测 Codex 模型失败: %w", err)
	}
	if parseErr != nil {
		return nil, fmt.Errorf("llm: 解析 Codex 模型目录失败: %w", parseErr)
	}
	if bErr != nil {
		return nil, fmt.Errorf("llm: 探测 Codex 模型失败: %w", bErr)
	}
	return nil, fmt.Errorf("llm: Codex 模型目录为空")
}

type codexModelCatalog struct {
	Models []codexCatalogModel `json:"models"`
}

type codexCatalogModel struct {
	Slug        string `json:"slug"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Visibility  string `json:"visibility"`
}

// ParseCodexModelCatalog 解析 `codex debug models` 的 JSON 输出。
func ParseCodexModelCatalog(raw []byte) ([]AgentModel, error) {
	raw = decodeCatalogBytes(raw)
	raw = extractJSONObject(raw)
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("empty catalog")
	}
	var cat codexModelCatalog
	if err := json.Unmarshal(raw, &cat); err != nil {
		var arr []codexCatalogModel
		if err2 := json.Unmarshal(raw, &arr); err2 != nil {
			return nil, err
		}
		cat.Models = arr
	}
	out := make([]AgentModel, 0, len(cat.Models))
	seen := map[string]struct{}{}
	for _, m := range cat.Models {
		if strings.EqualFold(strings.TrimSpace(m.Visibility), "hide") {
			continue
		}
		id := strings.TrimSpace(m.Slug)
		if id == "" {
			id = strings.TrimSpace(m.ID)
		}
		if id == "" {
			id = strings.TrimSpace(m.Name)
		}
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		name := strings.TrimSpace(m.DisplayName)
		if name == "" {
			name = strings.TrimSpace(m.Name)
		}
		if name == "" {
			name = id
		}
		out = append(out, AgentModel{ID: id, Name: name})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no visible models")
	}
	return out, nil
}

func decodeCatalogBytes(raw []byte) []byte {
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	if len(raw) >= 2 && raw[0] == 0xFF && raw[1] == 0xFE {
		u16 := make([]uint16, (len(raw)-2)/2)
		for i := range u16 {
			u16[i] = uint16(raw[2+i*2]) | uint16(raw[3+i*2])<<8
		}
		return []byte(string(utf16.Decode(u16)))
	}
	return raw
}

func extractJSONObject(raw []byte) []byte {
	raw = bytes.TrimSpace(raw)
	startObj := bytes.IndexByte(raw, '{')
	startArr := bytes.IndexByte(raw, '[')
	start := startObj
	endByte := byte('}')
	if startObj < 0 || (startArr >= 0 && startArr < startObj) {
		start = startArr
		endByte = ']'
	}
	if start < 0 {
		return raw
	}
	end := bytes.LastIndexByte(raw, endByte)
	if end <= start {
		return raw[start:]
	}
	return raw[start : end+1]
}
