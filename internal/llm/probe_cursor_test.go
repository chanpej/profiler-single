package llm

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

// 本机有 Cursor CLI 时实际跑一遍 ACP 握手，避免设置里的连接检测是假绿/假红。
func TestProbeCursorCLI(t *testing.T) {
	if _, err := exec.LookPath("agent"); err != nil {
		t.Skip("PATH 上没有 agent（Cursor CLI）")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	r := Probe(ctx, Config{ActiveID: "cursor", Agents: DefaultAgents()})
	t.Logf("cursor probe: ok=%v name=%q msg=%s", r.OK, r.Name, r.Message)
	if !r.OK {
		t.Fatalf("Cursor ACP 连接失败: %s", r.Message)
	}
}
