// Package ports discovers listening TCP ports and enriches them with process metadata.
package ports

import (
    "context"
    "fmt"
    "net"
    "os/exec"
    "path/filepath"
    "runtime"
    "strconv"
    "strings"
    "sync"
    "time"
)

// PortEntry holds information about a single process listening on a port or
// socket.  Fields mirror the expectations of the CLI/GUI renderers and are
// also convenient for aggregation by protocol/port.
//
// Host and AppBundle are both resolved lazily: Host is obtained by performing
// a reverse DNS lookup on the listening address, and AppBundle is the macOS
// bundle identifier (if known) for the process.  Additional fields (CPU/Mem)
// are reserved for future statistics enhancements.
//
// The new Protocol field distinguishes TCP/UDP (and eventually other types),
// which is required now that the tool understands more than just TCP.
//
// Any field may be empty; callers should treat missing data as non‑fatal.
type PortEntry struct {
    Pid       int32   // process ID
    Name      string  // process name
    Cmdline   string  // full command line
    Host      string  // resolved hostname for listening interface
    AppBundle string  // application bundle identifier (if macOS)
    Protocol  string  // "tcp", "udp", etc.
    Family    string  // address family as reported by lsof ("IPv4"/"IPv6")
    CPU       float64 // CPU usage percentage
    Mem       uint64  // RSS memory in bytes
}

// bundleIDForPID attempts to determine a running process's bundle
// identifier by invoking `ps` to get its command path and passing that path to
// `mdls`.  A blank string is returned on failure.
// The real bundle lookup is performed by bundleIDForPID.  Tests may
// override bundleIDFunc to avoid executing `ps`/`mdls`.
var bundleIDFunc = bundleIDForPID

func bundleIDForPID(pid int32) string {
    if v, ok := bundleCache.Load(pid); ok {
        return v.(string)
    }
    out, err := runCmd("ps", "-p", fmt.Sprintf("%d", pid), "-o", "comm=")
    if err != nil {
        bundleCache.Store(pid, "")
        return ""
    }
    path := strings.TrimSpace(string(out))
    if path == "" || !strings.HasPrefix(path, "/") {
        return ""
    }
    // climb until we reach an .app bundle, because mdls on the binary itself
    // doesn't expose the CFBundleIdentifier.
    dir := path
    for {
        if strings.HasSuffix(dir, ".app") {
            break
        }
        parent := filepath.Dir(dir)
        if parent == dir {
            dir = ""
            break
        }
        dir = parent
    }
    if dir == "" {
        return ""
    }
    if bid, err := runCmd("mdls", "-name", "kMDItemCFBundleIdentifier", "-raw", dir); err == nil {
        s := strings.TrimSpace(string(bid))
        if s != "(null)" {
            return s
        }
    }
    return ""
}

// lookupAddrFunc is used by resolveHost and can be swapped in tests to
// avoid real network lookups.
var lookupAddrFunc = net.LookupAddr

// caches to avoid repeated DNS and bundle lookups during a single run.
// sync.Map is used for simplicity; the data set is very small so contention
// is negligible.
var hostCache sync.Map    // map[string]string
var bundleCache sync.Map  // map[int32]string

// resolveHost performs a reverse DNS lookup on the address portion of an
// lsof NAME field (e.g. "127.0.0.1:80" or "[::1]:8000").  If resolution
// succeeds the first hostname is returned sans trailing dot.  Wildcards
// ("*" or "*") yield empty.  Bracketed IPv6 addresses are stripped prior
// to lookup.
func resolveHost(address string) string {
    colon := strings.LastIndex(address, ":")
    if colon == -1 {
        return ""
    }
    host := address[:colon]
    if host == "*" {
        return ""
    }
    // strip surrounding brackets for IPv6
    if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
        host = host[1 : len(host)-1]
    }
    if v, ok := hostCache.Load(host); ok {
        return v.(string)
    }
    names, err := lookupAddrFunc(host)
    result := ""
    if err == nil && len(names) > 0 {
        result = strings.TrimSuffix(names[0], ".")
    }
    hostCache.Store(host, result)
    return result
}

// commandTimeout is used when invoking external utilities.  It is intentionally
// short to avoid a hung tool locking the entire app; callers may override
// during testing if necessary.
var commandTimeout = 2 * time.Second

// runCmd executes a command with the global timeout and returns its output.
// It is a small wrapper around exec.CommandContext for convenience.
func runCmd(name string, args ...string) ([]byte, error) {
    ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
    defer cancel()
    return exec.CommandContext(ctx, name, args...).Output()
}

// PortKey identifies a unique listening port endpoint.  The combination of
// protocol and port number is required now that we track both TCP and UDP
// sockets (future expansions may include unix domain sockets, etc.).
// The type is comparable so it can be used directly as a map key.
type PortKey struct {
    Protocol string // e.g. "tcp", "udp"
    Port     int
}

