// Package procutil 提供外部进程启动的统一封装。
//
// UnityPerfAgent 是 Windows 桌面 GUI 程序，直接 exec 拉起 adb、LLM Agent
// 等控制台子进程时会弹出黑色控制台窗口。本包在 Windows 上通过
// CREATE_NO_WINDOW | HideWindow 隐藏控制台，其他平台为空操作。
package procutil

import (
	"context"
	"os/exec"
)

// CommandContext 等价于 exec.CommandContext，并隐藏子进程的控制台窗口。
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	hideConsole(cmd)
	return cmd
}

// Command 等价于 exec.Command，并隐藏子进程的控制台窗口。
func Command(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	hideConsole(cmd)
	return cmd
}
