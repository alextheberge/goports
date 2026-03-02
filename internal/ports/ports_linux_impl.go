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
)

// AppsByPort is a Linux-specific implementation that parses /proc/net/*
// directly, avoiding a dependency on lsof.  It returns a map similar to the
// default implementation; pid and Name fields may be empty if the inode lookup
// fails.  If an error occurs during the native scan we fall back to the
// generic lsof-based discovery.
func AppsByPort() map[PortKey][]PortEntry {
    if m, err := appsByProc(); err == nil && len(m) > 0 {
        return m
    }
    // fallback to earlier behavior
    portsMap := make(map[PortKey][]PortEntry)
    out, err := discoverPorts()
    if err != nil {
        return portsMap
    }
    return parseLsof(out)
}

// appsByProc tries to enumerate listening sockets via /proc.
func appsByProc() (map[PortKey][]PortEntry, error) {
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
            entry := PortEntry{Protocol: file.proto, Port: 0}
            portKey := PortKey{Protocol: file.proto, Port: int(portVal)}

            // resolve pid if we have an inode mapping
            if pid, ok := inodeToPid[inode]; ok {
                entry.Pid = pid
                // try to load process name from /proc/<pid>/comm
                if name, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid)); err == nil {
                    entry.Name = strings.TrimSpace(string(name))
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
    return result, nil
}
