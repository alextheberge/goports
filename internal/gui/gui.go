// Package gui implements the macOS menu bar interface.
package gui

import (
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "sort"
    "strings"
    "syscall"
    "time"

    "github.com/getlantern/systray"
    "github.com/gen2brain/beeep"

    "github.com/user/goports/internal/config"
    "github.com/user/goports/internal/ports"
)

// portMenuGroup holds the menu items associated with a particular port.

var iconCache = make(map[string][]byte)

// isDarkMode queries macOS for the current appearance.  It returns true when
// the system appearance is set to Dark.  Failures are treated as light mode.
func isDarkMode() bool {
    out, err := exec.Command("defaults", "read", "-g", "AppleInterfaceStyle").Output()
    if err != nil {
        return false
    }
    return strings.TrimSpace(string(out)) == "Dark"
}

// setTrayIcon picks an appropriate icon for the current appearance and
// assigns it to the tray.  The icons are embedded via assets.go.
func setTrayIcon() {
    if isDarkMode() && len(iconDarkData) > 0 {
        systray.SetIcon(iconDarkData)
    } else {
        systray.SetIcon(iconData)
    }
}

// promptFilter presents a modal dialog to the user asking for a filter string.
// On cancel the existing value is returned unchanged.
func promptFilter(old string) string {
    script := `tell application "System Events" to display dialog "Filter ports:" default answer "` + old + `"`
    out, err := exec.Command("osascript", "-e", script).Output()
    if err != nil {
        return old
    }
    // out looks like "button returned:OK, text returned:foo"
    parts := strings.Split(string(out), ",")
    for _, p := range parts {
        p = strings.TrimSpace(p)
        if strings.HasPrefix(p, "text returned:") {
            return strings.TrimPrefix(p, "text returned:")
        }
    }
    return old
}

// matchesFilter returns true if any of the port entry fields match the
// filter substring (case insensitive).
func matchesFilter(key ports.PortKey, entries []ports.PortEntry, filter string) bool {
    if filter == "" {
        return true
    }
    f := strings.ToLower(filter)
    if strings.Contains(key.Protocol, f) || strings.Contains(fmt.Sprint(key.Port), f) {
        return true
    }
    for _, e := range entries {
        if strings.Contains(strings.ToLower(e.Name), f) ||
            strings.Contains(strings.ToLower(e.AppBundle), f) ||
            strings.Contains(strings.ToLower(e.Host), f) {
            return true
        }
    }
    return false
}

// iconForBundle attempts to locate the .app associated with a bundle identifier,
// find a .icns resource, convert it to PNG via sips, and return the raw bytes.
// Results are cached in memory; an empty slice indicates failure.
func iconForBundle(bundle string) []byte {
    if bundle == "" {
        return nil
    }
    if b, ok := iconCache[bundle]; ok {
        return b
    }
    // locate app path via mdfind
    out, err := exec.Command("mdfind", "kMDItemCFBundleIdentifier == '"+bundle+"'").Output()
    if err != nil {
        fmt.Fprintf(os.Stderr, "iconForBundle: mdfind failed for %s: %v\n", bundle, err)
        iconCache[bundle] = nil
        return nil
    }
    lines := strings.Split(strings.TrimSpace(string(out)), "\n")
    if len(lines) == 0 || lines[0] == "" {
        fmt.Fprintf(os.Stderr, "iconForBundle: no path found for %s\n", bundle)
        iconCache[bundle] = nil
        return nil
    }
    appPath := lines[0]
    icnsPath := findIcns(appPath)
    if icnsPath == "" {
        fmt.Fprintf(os.Stderr, "iconForBundle: no icns under %s\n", appPath)
        iconCache[bundle] = nil
        return nil
    }
    // sips sometimes refuses to write to stdout (exit code 13), especially
    // when the source file is protected.  Work around by writing to a temp
    // file and reading that back.
    tmp, err := os.CreateTemp("", "goports-icon-*.png")
    if err != nil {
        fmt.Fprintf(os.Stderr, "iconForBundle: temp file create failed: %v\n", err)
        iconCache[bundle] = nil
        return nil
    }
    tmpPath := tmp.Name()
    tmp.Close()
    defer os.Remove(tmpPath)

    cmd := exec.Command("sips", "-s", "format", "png", icnsPath, "--out", tmpPath)
    if err := cmd.Run(); err != nil {
        fmt.Fprintf(os.Stderr, "iconForBundle: sips failed for %s: %v\n", icnsPath, err)
        iconCache[bundle] = nil
        return nil
    }
    png, err := os.ReadFile(tmpPath)
    if err != nil {
        fmt.Fprintf(os.Stderr, "iconForBundle: read temp icon failed: %v\n", err)
        iconCache[bundle] = nil
        return nil
    }
    fmt.Fprintf(os.Stderr, "iconForBundle: loaded icon for %s (%d bytes)\n", bundle, len(png))
    iconCache[bundle] = png
    return png
}

