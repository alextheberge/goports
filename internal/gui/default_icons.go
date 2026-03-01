package gui

import (
    _ "embed"
    "strings"
)

//go:embed icons/ruby.png
var rubyPng []byte
//go:embed icons/prometheus.png
var prometheusPng []byte
//go:embed icons/netcat.png
var netcatPng []byte
//go:embed icons/nxrunner.png
var nxrunnerPng []byte
//go:embed icons/onedrive.png
var onedrivePng []byte
//go:embed icons/rapportd.png
var rapportdPng []byte

// defaultIcons maps simple process names to bundled images.  These are used
// when bundle-based lookup fails or the process is not a Cocoa app.
var defaultIcons = map[string][]byte{
    "ruby":       rubyPng,
    "prometheus": prometheusPng,
    "netcat":     netcatPng,
    "nxrunner":   nxrunnerPng,
    "onedrive":   onedrivePng,
    "rapportd":   rapportdPng,
}

// iconForName returns a default icon for known processes using the base name
// of the executable.
func iconForName(name string) []byte {
    if name == "" {
        return nil
    }
    // strip any path
    if idx := strings.LastIndex(name, "/"); idx != -1 {
        name = name[idx+1:]
    }
    if b, ok := defaultIcons[name]; ok {
        return b
    }
    return nil
}
