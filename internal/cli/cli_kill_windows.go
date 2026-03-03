//go:build windows
// +build windows

package cli

import (
    "fmt"
    "os/exec"
    "syscall"
)

// killPID attempts to terminate a process on Windows.  We fall back to the
// taskkill command for simplicity; this requires the caller to have privileges.
func killPID(pid int, sig syscall.Signal) error {
    // Windows does not support Unix-style signals; ignore `sig` and just use
    // taskkill /PID <pid> /T /F
    cmd := exec.Command("taskkill", "/PID", fmt.Sprint(pid), "/T", "/F")
    return cmd.Run()
}
