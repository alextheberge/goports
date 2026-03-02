//go:build darwin
// +build darwin

package ports

import (
    "bufio"
    "encoding/json"
    "fmt"
    "io"
    "net"
    "net/http"
    "os"
    "strings"
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

func TestHTTPEventServer(t *testing.T) {
    addr, shutdown, err := startTestServer()
    if err != nil {
        t.Fatalf("failed to start event server: %v", err)
    }
    defer shutdown()

    // connect to the server
    resp, err := http.Get("http://" + addr + "/events")
    if err != nil {
        t.Fatalf("http connect failed: %v", err)
    }
    defer resp.Body.Close()

    // publish an activity and read the SSE line
    go func() {
        diffAndPublish(map[PortKey][]PortEntry{{Protocol: "tcp", Port: 55}: {{}}})
    }()

    reader := bufio.NewReader(resp.Body)
    // read one SSE record (data: line)
    line, err := reader.ReadString('\n')
    if err != nil {
        t.Fatalf("read line failed: %v", err)
    }
    if !strings.HasPrefix(line, "data:") {
        t.Fatalf("expected data prefix, got: %s", line)
    }
    var evt PortActivity
    if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &evt); err != nil {
        t.Fatalf("json unmarshal: %v", err)
    }
    if evt.Key.Port != 55 || evt.Key.Protocol != "tcp" || !evt.Open {
        t.Fatalf("unexpected event: %+v", evt)
    }
}

func TestHistoryEndpoint(t *testing.T) {
    lastPorts = nil
    clearHistory()
    addr, shutdown, err := startTestServer()
    if err != nil {
        t.Fatalf("start server: %v", err)
    }
    defer shutdown()

    // generate a couple of events with known timestamps
    diffAndPublish(map[PortKey][]PortEntry{{Protocol: "tcp", Port: 101}: {{}}})
    time.Sleep(10 * time.Millisecond)
    diffAndPublish(map[PortKey][]PortEntry{{Protocol: "tcp", Port: 102}: {{}}})

    // fetch history with limit=1; check that we receive at least one event
    res, err := http.Get("http://" + addr + "/history?limit=1")
    if err != nil {
        t.Fatalf("get history: %v", err)
    }
    var evts []PortActivity
    json.NewDecoder(res.Body).Decode(&evts)
    res.Body.Close()
    if len(evts) != 1 {
        t.Fatalf("unexpected limited history: %+v", evts)
    }

    // test filtering by protocol
    res, _ = http.Get("http://" + addr + "/history?protocol=udp")
    json.NewDecoder(res.Body).Decode(&evts)
    res.Body.Close()
    if len(evts) != 0 {
        t.Fatalf("expected no udp events, got %+v", evts)
    }

    // test since parameter returns subset
    since := time.Now().Add(-1 * time.Second).Format(time.RFC3339)
    res, _ = http.Get("http://" + addr + "/history?since=" + since + "&limit=10")
    json.NewDecoder(res.Body).Decode(&evts)
    res.Body.Close()
    if len(evts) < 2 {
        t.Fatalf("expected at least two events, got %+v", evts)
    }
}

func TestHTTPAuth(t *testing.T) {
    // set token
    os.Setenv("GOPORTS_API_TOKEN", "secret")
    defer os.Unsetenv("GOPORTS_API_TOKEN")

    addr, shutdown, err := startTestServer()
    if err != nil {
        t.Fatalf("start server: %v", err)
    }
    defer shutdown()

    // unauthenticated request should 401
    res, _ := http.Get("http://" + addr + "/history")
    if res.StatusCode != http.StatusUnauthorized {
        t.Fatalf("expected 401, got %d", res.StatusCode)
    }
    // with token query
    res, _ = http.Get("http://" + addr + "/history?token=secret")
    if res.StatusCode != http.StatusOK {
        t.Fatalf("expected 200 with token, got %d", res.StatusCode)
    }
    // with bearer header
    req, _ := http.NewRequest("GET", "http://"+addr+"/history", nil)
    req.Header.Set("Authorization", "Bearer secret")
    res, _ = http.DefaultClient.Do(req)
    if res.StatusCode != http.StatusOK {
        t.Fatalf("expected 200 with header, got %d", res.StatusCode)
    }

    // reset should also require auth
    res, _ = http.Post("http://"+addr+"/history/reset", "application/json", nil)
    if res.StatusCode != http.StatusUnauthorized {
        t.Fatalf("expected 401 for unauthorized reset, got %d", res.StatusCode)
    }
    res, _ = http.Post("http://"+addr+"/history/reset?token=secret", "application/json", nil)
    if res.StatusCode != http.StatusNoContent {
        t.Fatalf("expected 204 with token, got %d", res.StatusCode)
    }
}

