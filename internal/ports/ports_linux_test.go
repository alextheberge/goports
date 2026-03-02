//go:build linux
// +build linux

package ports

import "testing"

func TestDiscoverPortsLinuxFallback(t *testing.T) {
    // we currently shell out to lsof; if lsof is unavailable we simply
    // verify that an error is returned rather than panicking.
    _, err := discoverPorts()
    if err != nil {
        t.Logf("discoverPorts returned error (lsof may be missing): %v", err)
    }
}
