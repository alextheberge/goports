//go:build windows
// +build windows

package ports

import (
    "net"
    "testing"
    "time"
)

func TestAppsByWinMetadata(t *testing.T) {
    ln, err := net.Listen("tcp4", "127.0.0.1:0")
    if err != nil {
        t.Skipf("unable to listen: %v", err)
    }
    defer ln.Close()
    port := ln.Addr().(*net.TCPAddr).Port
    time.Sleep(100 * time.Millisecond)

    m, err := appsByWin()
    if err != nil {
        t.Fatalf("appsByWin failed: %v", err)
    }
    found := false
    for k, ents := range m {
        if k.Protocol == "tcp" && k.Port == port {
            for _, e := range ents {
                if e.Pid != 0 {
                    found = true
                    break
                }
            }
        }
        if found {
            break
        }
    }
    if !found {
        t.Fatalf("expected entry for port %d, got %v", port, m)
    }
}
