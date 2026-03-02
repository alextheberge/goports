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

func TestProtocolSettingsDefault(t *testing.T) {
    // empty file -> both protocols visible by default
    tmp, err := os.CreateTemp("", "settings*.json")
    if err != nil {
        t.Fatal(err)
    }
    path := tmp.Name()
    tmp.Close()
    defer os.Remove(path)

    old := configPathFunc
    configPathFunc = func() (string, error) { return path, nil }
    defer func() { configPathFunc = old }()

    loaded := Load()
    if !loaded.ShowTCP || !loaded.ShowUDP {
        t.Errorf("expected both protocols to be visible by default, got %v", loaded)
    }
}

func TestProtocolSettingsSaveLoad(t *testing.T) {
    tmp, err := os.CreateTemp("", "settings*.json")
    if err != nil {
        t.Fatal(err)
    }
    path := tmp.Name()
    tmp.Close()
    defer os.Remove(path)

    old := configPathFunc
    configPathFunc = func() (string, error) { return path, nil }
    defer func() { configPathFunc = old }()

    s := Settings{ShowTCP: false, ShowUDP: true, NativeOnly: true,
        WebviewWidth: 1024, WebviewHeight: 768, WebviewDebug: true}
    if err := Save(s); err != nil {
        t.Fatalf("save failed: %v", err)
    }
    loaded := Load()
    if loaded.ShowTCP || !loaded.ShowUDP {
        t.Errorf("save/load mismatch: %v", loaded)
    }
    if !loaded.NativeOnly {
        t.Errorf("nativeOnly flag not preserved: %v", loaded)
    }
    if loaded.WebviewWidth != 1024 || loaded.WebviewHeight != 768 || !loaded.WebviewDebug {
        t.Errorf("webview settings not preserved: %+v", loaded)
    }
}

// ensure webview defaults are applied when missing from file
func TestWebviewDefaults(t *testing.T) {
    tmp, err := os.CreateTemp("", "settings*.json")
    if err != nil {
        t.Fatal(err)
    }
    path := tmp.Name()
    tmp.Close()
    defer os.Remove(path)

    old := configPathFunc
    configPathFunc = func() (string, error) { return path, nil }
    defer func() { configPathFunc = old }()

    // write an empty JSON object
    os.WriteFile(path, []byte("{}"), 0644)

    loaded := Load()
    if loaded.WebviewWidth != 800 || loaded.WebviewHeight != 600 {
        t.Errorf("expected default webview size 800x600, got %d x %d", loaded.WebviewWidth, loaded.WebviewHeight)
    }
}
