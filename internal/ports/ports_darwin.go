//go:build darwin
// +build darwin

package ports

/*
#include <stdlib.h>
#include <sys/types.h>
#include <sys/sysctl.h>
#include <netinet/in_pcb.h>
#include <netinet/tcp_var.h>
#include <arpa/inet.h>
#include <libproc.h>
#include <sys/proc_info.h>
#include <sys/socket.h>
#include <netinet/in.h>
#include <string.h>

// procentry represents a listening socket associated with a process.  It is
// analogous to the goentry struct used for sysctl results, but carries a PID
// so we can match entries to processes without external tools.
struct procentry {
    pid_t pid;
    int proto;
    int port;
    uint32_t ip4;
    unsigned char ip6[16];
};

// gather a list of all socket endpoints owned by all processes.  The caller
// must free the returned array with free().  The routine is intentionally
// robust: errors scanning one process are ignored so that the complete set of
// sockets can still be returned.
int get_proc_sockets(struct procentry **out, int *count) {
    int bufbytes = proc_listpids(PROC_ALL_PIDS, 0, NULL, 0);
    if (bufbytes <= 0) {
        return -1;
    }
    pid_t *pids = malloc(bufbytes);
    if (!pids) return -1;
    int ret = proc_listpids(PROC_ALL_PIDS, 0, pids, bufbytes);
    if (ret <= 0) {
        free(pids);
        return -1;
    }
    int npids = ret / sizeof(pid_t);
    int cap = 256;
    *count = 0;
    *out = malloc(cap * sizeof(struct procentry));
    for (int i = 0; i < npids; i++) {
        pid_t pid = pids[i];
        if (pid == 0) continue;
        int fdbuf = proc_pidinfo(pid, PROC_PIDLISTFDS, 0, NULL, 0);
        if (fdbuf <= 0) continue;
        struct proc_fdinfo *fds = malloc(fdbuf);
        if (!fds) continue;
        if (proc_pidinfo(pid, PROC_PIDLISTFDS, 0, fds, fdbuf) <= 0) {
            free(fds);
            continue;
        }
        int nfds = fdbuf / sizeof(struct proc_fdinfo);
        for (int j = 0; j < nfds; j++) {
            if (fds[j].proc_fdtype != PROX_FDTYPE_SOCKET) continue;
            struct socket_fdinfo sfi;
            if (proc_pidfdinfo(pid, fds[j].proc_fd, PROC_PIDFDSOCKETINFO, &sfi, sizeof(sfi)) <= 0) {
                continue;
            }
            struct in_sockinfo *ini = &sfi.psi.soi_proto.pri_in;
            int port = ntohs((uint16_t)ini->insi_lport);
            if (port == 0) continue;
            int proto = sfi.psi.soi_protocol;
            uint32_t ip4 = 0;
            unsigned char ip6[16] = {0};
            if (ini->insi_vflag & INI_IPV6) {
                memcpy(ip6, &ini->insi_laddr.ina_6, sizeof(ip6));
            } else {
                ip4 = ini->insi_laddr.ina_46.i46a_addr4.s_addr;
            }
            if (*count >= cap) {
                cap *= 2;
                *out = realloc(*out, cap * sizeof(struct procentry));
            }
            (*out)[*count].pid = pid;
            (*out)[*count].proto = proto;
            (*out)[*count].port = port;
            (*out)[*count].ip4 = ip4;
            memcpy((*out)[*count].ip6, ip6, sizeof(ip6));
            (*count)++;
        }
        free(fds);
    }
    free(pids);
    return 0;
}
#include <string.h>

struct goentry { int proto; int port; uint32_t ip4; unsigned char ip6[16]; };

// parse a single pcblist buffer (tcp or udp) and append entries
static void parse_pcblist(void *buf, size_t size, int proto, struct goentry **out, int *count, int *cap) {
    char *ptr = buf, *end = (char*)buf + size;
    while (ptr < end) {
        struct xinpgen *xig = (struct xinpgen*)ptr;
        if (xig->xig_len <= sizeof(*xig)) break;
        struct xinpcb *xin = (struct xinpcb*)ptr;
        uint16_t lport = ntohs(xin->xi_inp.inp_lport);
        uint32_t lip = 0;
        unsigned char lip6[16] = {0};
        if (xin->xi_inp.inp_vflag & INP_IPV6) {
            // IPv6 local address is stored in inp_dependladdr.inp6_local
            memcpy(lip6, &xin->xi_inp.inp_dependladdr.inp6_local, sizeof(lip6));
        } else {
            lip = xin->xi_inp.inp_laddr.s_addr;
        }
        if (*count >= *cap) {
            *cap *= 2;
            *out = realloc(*out, (*cap) * sizeof(struct goentry));
        }
        (*out)[*count].proto = proto;
        (*out)[*count].port = lport;
        (*out)[*count].ip4 = lip;
        memcpy((*out)[*count].ip6, lip6, sizeof(lip6));
        (*count)++;
        ptr += xig->xig_len;
    }
}

int sysctl_pcblist(struct goentry **out, int *count) {
    *count = 0;
    int cap = 64;
    *out = malloc(cap * sizeof(struct goentry));
    if (!*out) return -1;
    const char *names[2] = {"net.inet.tcp.pcblist", "net.inet.udp.pcblist"};
    int prots[2] = {6, 17};
    for (int i = 0; i < 2; i++) {
        size_t len = 0;
        if (sysctlbyname(names[i], NULL, &len, NULL, 0) < 0) continue;
        void *buf = malloc(len);
        if (!buf) continue;
        if (sysctlbyname(names[i], buf, &len, NULL, 0) == 0) {
            parse_pcblist(buf, len, prots[i], out, count, &cap);
        }
        free(buf);
    }
    return 0;
}
*/
import "C"

