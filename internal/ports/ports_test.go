package ports

import (
    "sync"
    "testing"
    "time"
)

func TestParseLsof(t *testing.T) {
    // stub out external dependencies to make the test deterministic
    lookupAddrFunc = func(addr string) ([]string, error) { return []string{addr + ".host"}, nil }
    originalBundle := bundleIDFunc
    bundleIDFunc = func(pid int32) string { return "bundle" }
    defer func() { bundleIDFunc = originalBundle }()

    raw := `COMMAND     PID USER   FD   TYPE             DEVICE SIZE/OFF NODE NAME
` +
        `foo        111 alice 3u  IPv4 0x1234      0t0  TCP *:8080 (LISTEN)
` +
        `bar        222 bob   4u  IPv6 0x2345      0t0  UDP 127.0.0.1:53
` +
        `baz        333 carol 5u  IPv6 0x3456      0t0  TCP [::1]:443 (LISTEN)
` +
        `bogus      444 dave  1u  IPv4 0x4567      0t0  UDP *:*` // wildcard port

    m := parseLsof([]byte(raw))

    // expect three non-wild entries
    if len(m) != 3 {
        t.Fatalf("expected 3 keys, got %d", len(m))
    }

    check := func(proto string, port int) {
        key := PortKey{Protocol: proto, Port: port}
        ents, ok := m[key]
        if !ok {
            t.Fatalf("missing key %v", key)
        }
        if len(ents) != 1 {
            t.Fatalf("expected 1 entry for %v, got %d", key, len(ents))
        }
        e := ents[0]
        if e.Protocol != proto {
            t.Errorf("proto mismatch: have %q want %q", e.Protocol, proto)
        }
        if e.AppBundle != "bundle" {
            t.Errorf("expected stubbed bundle, got %q", e.AppBundle)
        }
    }
    check("tcp", 8080)
    check("udp", 53)
    check("tcp", 443)

    // ensure host values are trimmed/lookup applied
    // wildcards should yield empty host string
    if m[PortKey{Protocol: "tcp", Port: 8080}][0].Host != "" {
        t.Errorf("unexpected host for wildcard: %q", m[PortKey{Protocol: "tcp", Port: 8080}][0].Host)
    }
    // IPv6 loopback should strip brackets, lookup stub returns ::1.host
    if m[PortKey{Protocol: "tcp", Port: 443}][0].Host != "::1.host" {
        t.Errorf("unexpected host for ipv6: %q", m[PortKey{Protocol: "tcp", Port: 443}][0].Host)
    }
}

func TestCaching(t *testing.T) {
    // reset caches by overwriting (not exported but accessible within package)
    hostCache = sync.Map{}
    bundleCache = sync.Map{}

    calls := 0
    lookupAddrFunc = func(a string) ([]string, error) {
        calls++
        return []string{"r"}, nil
    }

    _ = resolveHost("1.2.3.4:80")
    _ = resolveHost("1.2.3.4:81") // should hit cache
    if calls != 1 {
        t.Errorf("expected 1 DNS call, got %d", calls)
    }

    // bundle cache behaviour is exercised indirectly in production code
    // and is not easily validated with a simple unit test since it depends on
    // executing `ps`; the primary performance win comes from DNS caching above.
}

func TestRunCmdTimeout(t *testing.T) {
    // set very short timeout and expect runCmd to fail
    original := commandTimeout
    commandTimeout = 1 * time.Millisecond
    defer func() { commandTimeout = original }()

    _, err := runCmd("sleep", "0.1")
    if err == nil {
        t.Errorf("expected error due to timeout")
    }
}

func TestParseHexIP(t *testing.T) {
    if parseHexIP("0100007F") != "127.0.0.1" {
        t.Error("ipv4 parse failed")
    }
    // IPv6 loopback corresponds to 00000000000000000000000000000001 in /proc/net.
    if parseHexIP("00000000000000000000000000000001") != "::1" {
        t.Error("ipv6 parse failed")
    }
}