// findIcns looks for the first .icns file under the app's Resources directory.
func findIcns(appPath string) string {
    var result string
    resources := filepath.Join(appPath, "Contents", "Resources")
    filepath.Walk(resources, func(path string, info os.FileInfo, err error) error {
        if err != nil || result != "" {
            return nil
        }
        if !info.IsDir() && strings.HasSuffix(path, ".icns") {
            result = path
            return filepath.SkipDir
        }
        return nil
    })
    return result
}
// by keeping a permanent structure we avoid recreating goroutines and menu
// entries on every reopen.
type portMenuGroup struct {
    parent   *systray.MenuItem
    pidItem  *systray.MenuItem
    cmdItem  *systray.MenuItem
    killItem *systray.MenuItem
    openItem *systray.MenuItem
    visible  bool
}

// Run starts the menu bar application. It blocks until the user quits.
func Run() {
    systray.Run(onReady, onExit)
}

func onReady() {
    // configure tray icon and tooltip.  the icon may change based on dark
    // mode; setTrayIcon handles the decision and will be re-run periodically.
    setTrayIcon()
    systray.SetTitle("") // no title, just an icon
    systray.SetTooltip("Ports")

    // static items at the bottom of the menu
    aboutItem := systray.AddMenuItem("About goports", "Open project page")
    systray.AddSeparator()
    quitItem := systray.AddMenuItem("Quit", "Quit goports")

    // settings submenu
    settingsItem := systray.AddMenuItem("Settings", "Preferences")
    startItem := settingsItem.AddSubMenuItemCheckbox("Start at Login", "Launch goports when you log in", false)
    notifItem := settingsItem.AddSubMenuItemCheckbox("Enable Notifications", "Notify when ports open/close", false)
    tcpItem := settingsItem.AddSubMenuItemCheckbox("Show TCP", "Display TCP listening ports", true)
    udpItem := settingsItem.AddSubMenuItemCheckbox("Show UDP", "Display UDP listeners", true)
    nativeItem := settingsItem.AddSubMenuItemCheckbox("Use native discovery only", "Do not invoke lsof or other helpers", false)
    filterItem := settingsItem.AddSubMenuItem("Filter...", "Show only ports matching text")
    refreshItem := settingsItem.AddSubMenuItem("Refresh interval", "Cycle between 5/10/15s")

    cfg := config.Load()
    if cfg.StartOnLogin {
        startItem.Check()
    } else {
        startItem.Uncheck()
    }
    if cfg.Notifications {
        notifItem.Check()
    } else {
        notifItem.Uncheck()
    }
    if cfg.ShowTCP {
        tcpItem.Check()
    } else {
        tcpItem.Uncheck()
    }
    if cfg.ShowUDP {
        udpItem.Check()
    } else {
        udpItem.Uncheck()
    }
    if cfg.NativeOnly {
        nativeItem.Check()
    } else {
        nativeItem.Uncheck()
    }
    if cfg.SearchFilter != "" {
        filterItem.SetTitle(fmt.Sprintf("Filter: %s", cfg.SearchFilter))
    }
    refreshItem.SetTitle(fmt.Sprintf("Refresh interval (%ds)", cfg.RefreshInterval))

    // click handlers for static items
    go func() {
        for range aboutItem.ClickedCh {
            exec.Command("open", "https://github.com/alextheberge/goports").Run()
        }
    }()
    go func() {
        for range quitItem.ClickedCh {
            systray.Quit()
        }
    }()

    // settings handlers
    go func() {
        for range startItem.ClickedCh {
            cfg.StartOnLogin = !cfg.StartOnLogin
            if cfg.StartOnLogin {
                setStartAtLogin(true)
                startItem.Check()
            } else {
                setStartAtLogin(false)
                startItem.Uncheck()
            }
            _ = config.Save(cfg)
        }
    }()
    go func() {
        for range nativeItem.ClickedCh {
            cfg.NativeOnly = !cfg.NativeOnly
            ports.SetNativeOnly(cfg.NativeOnly)
            if cfg.NativeOnly {
                nativeItem.Check()
            } else {
                nativeItem.Uncheck()
            }
            _ = config.Save(cfg)
        }
    }()
    go func() {
        for range filterItem.ClickedCh {
            newf := promptFilter(cfg.SearchFilter)
            cfg.SearchFilter = newf
            if newf == "" {
                filterItem.SetTitle("Filter...")
            } else {
                filterItem.SetTitle(fmt.Sprintf("Filter: %s", newf))
            }
            _ = config.Save(cfg)
        }
    }()
    go func() {
        for range tcpItem.ClickedCh {
            cfg.ShowTCP = !cfg.ShowTCP
            if cfg.ShowTCP {
                tcpItem.Check()
            } else {
                tcpItem.Uncheck()
            }
            _ = config.Save(cfg)
        }
    }()
    go func() {
        for range udpItem.ClickedCh {
            cfg.ShowUDP = !cfg.ShowUDP
            if cfg.ShowUDP {
                udpItem.Check()
            } else {
                udpItem.Uncheck()
            }
            _ = config.Save(cfg)
        }
    }()
    go func() {
        for range notifItem.ClickedCh {
            cfg.Notifications = !cfg.Notifications
            if cfg.Notifications {
                notifItem.Check()
            } else {
                notifItem.Uncheck()
            }
            _ = config.Save(cfg)
        }
    }()
    go func() {
        for range refreshItem.ClickedCh {
            switch cfg.RefreshInterval {
            case 5:
                cfg.RefreshInterval = 10
            case 10:
                cfg.RefreshInterval = 15
            default:
                cfg.RefreshInterval = 5
            }
            refreshItem.SetTitle(fmt.Sprintf("Refresh interval (%ds)", cfg.RefreshInterval))
            _ = config.Save(cfg)
        }
    }()

    // start the ticker loop; it will mutate the menu and therefore must be
    // launched from the same goroutine as onReady. We intentionally start it
    // as a goroutine so that onReady may return immediately.
    go tickerLoop()
}

