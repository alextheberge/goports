//go:build !windows
// +build !windows

package cli

import (
    "syscall"
)

// killPID sends the specified signal to the process identified by pid.
// On Unix-like systems we can use syscall.Kill directly.
func killPID(pid int, sig syscall.Signal) error {
    return syscall.Kill(pid, sig)
}
