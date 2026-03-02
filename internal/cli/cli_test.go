package cli

import (
    "reflect"
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
