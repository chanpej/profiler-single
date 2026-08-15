//go:build windows

package procutil

import (
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// createNoWindow 即 Win32 CREATE_NO_WINDOW：不为控制台子进程创建新窗口。
const createNoWindow = 0x08000000

// hideConsole 阻止子进程弹出控制台黑窗口。
// HideWindow 始终设置。.cmd/.bat/.ps1 不能再加 CREATE_NO_WINDOW，否则 ACP stdio 会卡住。
func hideConsole(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	if !isWindowsScript(cmd) {
		cmd.SysProcAttr.CreationFlags |= createNoWindow
	}
}

func isWindowsScript(cmd *exec.Cmd) bool {
	name := cmd.Path
	if name == "" && len(cmd.Args) > 0 {
		name = cmd.Args[0]
	}
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".cmd" || ext == ".bat" || ext == ".ps1"
}
