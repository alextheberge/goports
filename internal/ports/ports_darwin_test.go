//go:build darwin
// +build darwin

package ports

import "testing"

func TestDiscoverPortsDarwin(t *testing.T) {
    out, err := discoverPorts()
    if err != nil {
        t.Fatalf("darwin discoverPorts failed: %v", err)
    }
    if len(out) == 0 {
        t.Fatalf("darwin discoverPorts returned empty output")
    }
}
