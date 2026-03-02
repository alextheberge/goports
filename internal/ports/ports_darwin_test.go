//go:build darwin
// +build darwin

package ports

import (
    "net"
    "testing"
    "time"
)

func TestDiscoverPortsDarwin(t *testing.T) {
    out, err := discoverPorts()
    if err != nil {
        t.Fatalf("darwin discoverPorts failed: %v", err)
    }
    if len(out) == 0 {
        t.Fatalf("darwin discoverPorts returned empty output")
    }
}

func TestNativeOnlyMetadata(t *testing.T) {
    // We can't predict the system state, but if lsof is working we should
    // see at least one nonzero PID when nativeOnly is false.
    SetNativeOnly(false)
    m1, err := appsBySysctl()
    if err != nil {
        t.Skipf("sysctl not available: %v", err)
    }
    found := false
    for _, list := range m1 {
        for _, e := range list {
            if e.Pid != 0 {
                found = true
                break
            }
        }
        if found {
            break
        }
    }
    if !found {
        t.Skip("no metadata returned by lsof; skipping check")
    }

    SetNativeOnly(true)
    m2, err := appsBySysctl()
    if err != nil {
        t.Skipf("sysctl not available: %v", err)
    }
    for _, list := range m2 {
        for _, e := range list {
            if e.Pid != 0 {
                t.Errorf("expected zero pid with nativeOnly, got %d", e.Pid)
            }
        }
    }
}

func TestHostAndFamily(t *testing.T) {
    // IPv4 only
    host, fam := hostAndFamily(0x0100007F, [16]byte{}) // 127.0.0.1
    if host != "127.0.0.1" || fam != "IPv4" {
        t.Fatalf("ipv4 conversion failed: %s %s", host, fam)
    }
    // IPv6 address
    var v6 [16]byte
    ip := net.ParseIP("fe80::1").To16()
    copy(v6[:], ip)
    host, fam = hostAndFamily(0, v6)
    if host != "fe80::1" || fam != "IPv6" {
        t.Fatalf("ipv6 conversion failed: %s %s", host, fam)
    }
}

func TestSysctlIPv6(t *testing.T) {
    // listen on a temporary IPv6 TCP port and ensure appsBySysctl returns
    // an entry for it.  Skipped if the platform doesn't support IPv6.
    ln, err := net.Listen("tcp6", "[::1]:0")
    if err != nil {
        t.Skipf("ipv6 not available: %v", err)
    }
    defer ln.Close()
    port := ln.Addr().(*net.TCPAddr).Port

    // give the kernel a moment to register the socket
    time.Sleep(100 * time.Millisecond)

    m, err := appsBySysctl()
    if err != nil {
        t.Skipf("appsBySysctl failed: %v", err)
    }
    if len(m) == 0 {
        t.Skip("appsBySysctl returned no entries; possibly restricted environment")
    }
    found := false
    for k, ents := range m {
        if k.Protocol == "tcp" && k.Port == port {
            for _, e := range ents {
                if e.Family == "IPv6" {
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
        t.Fatalf("expected IPv6 listener %d in map: %v", port, m)
    }
}

func TestMergeLsofMeta(t *testing.T) {
    // create a fake native result with one entry
    key := PortKey{Protocol: "tcp", Port: 8080}
    base := map[PortKey][]PortEntry{
        key: {{Host: "127.0.0.1", Protocol: "tcp"}},
    }
    lsof := []byte("COMMAND PID USER FD TYPE DEVICE SIZE/OFF NODE NAME\nnginx 1234 root 10u IPv4 0x1234 0t0 TCP 127.0.0.1:8080 (LISTEN)\n")
    meta := parseLsof(lsof)
    if _, ok := meta[key]; !ok {
        t.Fatalf("parseLsof returned no entry; meta=%+v", meta)
    }
    mergeLsofMeta(base, lsof)
    if e := base[key][0]; e.Pid != 1234 || e.Name != "nginx" {
        t.Fatalf("merge failed, got %+v; meta=%+v", e, meta)
    }
}
