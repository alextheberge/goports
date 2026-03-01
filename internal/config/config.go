package config

import (
    "encoding/json"
    "os"
    "path/filepath"
)

// Settings holds persistent user preferences for the application.
type Settings struct {
    StartOnLogin     bool `json:"start_on_login"`
    Notifications    bool `json:"notifications"`
    RefreshInterval  int  `json:"refresh_interval"` // seconds
}

const defaultInterval = 5

func configPath() (string, error) {
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

// Load returns the current settings, supplying defaults if the file is
// missing or unreadable.
func Load() Settings {
    path, err := configPath()
    if err != nil {
        return Settings{Notifications: true, RefreshInterval: defaultInterval}
    }
    f, err := os.Open(path)
    if err != nil {
        return Settings{Notifications: true, RefreshInterval: defaultInterval}
    }
    defer f.Close()
    var s Settings
    if err := json.NewDecoder(f).Decode(&s); err != nil {
        return Settings{Notifications: true, RefreshInterval: defaultInterval}
    }
    if s.RefreshInterval <= 0 {
        s.RefreshInterval = defaultInterval
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
