package gui

import (
    "testing"

    "github.com/user/goports/internal/ports"
)

func makeEntry(name string) ports.PortEntry {
    return ports.PortEntry{Name: name, AppBundle: "", Host: "", Pid: 1, Family: "IPv4", Protocol: "tcp"}
}

func TestMatchesFilter(t *testing.T) {
    key := ports.PortKey{Protocol: "tcp", Port: 80}
    entries := []ports.PortEntry{makeEntry("nginx")}

    if !matchesFilter(key, entries, "") {
        t.Error("empty filter should match")
    }
    if !matchesFilter(key, entries, "nginx") {
        t.Error("should match by name")
    }
    if matchesFilter(key, entries, "foo") {
        t.Error("wrongly matched unrelated text")
    }
    if !matchesFilter(key, entries, "tcp") {
        t.Error("should match protocol")
    }
    if !matchesFilter(key, entries, "80") {
        t.Error("should match port number string")
    }
}

func TestIsDarkMode(t *testing.T) {
    // just ensure it runs without crashing; value depends on system appearance
    _ = isDarkMode()
}
