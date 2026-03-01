// Package ports discovers listening TCP ports and enriches them with process metadata.
package ports

import (
    "fmt"
    "net"
    "os/exec"
    "strings"
)

// PortEntry holds information about a single process listening on a port.
// The fields mirror the expectations of the CLI renderer and allow callers to
// aggregate by port number.
//
// Host and AppBundle are both resolved lazily: Host is obtained by performing
// a reverse DNS lookup on the local address, and AppBundle is the macOS bundle
// identifier (if known) for the process.  CPU and Mem are reserved for future
// statistics enhancements.
//
// Any field may be empty; callers should treat missing data as non‑fatal.
type PortEntry struct {
    Pid       int32   // process ID
    Name      string  // process name
    Cmdline   string  // full command line
    Host      string  // resolved hostname for listening interface
    AppBundle string  // application bundle identifier (if macOS)
    CPU       float64 // CPU usage percentage
    Mem       uint64  // RSS memory in bytes
}

// bundleIDForPID attempts to determine a running process's bundle
// identifier by invoking `ps` to get its command path and passing that path to
// `mdls`.  A blank string is returned on failure.
func bundleIDForPID(pid int32) string {
    out, err := exec.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "comm=").Output()
    if err != nil {
        return ""
    }
    path := strings.TrimSpace(string(out))
    if path == "" || !strings.HasPrefix(path, "/") {
        return ""
    }
    if bid, err := exec.Command("mdls", "-name", "kMDItemCFBundleIdentifier", "-raw", path).Output(); err == nil {
        s := strings.TrimSpace(string(bid))
        if s != "(null)" {
            return s
        }
    }
    return ""
}

// resolveHost performs a reverse DNS lookup on the address portion of an
// lsof NAME field (e.g. "127.0.0.1:80").  If resolution succeeds the first
// hostname is returned sans trailing dot.  Wildcards ("*") yield empty.
func resolveHost(address string) string {
    colon := strings.LastIndex(address, ":")
    if colon == -1 {
        return ""
    }
    host := address[:colon]
    if host == "*" {
        return ""
    }
    names, err := net.LookupAddr(host)
    if err == nil && len(names) > 0 {
        return strings.TrimSuffix(names[0], ".")
    }
    return ""
}

// AppsByPort returns a mapping from listening port numbers to a slice of
// PortEntry instances representing processes bound to that port.  On macOS
// we shell out to `lsof` because Go does not provide a cross‑platform API for
// enumerating listening sockets.  The output is parsed and then enriched with
// a full command line pulled from `ps` so the CLI/table and GUI can display
// something useful.
//
// If anything goes wrong we silently return an empty map; callers are expected
// to handle that gracefully (the GUI already does).
func AppsByPort() map[int][]PortEntry {
    portsMap := make(map[int][]PortEntry)

    out, err := exec.Command("lsof", "-nP", "-iTCP", "-sTCP:LISTEN").Output()
    if err != nil {
        return portsMap
    }

    lines := strings.Split(string(out), "\n")
    if len(lines) <= 1 {
        return portsMap
    }

    for _, line := range lines[1:] {
        if line == "" {
            continue
        }
        fields := strings.Fields(line)
        if len(fields) < 9 {
            continue
        }
        // lsof output columns: COMMAND PID USER FD TYPE DEVICE SIZE/OFF NODE NAME
        cmdName := fields[0]
        pidStr := fields[1]
        // lsof typically appends a final "(LISTEN)" token; if present
        // ignore it and take the preceding field as the address.
        nameField := fields[len(fields)-1]
        if nameField == "(LISTEN)" && len(fields) >= 2 {
            nameField = fields[len(fields)-2]
        }

        // extract port from NAME column (e.g. "*:8080" or "127.0.0.1:80").
        // there may be IPv6 addresses like "[::1]:8000"; use LastIndex.
        colon := strings.LastIndex(nameField, ":")
        if colon == -1 {
            continue
        }
        portNum := 0
        fmt.Sscanf(nameField[colon+1:], "%d", &portNum)
        if portNum == 0 {
            continue
        }

        pid := int32(0)
        fmt.Sscanf(pidStr, "%d", &pid)

        // obtain full command line via ps
        cmdline := cmdName
        if pid > 0 {
            if cl, err := exec.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "command=").Output(); err == nil {
                cmdline = strings.TrimSpace(string(cl))
            }
        }

        entry := PortEntry{
            Pid:     pid,
            Name:    cmdName,
            Cmdline: cmdline,
            Host:    resolveHost(nameField),
            AppBundle: bundleIDForPID(pid),
        }
        portsMap[portNum] = append(portsMap[portNum], entry)
    }

    return portsMap
}
