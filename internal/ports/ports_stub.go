//go:build !darwin && !linux
// +build !darwin,!linux

package ports

import "errors"

// default stub used on unsupported platforms.
func discoverPorts() ([]byte, error) {
    return nil, errors.New("port enumeration not implemented on this platform")
}
