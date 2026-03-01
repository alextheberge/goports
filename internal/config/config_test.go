package config

import (
    "encoding/json"
    "os"
    "testing"
)

// ensure that loading and saving preserves the blocked notifications map
func TestBlockedNotifications(t *testing.T) {
    tmp, err := os.CreateTemp("", "settings*.json")
    if err != nil {
        t.Fatal(err)
    }
    path := tmp.Name()
    tmp.Close()
    defer os.Remove(path)

    // create settings with one blocked entry
    s := Settings{
        StartOnLogin: true,
        Notifications: true,
        RefreshInterval: 5,
        BlockedNotifications: map[string]bool{"tcp/80": true},
    }
    // write manually to path
    f, err := os.Create(path)
    if err != nil {
        t.Fatal(err)
    }
    enc := json.NewEncoder(f)
    if err := enc.Encode(&s); err != nil {
        t.Fatal(err)
    }
    f.Close()

    // override configPathFunc to return our temp file
    old := configPathFunc
    configPathFunc = func() (string, error) { return path, nil }
    defer func() { configPathFunc = old }()

    loaded := Load()
    if !loaded.BlockedNotifications["tcp/80"] {
        t.Errorf("expected blocked entry, got %+v", loaded)
    }
}
