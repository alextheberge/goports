//go:build linux
// +build linux

package ports

import (
    "bufio"
    "fmt"
    "net"
    "os"
    "path/filepath"
    "strconv"
    "strings"
    "sync"
    "testing"
    "time"
)

// appsByProc is a Linux-specific helper that parses /proc/net/*
// directly, avoiding a dependency on lsof.  It returns a map similar to the
// default implementation; pid and Name fields may be empty if the inode lookup
// fails.  When built for linux the generic AppsByPort in ports.go will call
// this helper before falling back to the shell-out path.
// caching for appsByProc
var procCache struct {
    mu sync.Mutex
    m  map[PortKey][]PortEntry
    ts time.Time
}

func appsByProc() (map[PortKey][]PortEntry, error) {
    procCache.mu.Lock()
    if procCache.m != nil && timeNow().Sub(procCache.ts) < cacheDuration {
        res := make(map[PortKey][]PortEntry, len(procCache.m))
        for k, v := range procCache.m {
            res[k] = append([]PortEntry(nil), v...)
        }
        procCache.mu.Unlock()
        return res, nil
    }
    procCache.mu.Unlock()

    inodeToPid := make(map[string]int32)
    // build inode->pid map by scanning /proc/[0-9]*/fd
    filepath.WalkDir("/proc", func(path string, d os.DirEntry, err error) error {
        if err != nil || !d.IsDir() {
            return nil
        }
        parts := strings.Split(path, "/")
        if len(parts) < 3 {
            return nil
        }
        pid, perr := strconv.Atoi(parts[2])
        if perr != nil {
            return nil // not a numeric directory
        }
        fdDir := filepath.Join(path, "fd")
        entries, err := os.ReadDir(fdDir)
        if err != nil {
            return nil
        }
        for _, e := range entries {
            link, err := os.Readlink(filepath.Join(fdDir, e.Name()))
            if err != nil {
                continue
            }
            if strings.HasPrefix(link, "socket:[") && strings.HasSuffix(link, "]") {
                inode := link[len("socket:[") : len(link)-1]
                inodeToPid[inode] = int32(pid)
            }
        }
        return nil
    })

    result := make(map[PortKey][]PortEntry)
    files := []struct{
        proto string
        path  string
    }{
        {"tcp", "/proc/net/tcp"},
        {"tcp", "/proc/net/tcp6"},
        {"udp", "/proc/net/udp"},
        {"udp", "/proc/net/udp6"},
    }
    for _, file := range files {
        f, err := os.Open(file.path)
        if err != nil {
            // if file missing just skip
            continue
        }
        scanner := bufio.NewScanner(f)
        // skip header
        if scanner.Scan() {
            // header line
        }
        for scanner.Scan() {
            fields := strings.Fields(scanner.Text())
            if len(fields) < 10 {
                continue
            }
            local := fields[1]
            state := fields[3]
            inode := fields[9]
            // for TCP only include LISTEN state (0A)
            if file.proto == "tcp" && state != "0A" {
                continue
            }
            // parse port
            colon := strings.LastIndex(local, ":")
            if colon == -1 {
                continue
            }
            portHex := local[colon+1:]
            portVal, err := strconv.ParseInt(portHex, 16, 32)
            if err != nil || portVal == 0 {
                continue
            }
            entry := PortEntry{Protocol: file.proto, Family: "IPv4"}
            portKey := PortKey{Protocol: file.proto, Port: int(portVal)}
            if strings.Contains(file.path, "6") {
                entry.Family = "IPv6"
            }

            // resolve pid if we have an inode mapping
            if pid, ok := inodeToPid[inode]; ok {
                entry.Pid = pid
                // try to load process name from /proc/<pid>/comm
                if name, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid)); err == nil {
                    entry.Name = strings.TrimSpace(string(name))
                }
                // also attempt to read full command line (nul-separated)
                if cmd, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid)); err == nil {
                    entry.Cmdline = strings.ReplaceAll(strings.TrimRight(string(cmd), "\x00"), "\x00", " ")
                }
            }

            // host resolution by converting hex ip to dotted or ipv6
            addrPart := local[:colon]
            host := parseHexIP(addrPart)
            if host != "" {
                entry.Host = resolveHost(host + ":" + strconv.FormatInt(portVal,10))
            }

            result[portKey] = append(result[portKey], entry)
        }
        f.Close()
    }
    // store in cache
    procCache.mu.Lock()
    procCache.m = make(map[PortKey][]PortEntry, len(result))
    for k, v := range result {
        procCache.m[k] = append([]PortEntry(nil), v...)
    }
    procCache.ts = timeNow()
    procCache.mu.Unlock()
    return result, nil
}
// The following test exercises appsByProc to ensure PID and command-line
// information are attached.  It opens a temporary listener and looks for it in
// the map.

func TestAppsByProcMetadata(t *testing.T) {
    ln, err := net.Listen("tcp4", "127.0.0.1:0")
    if err != nil {
        t.Skipf("unable to listen: %v", err)
    }
    defer ln.Close()
    port := ln.Addr().(*net.TCPAddr).Port
    time.Sleep(100 * time.Millisecond)

    m, err := appsByProc()
    if err != nil {
        t.Skipf("appsByProc failed: %v", err)
    }
    found := false
    for k, ents := range m {
        if k.Protocol == "tcp" && k.Port == port {
            for _, e := range ents {
                if e.Pid != 0 && e.Cmdline != "" {
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
        t.Fatalf("expected metadata for %d, got %v", port, m)
    }
}
// cache validation for appsByProc
func TestProcCache(t *testing.T) {
    original := timeNow
    defer func() { timeNow = original }()
    now := time.Now()
    timeNow = func() time.Time { return now }
    if _, err := appsByProc(); err != nil {
        t.Skipf("appsByProc failed: %v", err)
    }
    marker := PortKey{Protocol: "udp", Port: 54321}
    procCache.mu.Lock()
    if procCache.m == nil {
        procCache.m = make(map[PortKey][]PortEntry)
    }
    procCache.m[marker] = []PortEntry{{Name: "marker"}}
    procCache.mu.Unlock()
    timeNow = func() time.Time { return now.Add(500 * time.Millisecond) }
    m2, _ := appsByProc()
    if len(m2[marker]) == 0 {
        t.Fatalf("proc cache was not used, marker missing")
    }
}
