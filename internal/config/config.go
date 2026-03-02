package config

import (
    "encoding/json"
    "os"
    "path/filepath"
)

// Settings holds persistent user preferences for the application.
type Settings struct {
    StartOnLogin         bool              `json:"start_on_login"`
    Notifications        bool              `json:"notifications"`
    RefreshInterval      int               `json:"refresh_interval"` // seconds
    SearchFilter         string            `json:"search_filter,omitempty"` // last GUI filter text
    BlockedNotifications map[string]bool   `json:"blocked_notifications,omitempty"`
    ShowTCP              bool              `json:"show_tcp"`
    ShowUDP              bool              `json:"show_udp"`
    NativeOnly           bool              `json:"native_only"` // perform discovery without external tools
    // preferences for the embedded webview window used by the macOS GUI.
    WebviewWidth  int  `json:"webview_width,omitempty"`
    WebviewHeight int  `json:"webview_height,omitempty"`
    WebviewDebug  bool `json:"webview_debug,omitempty"`
}

const defaultInterval = 5

// configPathFunc is the function used internally to compute the settings
// file path.  It is a variable so tests may override it.
var configPathFunc = func() (string, error) {
    dir, err := os.UserConfigDir()
    if err != nil {
        return "", err
    }
    cfgDir := filepath.Join(dir, "goports")
    if err := os.MkdirAll(cfgDir, 0o755); err != nil {
        return "", err
    }
    return filepath.Join(cfgDir, "settings.json"), nil
}

func configPath() (string, error) {
    return configPathFunc()
}

// Load returns the current settings, supplying defaults if the file is
// missing or unreadable.
func Load() Settings {
    path, err := configPath()
    if err != nil {
        return Settings{Notifications: true, RefreshInterval: defaultInterval, ShowTCP: true, ShowUDP: true}
    }
    f, err := os.Open(path)
    if err != nil {
        return Settings{Notifications: true, RefreshInterval: defaultInterval, ShowTCP: true, ShowUDP: true}
    }
    defer f.Close()
    var s Settings
    if err := json.NewDecoder(f).Decode(&s); err != nil {
        return Settings{Notifications: true, RefreshInterval: defaultInterval, ShowTCP: true, ShowUDP: true}
    }
    if s.RefreshInterval <= 0 {
        s.RefreshInterval = defaultInterval
    }
    if s.BlockedNotifications == nil {
        s.BlockedNotifications = make(map[string]bool)
    }
    // default to showing both protocols unless explicitly disabled
    if !s.ShowTCP && !s.ShowUDP {
        s.ShowTCP = true
        s.ShowUDP = true
    }
    // sane defaults for webview
    if s.WebviewWidth <= 0 {
        s.WebviewWidth = 800
    }
    if s.WebviewHeight <= 0 {
        s.WebviewHeight = 600
    }
    return s
}

// Save writes the provided settings to disk.  It is a best-effort call; the
// caller may ignore errors.
func Save(s Settings) error {
    path, err := configPath()
    if err != nil {
        return err
    }
    tmp := path + ".tmp"
    f, err := os.Create(tmp)
    if err != nil {
        return err
    }
    enc := json.NewEncoder(f)
    enc.SetIndent("", "  ")
    if err := enc.Encode(&s); err != nil {
        f.Close()
        return err
    }
    f.Close()
    return os.Rename(tmp, path)
}
