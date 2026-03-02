//go:build !darwin
// +build !darwin

package ports

import "errors"

// Non-darwin builds need dummy versions of the darwin-specific helpers so that
// AppsByPort can compile unconditionally.
func appsBySysctl() (map[PortKey][]PortEntry, error) {
    return nil, errors.New("sysctl enumeration not available")
}

func appsByNetstat() (map[PortKey][]PortEntry, error) {
    return nil, errors.New("netstat enumeration not available")
}
