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
		StartOnLogin:         true,
		Notifications:        true,
		RefreshInterval:      5,
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
		WebviewWidth: 1024, WebviewHeight: 768, WebviewDebug: true, WebviewTitle: "custom",
		WebviewX: 123, WebviewY: 456}
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
	if loaded.WebviewTitle != "custom" {
		t.Errorf("webview title not preserved: %q", loaded.WebviewTitle)
	}
	if loaded.WebviewX != 123 || loaded.WebviewY != 456 {
		t.Errorf("webview position not preserved: %d,%d", loaded.WebviewX, loaded.WebviewY)
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
	if loaded.WebviewX != 0 || loaded.WebviewY != 0 {
		t.Errorf("expected default webview position 0,0, got %d,%d", loaded.WebviewX, loaded.WebviewY)
	}
}

func TestDefaultSettings(t *testing.T) {
	d := DefaultSettings()
	if !d.Notifications || d.RefreshInterval != 5 || !d.ShowTCP || !d.ShowUDP {
		t.Fatalf("unexpected defaults: %+v", d)
	}
	if d.WebviewWidth != 800 || d.WebviewHeight != 600 {
		t.Fatalf("webview defaults: %+v", d)
	}
}

func TestResetToDefaults(t *testing.T) {
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

	if err := Save(Settings{NativeOnly: true, ShowTCP: false, RefreshInterval: 15}); err != nil {
		t.Fatal(err)
	}
	if err := ResetToDefaults(); err != nil {
		t.Fatal(err)
	}
	loaded := Load()
	if loaded.NativeOnly {
		t.Fatal("expected NativeOnly false after reset")
	}
	if !loaded.ShowTCP || !loaded.ShowUDP {
		t.Fatal("expected TCP/UDP on after reset")
	}
	if loaded.RefreshInterval != 5 {
		t.Fatalf("expected interval 5, got %d", loaded.RefreshInterval)
	}
}
