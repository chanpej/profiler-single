//go:build !windows

package services

import "sync"

var updateMutex sync.Mutex

func lockUpdate(string) (func(), error) { updateMutex.Lock(); return updateMutex.Unlock, nil }
