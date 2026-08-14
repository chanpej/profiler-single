//go:build windows

package procutil

import (
	"os/exec"
	"syscall"
)

// createNoWindow 即 Win32 CREATE_NO_WINDOW：不为控制台子进程创建新窗口。
const createNoWindow = 0x08000000

// hideConsole 阻止子进程弹出控制台黑窗口：
// HideWindow 处理子进程继承父进程控制台的场景，
// CREATE_NO_WINDOW 处理子进程自行分配控制台的场景，两者同时设置才可靠。
func hideConsole(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
