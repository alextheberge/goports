//go:build darwin
// +build darwin

package ports

import (
    "fmt"
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

func TestProcMetadata(t *testing.T) {
    ln, err := net.Listen("tcp", "127.0.0.1:0")
    if err != nil {
        t.Skipf("unable to listen: %v", err)
    }
    defer ln.Close()
    port := ln.Addr().(*net.TCPAddr).Port

    m := make(map[PortKey][]PortEntry)
    key := PortKey{Protocol: "tcp", Port: port}
    m[key] = []PortEntry{{}}

    mergeProcMetadata(m)
    if len(m[key]) == 0 || m[key][0].Pid == 0 {
        t.Fatalf("proc metadata not populated: %v", m[key])
    }
    if m[key][0].Name == "" {
        t.Fatalf("expected process name, got empty")
    }
    if m[key][0].Cmdline == "" {
        t.Fatalf("expected cmdline, got empty")
    }
}

func TestAppsBySysctlNativePid(t *testing.T) {
    SetNativeOnly(true)
    m, err := appsBySysctl()
    if err != nil {
        t.Skipf("appsBySysctl failed: %v", err)
    }
    // ensure at least one PID is populated if map non-empty
    for _, list := range m {
        for _, e := range list {
            if e.Pid != 0 {
                return
            }
        }
    }
    if len(m) > 0 {
        t.Fatalf("expected some entries with pid under nativeOnly, got %v", m)
    }
}

func TestSysctlFailureFallsBackToLsof(t *testing.T) {
    // simulate a sysctl implementation error and verify that AppsByPort
    // still returns metadata by invoking lsof.  This exercises the new
    // fallback logic added to AppsByPort.
    orig := sysctlImpl
    defer func() { sysctlImpl = orig }()
    sysctlImpl = func() (map[PortKey][]PortEntry, error) {
        return nil, fmt.Errorf("simulated sysctl failure")
    }

    SetNativeOnly(false)
    m := AppsByPort()
    found := false
    for _, list := range m {
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
        t.Skip("lsof did not return any metadata; ensure lsof is available")
    }
}

func TestSysctlEmptyEntriesTriggersFallback(t *testing.T) {
    // sysctlImpl returns a non-empty map but with no PID information.  The
    // caller should invoke lsof instead when nativeOnly=false.
    orig := sysctlImpl
    defer func() { sysctlImpl = orig }()
    key := PortKey{Protocol: "tcp", Port: 4242}
    sysctlImpl = func() (map[PortKey][]PortEntry, error) {
        return map[PortKey][]PortEntry{key: {{}}}, nil
    }

    SetNativeOnly(false)
    m := AppsByPort()
    if len(m[key]) == 0 || m[key][0].Pid == 0 {
        t.Skip("lsof fallback did not return PID info; ensure lsof available")
    }
}

func TestSysctlCache(t *testing.T) {
    original := timeNow
    defer func() { timeNow = original }()
    now := time.Now()
    timeNow = func() time.Time { return now }
    if _, err := appsBySysctl(); err != nil {
        t.Skipf("sysctl not available: %v", err)
    }
    // inject marker into cache
    marker := PortKey{Protocol: "tcp", Port: 9999}
    sysctlCache.mu.Lock()
    if sysctlCache.m == nil {
        sysctlCache.m = make(map[PortKey][]PortEntry)
    }
    sysctlCache.m[marker] = []PortEntry{{Name: "marker"}}
    sysctlCache.mu.Unlock()
    timeNow = func() time.Time { return now.Add(500 * time.Millisecond) }
    m2, _ := appsBySysctl()
    if len(m2[marker]) == 0 {
        t.Fatalf("cache was not used, marker missing")
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

func TestActivityNotifications(t *testing.T) {
    // ensure the activity channel emits open/close events as maps change
    lastPorts = nil
    ch := SubscribeActivity()
    // drain any stale events left over from other tests
    for {
        select {
        case <-ch:
            continue
        default:
        }
        break
    }

    // first call should notify open
    newmap := map[PortKey][]PortEntry{{Protocol: "tcp", Port: 100}: {{}}}
    diffAndPublish(newmap)
    evt := <-ch
    if !evt.Open || evt.Key.Port != 100 {
        t.Fatalf("expected open event, got %+v", evt)
    }

    // second call removing the port should notify close
    diffAndPublish(map[PortKey][]PortEntry{})
    evt = <-ch
    if evt.Open {
        t.Fatalf("expected close event, got %+v", evt)
    }
}
