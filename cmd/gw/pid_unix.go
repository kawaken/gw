//go:build unix

package main

import (
	"errors"
	"syscall"
)

// pidAlive sends signal 0, which checks for the process without affecting it.
// EPERM means the process exists but belongs to another user.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
