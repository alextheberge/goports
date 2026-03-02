//go:build !linux
// +build !linux

package ports

import "errors"

// appsByProc is only implemented on linux; this stub satisfies references on
// other platforms.
func appsByProc() (map[PortKey][]PortEntry, error) {
    return nil, errors.New("native proc-based enumeration not available")
}
