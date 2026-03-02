package cli

import (
    "io"
    "os"
    "reflect"
    "strings"
    "testing"

    "github.com/user/goports/internal/ports"
)

func makeEntry(proto, fam string, port int, name, bundle string) ports.PortEntry {
    return ports.PortEntry{Protocol: proto, Family: fam, Cmdline: name, Name: name, AppBundle: bundle, Pid: 1}
}

func TestApplyFilters(t *testing.T) {
    key1 := ports.PortKey{Protocol: "tcp", Port: 80}
    key2 := ports.PortKey{Protocol: "udp", Port: 53}
    data := map[ports.PortKey][]ports.PortEntry{
        key1: {makeEntry("tcp", "IPv4", 80, "nginx", "com.example")},
        key2: {makeEntry("udp", "IPv6", 53, "dnsmasq", "")},
    }

    // no filters returns original
    if got := applyFilters(data, "", "", "", ""); !reflect.DeepEqual(got, data) {
        t.Errorf("expected unabbreviated data, got %v", got)
    }

    if got := applyFilters(data, "udp", "", "", ""); !reflect.DeepEqual(got, map[ports.PortKey][]ports.PortEntry{key2: data[key2]}) {
        t.Errorf("proto filter failed: %v", got)
    }

    if got := applyFilters(data, "", "nginx", "", ""); !reflect.DeepEqual(got, map[ports.PortKey][]ports.PortEntry{key1: data[key1]}) {
        t.Errorf("name filter failed: %v", got)
    }

    if got := applyFilters(data, "", "", "com.example", ""); !reflect.DeepEqual(got, map[ports.PortKey][]ports.PortEntry{key1: data[key1]}) {
        t.Errorf("bundle filter failed: %v", got)
    }

    if got := applyFilters(data, "", "", "", "IPv6"); !reflect.DeepEqual(got, map[ports.PortKey][]ports.PortEntry{key2: data[key2]}) {
        t.Errorf("family filter failed: %v", got)
    }
}

func TestNativeFlagSetsPortPackage(t *testing.T) {
    // start with false
    ports.SetNativeOnly(false)
    Run([]string{"--native"})
    if !ports.NativeOnlyEnabled() {
        t.Errorf("expected nativeOnly to be set by CLI flag")
    }
}

func TestExportAndTUIFlags(t *testing.T) {
    // just ensure parsing of flags doesn't panic; actual behavior is difficult
    // to verify in unit tests without capturing stdout or starting a TUI.
    Run([]string{"--export"})
}

func TestSpecFlag(t *testing.T) {
    // capture stdout
    old := os.Stdout
    r, w, _ := os.Pipe()
    os.Stdout = w
    Run([]string{"--spec"})
    w.Close()
    var buf strings.Builder
    io.Copy(&buf, r)
    os.Stdout = old

    out := buf.String()
    if !strings.Contains(out, "openapi") {
        t.Errorf("expected spec output to contain 'openapi', got %q", out)
    }
}

// passing webview-specific flags to the CLI should be harmless (they are
// consumed by the darwin main if and only if GUI mode is requested).
func TestWebviewFlagsIgnored(t *testing.T) {
    Run([]string{"--webview-width", "100", "--webview-height", "200", "--webview-debug"})
}
