//go:build !windows

package services

import "github.com/wailsapp/wails/v3/pkg/application"

func revealFile(path string) error {
	return application.Get().Env.OpenFileManager(path, true)
}
