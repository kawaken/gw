//go:build !unix

package main

import "os"

// pidAlive reports whether the process can be opened. On non-Unix systems
// os.FindProcess fails for a process that does not exist.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = p.Release()
	return true
}