// parseHexIP converts the /proc/net hex IP representation to a numeric
// address string.  IPv6 entries are 32 hex digits.
func parseHexIP(hex string) string {
    if len(hex) == 8 {
        // IPv4 little-endian
        b := make([]byte, 4)
        for i := 0; i < 4; i++ {
            x, _ := strconv.ParseUint(hex[i*2:i*2+2], 16, 8)
            b[3-i] = byte(x)
        }
        return net.IP(b).String()
    }
    if len(hex) == 32 {
        // IPv6
        b := make([]byte, 16)
        for i := 0; i < 16; i++ {
            x, _ := strconv.ParseUint(hex[i*2:i*2+2], 16, 8)
            b[i] = byte(x)
        }
        return net.IP(b).String()
    }
    return ""
}

func (k PortKey) String() string {
    return fmt.Sprintf("%s/%d", k.Protocol, k.Port)
}

// AppsByPort returns a mapping from PortKey to a slice of PortEntry instances
// representing processes bound to that socket.  We shell out to `lsof` on
// macOS because Go has no cross‑platform introspection API; the command now
// requests both TCP listeners and all UDP sockets.  Lines are handed off to
// parseLsof so the behavior is testable.
// AppsByPort returns all listening sockets, keyed by protocol/port.  The
// implementation is intentionally thin: it retrieves raw data from
// discoverPorts, then parses it.  Eventually discoverPorts will have
// OS-specific implementations so other platforms can be supported without
// reworking the parsing logic.
// nativeOnly toggles whether we may shell out to external helpers such as
// lsof in order to enrich results.  A CLI flag can be used to set this when
// the user wants a purely native stack.
var nativeOnly bool

// SetNativeOnly is used by callers (typically CLI/GUI) to force discovery
// without invoking lsof.  Tests may also toggle it.
func SetNativeOnly(v bool) {
    nativeOnly = v
}

// NativeOnlyEnabled reports whether discovery is restricted to native
// mechanisms.  It exists primarily for tests and CLI/GUI integration.
func NativeOnlyEnabled() bool {
    return nativeOnly
}

func AppsByPort() map[PortKey][]PortEntry {
    portsMap := make(map[PortKey][]PortEntry)

    // darwin has a native path using sysctl; if that fails we try netstat
    // as a backup before falling back to lsof.  The nativeOnly flag prevents
    // us from ever invoking lsof.
    if runtime.GOOS == "darwin" {
        if m, err := appsBySysctl(); err == nil && len(m) > 0 {
            return m
        }
        if m, err := appsByNetstat(); err == nil && len(m) > 0 {
            return m
        }
        if nativeOnly {
            return portsMap
        }
        // fall through to lsof if both native attempts failed
    }

    // linux also has a native path using /proc; appsByProc is defined in
    // ports_linux_impl.go and will only exist when building for linux.
    if runtime.GOOS == "linux" {
        if m, err := appsByProc(); err == nil && len(m) > 0 {
            return m
        }
        if nativeOnly {
            return portsMap
        }
    }
    // windows uses Win32 APIs to enumerate listeners; appsByWin lives in
    // ports_windows_impl.go and is only available on that platform.
    if runtime.GOOS == "windows" {
        if m, err := appsByWin(); err == nil && len(m) > 0 {
            return m
        }
        if nativeOnly {
            return portsMap
        }
    }

    out, err := discoverPorts()
    if err != nil {
        return portsMap
    }
    return parseLsof(out)
}

// discoverPorts is implemented per-OS in other files.  The darwin version
// shells out to lsof; linux will eventually parse /proc/net, etc.  A generic
// stub (ports_stub.go) returns an error to indicate the platform is not
// supported.
//
// func discoverPorts() ([]byte, error)

// parseLsof consumes the raw output of an `lsof` invocation and returns the
// corresponding ports map.  It is exported indirectly via AppsByPort but also
// used in unit tests with synthetic data.
func parseLsof(out []byte) map[PortKey][]PortEntry {
    portsMap := make(map[PortKey][]PortEntry)

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
        // lsof columns: COMMAND PID USER FD TYPE DEVICE SIZE/OFF NODE NAME
        cmdName := fields[0]
        pidStr := fields[1]
        proto := strings.ToLower(fields[7]) // NODE column contains "TCP" or "UDP"
        family := fields[4]                    // TYPE column contains "IPv4" or "IPv6"

        // determine address field; skip trailing "(LISTEN)" token if present
        nameField := fields[len(fields)-1]
        if nameField == "(LISTEN)" && len(fields) >= 2 {
            nameField = fields[len(fields)-2]
        }

        // extract port number; works with IPv6 bracketed addresses as well
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

        // obtain full command line via ps; ignore errors
        cmdline := cmdName
        if pid > 0 {
            if cl, err := runCmd("ps", "-p", fmt.Sprintf("%d", pid), "-o", "command="); err == nil {
                cmdline = strings.TrimSpace(string(cl))
            }
        }

        entry := PortEntry{
            Pid:       pid,
            Name:      cmdName,
            Cmdline:   cmdline,
            Host:      resolveHost(nameField),
            AppBundle: bundleIDFunc(pid),
            Protocol:  proto,
            Family:    family,
        }
        key := PortKey{Protocol: proto, Port: portNum}
        portsMap[key] = append(portsMap[key], entry)
    }

    return portsMap
}
