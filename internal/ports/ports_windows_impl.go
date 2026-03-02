//go:build windows
// +build windows

package ports

import (
    "fmt"
    "net"
    "unsafe"

    "golang.org/x/sys/windows"
)

// minimal definitions used with GetExtendedTcpTable/GetExtendedUdpTable
const (
    AF_INET  = 2
    AF_INET6 = 23

    TCP_TABLE_OWNER_PID_ALL = 5
    UDP_TABLE_OWNER_PID     = 1

    ERROR_INSUFFICIENT_BUFFER = 0x7A
)

// util functions to convert network-order port
func ntohs(port uint32) uint16 {
    return uint16(port>>8) | uint16(port<<8)
}

// structures matching Windows API (IPv4 only for now)
type mibTcpRowOwnerPid struct {
    state      uint32
    localAddr  uint32
    localPort  uint32
    remoteAddr uint32
    remotePort uint32
    owningPid  uint32
}

type mibUdpRowOwnerPid struct {
    localAddr uint32
    localPort uint32
    owningPid uint32
}

// low-level procs for iphlpapi.dll
var (
    modiphlpapi              = windows.NewLazySystemDLL("iphlpapi.dll")
    procGetExtendedTcpTable = modiphlpapi.NewProc("GetExtendedTcpTable")
    procGetExtendedUdpTable = modiphlpapi.NewProc("GetExtendedUdpTable")
)

// appsByWin returns a ports map using native Win32 APIs.  It handles
// IPv4 TCP and UDP listeners; IPv6 could be added later.
func appsByWin() (map[PortKey][]PortEntry, error) {
    result := make(map[PortKey][]PortEntry)

    // helper to invoke GetExtended*Table using the proc variables
    var buf []byte
    var size uint32
    // TCP: first query size
    ret, _, _ := procGetExtendedTcpTable.Call(
        0,
        uintptr(unsafe.Pointer(&size)),
        0,
        AF_INET,
        TCP_TABLE_OWNER_PID_ALL,
        0,
    )
    if ret != uintptr(ERROR_INSUFFICIENT_BUFFER) {
        return nil, fmt.Errorf("GetExtendedTcpTable size query: %v", ret)
    }
    buf = make([]byte, size)
    ret, _, _ = procGetExtendedTcpTable.Call(
        uintptr(unsafe.Pointer(&buf[0])),
        uintptr(unsafe.Pointer(&size)),
        0,
        AF_INET,
        TCP_TABLE_OWNER_PID_ALL,
        0,
    )
    if ret != 0 {
        return nil, fmt.Errorf("GetExtendedTcpTable call failed: %v", ret)
    }
    // first DWORD is count
    count := *(*uint32)(unsafe.Pointer(&buf[0]))
    rowSize := uint32(unsafe.Sizeof(mibTcpRowOwnerPid{}))
    for i := uint32(0); i < count; i++ {
        start := 4 + i*rowSize
        row := *(*mibTcpRowOwnerPid)(unsafe.Pointer(&buf[start]))
        port := int(ntohs(row.localPort))
        addr := row.localAddr
        key := PortKey{Protocol: "tcp", Port: port}
        host := net.IPv4(byte(addr), byte(addr>>8), byte(addr>>16), byte(addr>>24)).String()
        entry := PortEntry{Host: host, Protocol: "tcp", Pid: int32(row.owningPid)}
        if entry.Pid != 0 {
            entry.Name = winProcName(uint32(entry.Pid))
            entry.Cmdline = entry.Name
        }
        result[key] = append(result[key], entry)
    }

    // UDP using proc call as well
    size = 0
    ret, _, _ = procGetExtendedUdpTable.Call(
        0,
        uintptr(unsafe.Pointer(&size)),
        0,
        AF_INET,
        UDP_TABLE_OWNER_PID,
        0,
    )
    if ret != uintptr(ERROR_INSUFFICIENT_BUFFER) {
        return result, nil
    }
    buf = make([]byte, size)
    ret, _, _ = procGetExtendedUdpTable.Call(
        uintptr(unsafe.Pointer(&buf[0])),
        uintptr(unsafe.Pointer(&size)),
        0,
        AF_INET,
        UDP_TABLE_OWNER_PID,
        0,
    )
    if ret != 0 {
        return result, nil
    }
    count = *(*uint32)(unsafe.Pointer(&buf[0]))
    rowSize = uint32(unsafe.Sizeof(mibUdpRowOwnerPid{}))
    for i := uint32(0); i < count; i++ {
        start := 4 + i*rowSize
        row := *(*mibUdpRowOwnerPid)(unsafe.Pointer(&buf[start]))
        port := int(ntohs(row.localPort))
        addr := row.localAddr
        key := PortKey{Protocol: "udp", Port: port}
        host := net.IPv4(byte(addr), byte(addr>>8), byte(addr>>16), byte(addr>>24)).String()
        entry := PortEntry{Host: host, Protocol: "udp", Pid: int32(row.owningPid)}
        if entry.Pid != 0 {
            entry.Name = winProcName(uint32(entry.Pid))
            entry.Cmdline = entry.Name
        }
        result[key] = append(result[key], entry)
    }

    return result, nil
}

// winProcName returns the base name of the executable for the given pid.
func winProcName(pid uint32) string {
    h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
    if err != nil {
        return ""
    }
    defer windows.CloseHandle(h)
    var buf [windows.MAX_PATH]uint16
    var size uint32 = windows.MAX_PATH
    if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
        return ""
    }
    path := windows.UTF16ToString(buf[:size])
    // drop directory
    if idx := len(path) - 1; idx >= 0 {
        for i := len(path)-1; i >= 0; i-- {
            if path[i] == '\\' || path[i] == '/' {
                return path[i+1:]
            }
        }
    }
    return path
}