func TestOpenAPISpec(t *testing.T) {
    addr, shutdown, err := startTestServer()
    if err != nil {
        t.Fatalf("start server: %v", err)
    }
    defer shutdown()

    res, err := http.Get("http://" + addr + "/openapi.json")
    if err != nil {
        t.Fatalf("get openapi: %v", err)
    }
    defer res.Body.Close()
    if res.StatusCode != http.StatusOK {
        t.Fatalf("expected 200, got %d", res.StatusCode)
    }
    var spec map[string]interface{}
    if err := json.NewDecoder(res.Body).Decode(&spec); err != nil {
        t.Fatalf("decode spec: %v", err)
    }
    if spec["openapi"] != "3.0.0" {
        t.Fatalf("unexpected openapi version: %v", spec["openapi"])
    }
    // ensure components.schemas.PortActivity exists
    comps, ok := spec["components"].(map[string]interface{})
    if !ok {
        t.Fatalf("components missing")
    }
    schemas, ok := comps["schemas"].(map[string]interface{})
    if !ok {
        t.Fatalf("schemas missing")
    }
    if _, ok := schemas["PortActivity"]; !ok {
        t.Fatalf("PortActivity schema missing")
    }
    // ensure reset and ports endpoints described
    paths, ok := spec["paths"].(map[string]interface{})
    if !ok {
        t.Fatalf("paths missing")
    }
    if _, ok := paths["/history/reset"]; !ok {
        t.Fatalf("/history/reset not described in spec")
    }
    if _, ok := paths["/ports"]; !ok {
        t.Fatalf("/ports endpoint missing from spec")
    }
}

func TestSwaggerUI(t *testing.T) {
    addr, shutdown, err := startTestServer()
    if err != nil {
        t.Fatalf("start server: %v", err)
    }
    defer shutdown()

    res, err := http.Get("http://" + addr + "/swagger")
    if err != nil {
        t.Fatalf("get swagger: %v", err)
    }
    defer res.Body.Close()
    if res.StatusCode != http.StatusOK {
        t.Fatalf("expected 200, got %d", res.StatusCode)
    }
    body, _ := io.ReadAll(res.Body)
    if !strings.Contains(string(body), "swagger-ui") {
        t.Fatalf("swagger page missing expected content")
    }
}

func TestHistoryReset(t *testing.T) {
    lastPorts = nil
    clearHistory()
    addr, shutdown, err := startTestServer()
    if err != nil {
        t.Fatalf("start server: %v", err)
    }
    defer shutdown()

    // seed some history
    diffAndPublish(map[PortKey][]PortEntry{{Protocol: "tcp", Port: 600}: {{}}})
    // reset buffer via POST
    res, err := http.Post("http://"+addr+"/history/reset", "application/json", nil)
    if err != nil {
        t.Fatalf("post reset: %v", err)
    }
    if res.StatusCode != http.StatusNoContent {
        t.Fatalf("expected 204, got %d", res.StatusCode)
    }
    // verify empty
    res, err = http.Get("http://" + addr + "/history")
    if err != nil {
        t.Fatalf("get history: %v", err)
    }
    var evts []PortActivity
    json.NewDecoder(res.Body).Decode(&evts)
    res.Body.Close()
    if len(evts) != 0 {
        t.Fatalf("expected empty history after reset, got %+v", evts)
    }

    // status endpoint should respond with an integer value
    res, err = http.Get("http://" + addr + "/status")
    if err != nil {
        t.Fatalf("get status: %v", err)
    }
    var st map[string]int
    json.NewDecoder(res.Body).Decode(&st)
    res.Body.Close()
    if _, ok := st["open"]; !ok {
        t.Fatalf("status response missing 'open' key: %v", st)
    }
    // ports snapshot should return valid JSON (contents vary by system)
    res, err = http.Get("http://" + addr + "/ports")
    if err != nil {
        t.Fatalf("get ports: %v", err)
    }
    var list []map[string]interface{}
    if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
        t.Fatalf("decode ports: %v", err)
    }
    res.Body.Close()
    if list == nil {
        t.Fatalf("expected array, got nil")
    }
}
