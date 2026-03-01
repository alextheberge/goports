//go:build linux
// +build linux

package ports

import "errors"

// discoverPorts for Linux is currently a stub.  A future implementation may
// read /proc/net/{tcp,udp} and translate the information into the same lsof-
// style text that parseLsof expects.  For now callers receive an error so that
// the rest of the program can decide how to behave (e.g. report "not
// supported" or fall back to some other mechanism).
func discoverPorts() ([]byte, error) {
    return nil, errors.New("port enumeration not implemented on linux")
}
