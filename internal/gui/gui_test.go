package gui

import (
    "strings"
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

func TestPortTitle(t *testing.T) {
    key := ports.PortKey{Protocol: "tcp", Port: 8080}
    entries := []ports.PortEntry{
        {Pid: 1234, Cmdline: "/usr/bin/foo --bar", Name: "foo", AppBundle: "com.example.foo", Host: "127.0.0.1"},
    }
    title := portTitle(key, entries)
    if !strings.Contains(title, "TCP 8080") {
        t.Errorf("title %q missing protocol/port", title)
    }
    if !strings.Contains(title, "[1234]") {
        t.Errorf("title %q missing pid", title)
    }
    if strings.Contains(title, "foo") == false {
        t.Errorf("title %q missing name", title)
    }
    if !strings.Contains(title, "com.example.foo") {
        t.Errorf("title %q missing bundle", title)
    }
    // ensure long cmdline is truncated
    entries[0].Cmdline = strings.Repeat("x", 100)
    longTitle := portTitle(key, entries)
    if strings.Contains(longTitle, strings.Repeat("x", 50)) {
        t.Errorf("title %q should have truncated command", longTitle)
    }
}
