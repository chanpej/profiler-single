package procutil

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// 命令与参数应原样透传给 exec 包。
func TestCommandPassthrough(t *testing.T) {
	cmd := Command("mytool", "a", "b")
	if cmd.Path != "mytool" && !strings.HasSuffix(cmd.Path, "mytool") {
		t.Errorf("Path = %q, want mytool", cmd.Path)
	}
	if got := strings.Join(cmd.Args, " "); got != "mytool a b" {
		t.Errorf("Args = %q, want %q", got, "mytool a b")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmdCtx := CommandContext(ctx, "mytool", "--version")
	if got := strings.Join(cmdCtx.Args, " "); got != "mytool --version" {
		t.Errorf("CommandContext Args = %q", got)
	}
}

// Windows 桌面程序必须隐藏子进程控制台窗口。
func TestHideConsoleWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅 Windows 校验控制台隐藏标志")
	}
	for _, cmd := range []*exec.Cmd{
		Command("gemini", "--experimental-acp"),
		CommandContext(context.Background(), "gemini", "--experimental-acp"),
	} {
		attr := cmd.SysProcAttr
		if attr == nil {
			t.Fatal("SysProcAttr 未设置")
		}
		if !attr.HideWindow {
			t.Error("HideWindow 应为 true")
		}
		const createNoWindow = 0x08000000
		if attr.CreationFlags&createNoWindow == 0 {
			t.Error("CreationFlags 应包含 CREATE_NO_WINDOW")
		}
	}
}

// 非 Windows 平台不应设置 SysProcAttr（保持零值，避免影响其他字段）。
func TestHideConsoleOtherPlatforms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("仅非 Windows 校验空操作")
	}
	cmd := Command("gemini", "--experimental-acp")
	if cmd.SysProcAttr != nil {
		t.Errorf("非 Windows 平台不应设置 SysProcAttr: %+v", cmd.SysProcAttr)
	}
}

// 端到端：通过封装真实执行一条命令并拿到输出。
func TestCommandRealRun(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("未找到 go 命令")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := CommandContext(ctx, goBin, "version").Output()
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if !strings.HasPrefix(string(out), "go version") {
		t.Errorf("输出异常: %q", out)
	}
}