import (
    "bufio"
    "bytes"
    "fmt"
    "net"
    "os"
    "strings"
    "sync"
    "time"
    "unsafe"
)

// cache for sysctl-based discovery (macOS).  Protected by mutex; tests
// manipulate it directly to validate caching behaviour.
var sysctlCache struct {
    mu sync.Mutex
    m  map[PortKey][]PortEntry
    ts time.Time
}


// allZero reports whether every byte in the slice is zero.
func allZero(b []byte) bool {
    for _, c := range b {
        if c != 0 {
            return false
        }
    }
    return true
}

// hostAndFamily converts the raw IPv4/IPv6 fields from the C
// structure into a human-readable host string and family label.  It is
// extracted for ease of testing and keeps the conversion logic in one place.
func hostAndFamily(ip4 uint32, ip6 [16]byte) (string, string) {
    if !allZero(ip6[:]) {
        return net.IP(ip6[:]).String(), "IPv6"
    }
    if ip4 != 0 {
        ip := make(net.IP, 4)
        ip[0] = byte(ip4)
        ip[1] = byte(ip4 >> 8)
        ip[2] = byte(ip4 >> 16)
        ip[3] = byte(ip4 >> 24)
        return ip.String(), "IPv4"
    }
    return "", ""
}

// procEntry mirrors the C struct used by get_proc_sockets.  It is used to
// communicate the results of scanning all processes for open sockets.
type procEntry struct {
    Pid   int32
    Proto int
    Port  int
    IP4   uint32
    IP6   [16]byte
}

// collectProcSockets invokes the C helper to enumerate sockets owned by each
// process.  The returned slice must not be retained across future calls; the
// C memory is freed immediately.
func collectProcSockets() ([]procEntry, error) {
    var centries *C.struct_procentry
    var cnt C.int
    if C.get_proc_sockets(&centries, &cnt) != 0 {
        return nil, fmt.Errorf("get_proc_sockets failed")
    }
    defer C.free(unsafe.Pointer(centries))

    slice := (*[1 << 20]C.struct_procentry)(unsafe.Pointer(centries))[:cnt:cnt]
    out := make([]procEntry, 0, cnt)
    for _, e := range slice {
        var ip6 [16]byte
        for i := 0; i < 16; i++ {
            ip6[i] = byte(e.ip6[i])
        }
        out = append(out, procEntry{
            Pid:   int32(e.pid),
            Proto: int(e.proto),
            Port:  int(e.port),
            IP4:   uint32(e.ip4),
            IP6:   ip6,
        })
    }
    return out, nil
}

