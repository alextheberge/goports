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
    // configure tray icon and tooltip
    systray.SetIcon(iconData)
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
    portMenu := make(map[int]*portMenuGroup)
    firstRun := true

    cfg := config.Load()
    interval := cfg.RefreshInterval
    if interval <= 0 {
        interval = 5
    }
    ticker := time.NewTicker(time.Duration(interval) * time.Second)
    defer ticker.Stop()

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
        newPorts := ports.AppsByPort()

        var portList []int
        for p := range newPorts {
            portList = append(portList, p)
        }
        sort.Ints(portList)

        // process current ports
        for _, p := range portList {
            entries := newPorts[p]
            if len(entries) == 0 {
                fmt.Printf("warning: no entries for port %d, skipping\n", p)
                continue
            }

            // aggregate metadata across all entries
            var pidStrs []string
            var cmdStrs []string
            for _, e := range entries {
                pidStrs = append(pidStrs, fmt.Sprintf("%d", e.Pid))
                cmdStrs = append(cmdStrs, e.Cmdline)
            }
            title := fmt.Sprintf("%d", p)
            if entries[0].Host != "" {
                title += fmt.Sprintf(" (%s)", entries[0].Host)
            }
            title += fmt.Sprintf(" - %s", entries[0].Name)
            if entries[0].AppBundle != "" {
                title += fmt.Sprintf(" (%s)", entries[0].AppBundle)
            }

            group, exists := portMenu[p]
            if !exists {
                parent := systray.AddMenuItem(title, "")
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
                pidItem := parent.AddSubMenuItem("PIDs: "+strings.Join(pidStrs, ", "), "")
                cmdItem := parent.AddSubMenuItem("Cmd: "+strings.Join(cmdStrs, " | "), "")
                killItem := parent.AddSubMenuItem("Kill All", "Terminate all processes on this port")
                openItem := parent.AddSubMenuItem(fmt.Sprintf("Open http://localhost:%d", p), "")

                group = &portMenuGroup{
                    parent:   parent,
                    pidItem:  pidItem,
                    cmdItem:  cmdItem,
                    killItem: killItem,
                    openItem: openItem,
                    visible:  true,
                }
                portMenu[p] = group

                if !firstRun && cfg.Notifications {
                    beeep.Notify("Open Port Discovered", fmt.Sprintf("Port %d was just opened by %s", p, entries[0].Name), "")
                }

                // kill handler; runs once per logical port
                go func(port int, kill *systray.MenuItem) {
                    for range kill.ClickedCh {
                        cur := ports.AppsByPort()
                        if ents, ok := cur[port]; ok {
                            for _, e := range ents {
                                syscall.Kill(int(e.Pid), syscall.SIGKILL)
                            }
                            if cfg.Notifications {
                    beeep.Notify("Killed Process", fmt.Sprintf("Terminated processes on port %d", port), "")
                }
                        }
                    }
                }(p, killItem)

                // open handler
                go func(port int, open *systray.MenuItem) {
                    for range open.ClickedCh {
                        exec.Command("open", fmt.Sprintf("http://localhost:%d", port)).Run()
                    }
                }(p, openItem)
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
                    if !firstRun && cfg.Notifications {
                        beeep.Notify("Open Port Discovered", fmt.Sprintf("Port %d was just opened by %s", p, entries[0].Name), "")
                    }
                }
            }
        }

        // hide ports that have closed
        for p, group := range portMenu {
            if _, still := newPorts[p]; !still && group.visible {
                group.parent.Hide()
                group.pidItem.Hide()
                group.cmdItem.Hide()
                group.killItem.Hide()
                group.openItem.Hide()
                group.visible = false
                if cfg.Notifications {
                    beeep.Notify("Closed Port Discovered", fmt.Sprintf("Port %d was just closed", p), "")
                }
            }
        }

        firstRun = false
        <-ticker.C
    }
}
