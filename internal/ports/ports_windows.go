//go:build windows
// +build windows

package ports

import (
    "os/exec"
)

// on Windows we fall back to netstat output; this is not very efficient but
// allows the CLI to run if netstat is present.  Future versions should use
// native Win32 APIs (GetExtendedTcpTable/GetExtendedUdpTable).
func discoverPorts() ([]byte, error) {
    return exec.Command("netstat", "-an").Output()
}