func onExit() {
    // nothing special to clean up
}

// setStartAtLogin attempts to add or remove a login item using AppleScript.
// We don't return an error to the caller — failures are non‑fatal but logged
// to stderr.
func setStartAtLogin(enable bool) {
    exe, err := os.Executable()
    if err != nil {
        return
    }
    if enable {
        // add login item
        script := fmt.Sprintf(`tell application "System Events" to make login item at end with properties {name:"goports",path:"%s",hidden:false}`, exe)
        exec.Command("osascript", "-e", script).Run()
    } else {
        exec.Command("osascript", "-e", `tell application "System Events" to delete login item "goports"`).Run()
    }
}

func tickerLoop() {
    portMenu := make(map[ports.PortKey]*portMenuGroup)
    firstRun := true

    cfg := config.Load()
    interval := cfg.RefreshInterval
    if interval <= 0 {
        interval = 5
    }
    ticker := time.NewTicker(time.Duration(interval) * time.Second)
    defer ticker.Stop()

    // helper that updates icon based on appearance; we remember last value
    lastDark := isDarkMode()
    if lastDark {
        setTrayIcon()
    }

    for {
        // refresh config each loop in case user toggled settings
        cfg = config.Load()
        if cfg.RefreshInterval <= 0 {
            cfg.RefreshInterval = 5
        }
        if cfg.RefreshInterval != interval {
            interval = cfg.RefreshInterval
            ticker.Stop()
            ticker = time.NewTicker(time.Duration(interval) * time.Second)
        }
        // update icon if appearance changed
        if dark := isDarkMode(); dark != lastDark {
            setTrayIcon()
            lastDark = dark
        }

        newPorts := ports.AppsByPort()
        // respect protocol visibility settings
        if !cfg.ShowTCP || !cfg.ShowUDP {
            for k := range newPorts {
                if (k.Protocol == "tcp" && !cfg.ShowTCP) || (k.Protocol == "udp" && !cfg.ShowUDP) {
                    delete(newPorts, k)
                }
            }
        }

        var keys []ports.PortKey
        for k := range newPorts {
            keys = append(keys, k)
        }
        sort.Slice(keys, func(i, j int) bool {
            if keys[i].Protocol == keys[j].Protocol {
                return keys[i].Port < keys[j].Port
            }
            return keys[i].Protocol < keys[j].Protocol
        })

        // process current ports
        for _, key := range keys {
            entries := newPorts[key]
            if len(entries) == 0 {
                fmt.Printf("warning: no entries for %s, skipping\n", key)
                continue
            }

            // drop entries that don't match the user's filter
            if !matchesFilter(key, entries, cfg.SearchFilter) {
                if grp, ok := portMenu[key]; ok && grp.visible {
                    grp.parent.Hide()
                    grp.pidItem.Hide()
                    grp.cmdItem.Hide()
                    grp.killItem.Hide()
                    grp.openItem.Hide()
                    grp.visible = false
                }
                continue
            }

            // aggregate metadata across all entries
            var pidStrs []string
            var cmdStrs []string
            for _, e := range entries {
                pidStrs = append(pidStrs, fmt.Sprintf("%d", e.Pid))
                cmdStrs = append(cmdStrs, e.Cmdline)
            }
            title := fmt.Sprintf("%s %d", strings.ToUpper(key.Protocol), key.Port)
            if entries[0].Host != "" {
                title += fmt.Sprintf(" (%s)", entries[0].Host)
            }
            title += fmt.Sprintf(" - %s", entries[0].Name)
            if entries[0].AppBundle != "" {
                title += fmt.Sprintf(" (%s)", entries[0].AppBundle)
            }

            group, exists := portMenu[key]
            if !exists {
                var suffix string
                if entries[0].AppBundle != "" {
                    suffix = " (" + entries[0].AppBundle + ")"
                }
                desc := fmt.Sprintf("%s listening on %s%s", entries[0].Name, entries[0].Host, suffix)
                parent := systray.AddMenuItem(title, desc)
                // attempt to attach application icon if available
                var setIcon bool
                if entries[0].AppBundle != "" {
                    if icon := iconForBundle(entries[0].AppBundle); len(icon) > 0 {
                        parent.SetIcon(icon)
                        setIcon = true
                    }
                }
                // fallback based on executable name when bundle lookup fails
                if !setIcon {
                    if icon := iconForName(entries[0].Name); len(icon) > 0 {
                        parent.SetIcon(icon)
                    }
                }
                pidItem := parent.AddSubMenuItem("PIDs: "+strings.Join(pidStrs, ", "), "Process IDs listening on this port")
                cmdItem := parent.AddSubMenuItem("Cmd: "+strings.Join(cmdStrs, " | "), "Full command lines for the processes")
                notifToggle := parent.AddSubMenuItemCheckbox("Enable Notifications", "Toggle alerts for this port", true)
                if cfg.BlockedNotifications[key.String()] {
                    notifToggle.Uncheck()
                } else {
                    notifToggle.Check()
                }
                killItem := parent.AddSubMenuItem("Kill All", "Terminate all processes listening on this port")
                openItem := parent.AddSubMenuItem(fmt.Sprintf("Open http://localhost:%d", key.Port), "Open this port in the default browser")

                group = &portMenuGroup{
                    parent:   parent,
                    pidItem:  pidItem,
                    cmdItem:  cmdItem,
                    killItem: killItem,
                    openItem: openItem,
                    visible:  true,
                }

                // notification toggle handler
                go func(k ports.PortKey, toggle *systray.MenuItem) {
                    for range toggle.ClickedCh {
                        cfg := config.Load()
                        current := cfg.BlockedNotifications[k.String()]
                        // flip
                        cfg.BlockedNotifications[k.String()] = !current
                        if cfg.BlockedNotifications[k.String()] {
                            toggle.Uncheck()
                        } else {
                            toggle.Check()
                        }
                        _ = config.Save(cfg)
                    }
                }(key, notifToggle)
                portMenu[key] = group

                if !firstRun && cfg.Notifications && !cfg.BlockedNotifications[key.String()] {
                    beeep.Notify("Open Port Discovered", fmt.Sprintf("Port %d (%s) was just opened by %s", key.Port, strings.ToUpper(key.Protocol), entries[0].Name), "")
                }

                // kill handler; runs once per logical port
                go func(key ports.PortKey, kill *systray.MenuItem) {
                    for range kill.ClickedCh {
                        cur := ports.AppsByPort()
                        if ents, ok := cur[key]; ok {
                            for _, e := range ents {
                                syscall.Kill(int(e.Pid), syscall.SIGKILL)
                            }
                            if cfg.Notifications && !cfg.BlockedNotifications[key.String()] {
                    beeep.Notify("Killed Process", fmt.Sprintf("Terminated processes on %s", key), "")
                }
                        }
                    }
                }(key, killItem)

                // open handler
                go func(key ports.PortKey, open *systray.MenuItem) {
                    for range open.ClickedCh {
                        // only makes sense for tcp
                        if key.Protocol == "tcp" {
                            exec.Command("open", fmt.Sprintf("http://localhost:%d", key.Port)).Run()
                        }
                    }
                }(key, openItem)
            } else {
                // update existing group and make visible if it was hidden
                group.parent.SetTitle(title)
                group.pidItem.SetTitle("PIDs: "+strings.Join(pidStrs, ", "))
                group.cmdItem.SetTitle("Cmd: "+strings.Join(cmdStrs, " | "))
                if !group.visible {
                    group.parent.Show()
                    group.pidItem.Show()
                    group.cmdItem.Show()
                    group.killItem.Show()
                    group.openItem.Show()
                    group.visible = true
                    if !firstRun && cfg.Notifications && !cfg.BlockedNotifications[key.String()] {
                        beeep.Notify("Open Port Discovered", fmt.Sprintf("Port %d (%s) was just opened by %s", key.Port, strings.ToUpper(key.Protocol), entries[0].Name), "")
                    }
                }
            }
        }

        // hide ports that have closed
        for k, group := range portMenu {
            if _, still := newPorts[k]; !still && group.visible {
                group.parent.Hide()
                group.pidItem.Hide()
                group.cmdItem.Hide()
                group.killItem.Hide()
                group.openItem.Hide()
                group.visible = false
                if cfg.Notifications && !cfg.BlockedNotifications[k.String()] {
                    beeep.Notify("Closed Port Discovered", fmt.Sprintf("Port %d (%s) was just closed", k.Port, strings.ToUpper(k.Protocol)), "")
                }
            }
        }

        firstRun = false
        <-ticker.C
    }
}
