//go:build !windows
// +build !windows

package ports

import "errors"

// appsByWin is windows-specific. provide a stub so AppsByPort compiles on
// other platforms.
func appsByWin() (map[PortKey][]PortEntry, error) {
    return nil, errors.New("windows socket enumeration not available")
}