// protoName converts a numeric protocol value into the string used by
// PortKey.  Values are taken from the kernel (IPPROTO_TCP, IPPROTO_UDP, etc.).
func protoName(p int) string {
    switch p {
    case 6:
        return "tcp"
    case 17:
        return "udp"
    default:
        return fmt.Sprintf("ip%d", p)
    }
}

// procName returns the short name of a process using libproc's proc_name.
// cmdline attempts to use proc_pidpath to obtain the full executable path.
func procName(pid int32) string {
    var buf [256]C.char
    if C.proc_name(C.int(pid), unsafe.Pointer(&buf[0]), C.uint32_t(len(buf))) <= 0 {
        return ""
    }
    return C.GoString(&buf[0])
}

func procPath(pid int32) string {
    var buf [1024]C.char
    if C.proc_pidpath(C.int(pid), unsafe.Pointer(&buf[0]), C.uint32_t(len(buf))) <= 0 {
        return ""
    }
    return C.GoString(&buf[0])
}

// mergeProcMetadata walks the sockets owned by each process and merges
// PID/name/command-line information into the provided result map.  Only
// entries that already exist in the map (typically discovered via sysctl)
// are updated; everything else is ignored.
func mergeProcMetadata(result map[PortKey][]PortEntry) {
    entries, err := collectProcSockets()
    if err != nil || len(entries) == 0 {
        return
    }
    for _, pe := range entries {
        key := PortKey{Protocol: protoName(pe.Proto), Port: pe.Port}
        if ents, ok := result[key]; ok {
            for i := range ents {
                if ents[i].Pid == 0 {
                    ents[i].Pid = pe.Pid
                    ents[i].Name = procName(pe.Pid)
                    ents[i].Cmdline = procPath(pe.Pid)
                    // bundle lookup may still use ps/mdls but that's fine
                    ents[i].AppBundle = bundleIDFunc(pe.Pid)
                }
            }
        }
    }
}

// appsByNetstat parses the output of `netstat -an` for TCP and UDP
// listeners.  This method is lightweight but does not know about process
// ownership.
func appsByNetstat() (map[PortKey][]PortEntry, error) {
    out, err := runCmd("netstat", "-an")
    if err != nil {
        return nil, err
    }
    return parseNetstat(out), nil
}

// appsBySysctl obtains listener information via native sysctl calls; it may
// return entries with empty Name/Pid.
func appsBySysctl() (map[PortKey][]PortEntry, error) {
    // permit callers to simulate a failure (useful during debugging or in
    // constrained environments).  When GOPORTS_FAKE_SYSCTL is set the native
    // path will return an error and AppsByPort will fall back to lsof/netstat.
    if os.Getenv("GOPORTS_FAKE_SYSCTL") == "1" {
        return nil, fmt.Errorf("forced sysctl failure")
    }

    // caching
    sysctlCache.mu.Lock()
    if sysctlCache.m != nil && timeNow().Sub(sysctlCache.ts) < cacheDuration {
        res := make(map[PortKey][]PortEntry, len(sysctlCache.m))
        for k, v := range sysctlCache.m {
            res[k] = append([]PortEntry(nil), v...)
        }
        sysctlCache.mu.Unlock()
        return res, nil
    }
    sysctlCache.mu.Unlock()

    var entries *C.struct_goentry
    var cnt C.int
    if C.sysctl_pcblist(&entries, &cnt) != 0 {
        return nil, fmt.Errorf("sysctl_pcblist failed")
    }
    defer C.free(unsafe.Pointer(entries))

    result := make(map[PortKey][]PortEntry)
    slice := (*[1 << 20]C.struct_goentry)(unsafe.Pointer(entries))[:cnt:cnt]
    for _, e := range slice {
        key := PortKey{Protocol: "", Port: int(e.port)}
        if e.proto == 6 {
            key.Protocol = "tcp"
        } else if e.proto == 17 {
            key.Protocol = "udp"
        } else {
            key.Protocol = fmt.Sprintf("ip%d", e.proto)
        }
        host, family := hostAndFamily(uint32(e.ip4), *(*[16]byte)(unsafe.Pointer(&e.ip6)))
        entry := PortEntry{Host: host, Protocol: key.Protocol, Family: family}
        result[key] = append(result[key], entry)
    }

    // always attempt to enrich with PID/name/cmdline using native APIs.  We
    // do this unconditionally because it requires no external binary.
    mergeProcMetadata(result)

    // if nativeOnly is requested we don't fall back to lsof, otherwise merge
    // any additional metadata it provides (might include duplicate entries).
    if !nativeOnly {
        if out, err := runCmd("lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-iUDP"); err == nil {
            mergeLsofMeta(result, out)
        }
    }

    // update cache
    sysctlCache.mu.Lock()
    sysctlCache.m = make(map[PortKey][]PortEntry, len(result))
    for k, v := range result {
        sysctlCache.m[k] = append([]PortEntry(nil), v...)
    }
    sysctlCache.ts = timeNow()
    sysctlCache.mu.Unlock()
    return result, nil
}

