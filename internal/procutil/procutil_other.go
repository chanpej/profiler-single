//go:build !windows

package procutil

import "os/exec"

// hideConsole 在非 Windows 平台为空操作（类 Unix 子进程不弹控制台窗口）。
func hideConsole(cmd *exec.Cmd) {}
