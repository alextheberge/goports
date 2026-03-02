//go:build !darwin && !linux && !windows
// +build !darwin,!linux,!windows

package ports

import "errors"

// default stub used on unsupported platforms.
func discoverPorts() ([]byte, error) {
    return nil, errors.New("port enumeration not implemented on this platform")
}

// appsByProc is only implemented on linux.  Provide a stub on other OSes so
// the generic AppsByPort can be compiled unconditionally.
func appsByProc() (map[PortKey][]PortEntry, error) {
    return nil, errors.New("native proc-based enumeration not available")
}