// mergeLsofMeta takes the output from an `lsof` invocation and applies the
// PID/Name/Command line information to any matching entries in the native
// map.  This helper is separately testable because callers can provide
// synthetic lsof output.
func mergeLsofMeta(result map[PortKey][]PortEntry, out []byte) {
    meta := parseLsof(out)
    for k, metas := range meta {
        if ents, ok := result[k]; ok {
            for i := range ents {
                if len(metas) > 0 {
                    ents[i].Pid = metas[0].Pid
                    ents[i].Name = metas[0].Name
                    ents[i].Cmdline = metas[0].Cmdline
                    if metas[0].AppBundle != "" {
                        ents[i].AppBundle = metas[0].AppBundle
                    }
                }
            }
        }
    }
}

// parseNetstat consumes the raw data produced by `netstat -an` and returns a
// ports map.  Lines not representing listening sockets are ignored.
func parseNetstat(data []byte) map[PortKey][]PortEntry {
    m := make(map[PortKey][]PortEntry)
    scanner := bufio.NewScanner(bytes.NewReader(data))
    for scanner.Scan() {
        line := scanner.Text()
        fields := strings.Fields(line)
        if len(fields) < 5 {
            continue
        }
        proto := fields[0]
        if !strings.HasPrefix(proto, "tcp") && !strings.HasPrefix(proto, "udp") {
            continue
        }
        if strings.HasPrefix(proto, "tcp") {
            if len(fields) < 6 {
                continue
            }
            state := fields[len(fields)-1]
            if state != "LISTEN" {
                continue
            }
        }
        local := fields[3]
        // address may be like "*.80" or "127.0.0.1.22" or "[::1].80"
        host, port := splitNetstatAddr(local)
        if port == 0 {
            continue
        }
        key := PortKey{Protocol: strings.TrimPrefix(proto, "4"), Port: port}
        family := ""
        if strings.HasSuffix(proto, "4") {
            family = "IPv4"
        } else if strings.HasSuffix(proto, "6") {
            family = "IPv6"
        }
        entry := PortEntry{Host: host, Protocol: key.Protocol, Family: family}
        m[key] = append(m[key], entry)
    }
    return m
}

// splitNetstatAddr separates a netstat address of the form host.port and
// returns the host string and numeric port.  IPv6 addresses are bracketed or
// expressed with hex, but netstat uses dots, so we simply split at the last
// dot.
func splitNetstatAddr(addr string) (string, int) {
    i := strings.LastIndex(addr, ".")
    if i == -1 {
        return "", 0
    }
    h := addr[:i]
    pstr := addr[i+1:]
    var p int
    fmt.Sscanf(pstr, "%d", &p)
    return h, p
}

// discoverPorts for darwin falls back to lsof when the native netstat-based
// enumeration does not succeed (we only use netstat for lighter weight
// reads).  Having this function ensures the generic AppsByPort can call it.
func discoverPorts() ([]byte, error) {
    return runCmd("lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-iUDP")
}
