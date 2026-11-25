//go:build windows

package services

import "golang.org/x/sys/windows"

func lockUpdate(target string) (func(), error) {
	path, err := windows.UTF16PtrFromString(target + ".update-lock")
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(path, windows.GENERIC_WRITE|windows.DELETE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_FLAG_DELETE_ON_CLOSE, 0)
	if err != nil {
		return nil, err
	}
	return func() { windows.CloseHandle(h) }, nil
}
