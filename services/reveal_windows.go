//go:build windows

package services

import (
	"os/exec"
	"syscall"
)

func revealFile(path string) error {
	cmd := exec.Command("explorer.exe")
	// Explorer expects the /select, prefix outside the quoted literal path.
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe /select,"` + path + `"`}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
