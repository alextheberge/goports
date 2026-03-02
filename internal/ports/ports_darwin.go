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
    "strings"
    "unsafe"
)


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

    // if nativeOnly is requested we don't attempt to merge metadata.
    if !nativeOnly {
        if out, err := runCmd("lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-iUDP"); err == nil {
            mergeLsofMeta(result, out)
        }
    }

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
