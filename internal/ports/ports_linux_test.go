//go:build linux
// +build linux

package ports

import "testing"

func TestDiscoverPortsLinuxStub(t *testing.T) {
    if _, err := discoverPorts(); err == nil {
        t.Fatalf("expected error from linux stub, got nil")
    }
}
