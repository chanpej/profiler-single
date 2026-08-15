package llm

import (
	"fmt"
	"os/exec"
	"strings"
)

// ResolveLaunch 把 Agent 启动命令解析成可直接 exec 的绝对路径与参数。
// ACP 协议层只用 acp-go-sdk；这里不再把 .cmd 展开成 node.exe。
func ResolveLaunch(command string, args []string) (string, []string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", nil, ErrNotConfigured
	}
	path, err := exec.LookPath(command)
	if err != nil {
		return "", nil, fmt.Errorf("找不到可执行文件 %q", command)
	}
	return path, append([]string{}, args...), nil
}
