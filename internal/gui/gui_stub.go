//go:build !darwin
// +build !darwin

// Package gui implements a simple tray/menu GUI on Linux and Windows.  It
// provides roughly the same behaviour as the macOS version, although without
// embedded webviews – clicking "View Activity Graph" simply opens the URL in
// the default browser.  The package still compiles on all platforms; the
// darwin-specific files contain the full macOS implementation.
package gui

import (
    "fmt"
    "net"
    "net/http"
    "os"
    "os/exec"
    "sort"
    "strconv"
    "strings"
    "time"

    "github.com/getlantern/systray"
    "github.com/gen2brain/beeep"

    "github.com/user/goports/internal/config"
    "github.com/user/goports/internal/ports"
)

// graphMenuItem abstracts the minimal subset of systray.MenuItem that we
// interact with when opening the activity graph.  The indirection allows
// unit tests to substitute a fake without pulling in the full systray
// machinery.
type graphMenuItem interface {
    Disable()
    SetTitle(string)
}

// execCommand is a variable so tests can stub out command execution.
var execCommand = exec.Command

func handleGraphClick(item graphMenuItem, url string) {
    logf("graph menu clicked, eventURL=%s\n", url)
    if url == "" {
        url = "http://localhost"
    }
    // open in default browser using platform-appropriate command
    var cmd *exec.Cmd
    switch os := strings.ToLower(os.Getenv("OS")); os {
    case "windows_nt": // weird value on Windows
        cmd = execCommand("rundll32", "url.dll,FileProtocolHandler", url)
    default:
        // assume Unix-like
        cmd = execCommand("xdg-open", url)
    }
    if err := cmd.Start(); err != nil {
        logf("failed to launch browser: %v\n", err)
    }
}

// logf writes to stderr and also appends the same message to a
// log file under the config directory.  This ensures diagnostics are
// available even when the app is started from a desktop launcher.
func logf(format string, a ...interface{}) {
    msg := fmt.Sprintf(format, a...)
    fmt.Fprint(os.Stderr, msg)
    if path, err := config.Path(); err == nil {
        logPath := path + ".log"
        f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
        if err == nil {
            f.WriteString(msg)
            f.Close()
        }
    }
}

// stub functions used by mac-only code; on other systems they do trivial work
func isDarkMode() bool { return false }
func setTrayIcon() { systray.SetIcon(iconData) }
func promptFilter(old string) string { return old }
func promptNumber(prompt string, old int) int { return old }
func iconForBundle(bundle string) []byte { return nil }

// portMenuGroup holds the menu items associated with a particular port.
type portMenuGroup struct {
    parent   *systray.MenuItem
    pidItem  *systray.MenuItem
    cmdItem  *systray.MenuItem
    killItem *systray.MenuItem
    openItem *systray.MenuItem
    visible  bool
}

func Run() {
    systray.Run(onReady, onExit)
}

var (
    eventSrv     *http.Server
    eventCleanup func()
    eventAddr    string
    eventURL     string
)

// remaining setters exist to satisfy API but are irrelevant here
func SetWebviewSize(w, h int)      {}
func SetWebviewDebug(d bool)       {}
func SetWebviewTitle(t string)     {}
func SetWebviewPosition(x, y int)  {}

func onReady() {
    systray.SetTitle("")
    systray.SetTooltip("Ports")

    aboutItem := systray.AddMenuItem("About goports", "Open project page")
    var graphItem *systray.MenuItem
    if eventAddr != "" {
        graphItem = systray.AddMenuItem("View Activity Graph", "Open live activity web UI")
    }
    systray.AddSeparator()
    quitItem := systray.AddMenuItem("Quit", "Quit goports")

    go func() {
        for range aboutItem.ClickedCh {
            exec.Command("xdg-open", "https://github.com/alextheberge/goports").Run()
        }
    }()
    go func() {
        for range quitItem.ClickedCh {
            systray.Quit()
        }
    }()
    if graphItem != nil {
        go func() {
            for range graphItem.ClickedCh {
                handleGraphClick(graphItem, eventURL)
            }
        }()
    }

    // use existing logic for events server and ticker loop
    if eventAddr != "" {
        // nothing special
    }
    go tickerLoop()
}

func onExit() {
    if eventSrv != nil {
        _ = eventSrv.Close()
    }
    if eventCleanup != nil {
        eventCleanup()
    }
}

func tickerLoop() {
    // identical to darwin implementation, simplified imports
    var groups map[ports.PortKey]*portMenuGroup
    groups = make(map[ports.PortKey]*portMenuGroup)
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()
    for {
        ports.Scan() // keep cache warm
        list := ports.GetPorts()
        sort.Slice(list, func(i, j int) bool {
            if list[i].Key.Protocol != list[j].Key.Protocol {
                return list[i].Key.Protocol < list[j].Key.Protocol
            }
            return list[i].Key.Port < list[j].Key.Port
        })
        cfg := config.Load()
        for _, p := range list {
            if !matchesFilter(p.Key, p.Entries, cfg.SearchFilter) {
                if g, ok := groups[p.Key]; ok && g.visible {
                    g.parent.Hide()
                    g.visible = false
                }
                continue
            }
            g, ok := groups[p.Key]
            if !ok {
                g = &portMenuGroup{}
                g.parent = systray.AddMenuItem(portTitle(p.Key, p.Entries), "")
                g.pidItem = g.parent.AddSubMenuItem("", "")
                g.cmdItem = g.parent.AddSubMenuItem("", "")
                g.killItem = g.parent.AddSubMenuItem("kill", "kill process")
                g.openItem = g.parent.AddSubMenuItem("", "open URL")
                go func(item *systray.MenuItem, key ports.PortKey) {
                    for range item.ClickedCh {
                        // ignore for now
                    }
                }(g.killItem, p.Key)
                groups[p.Key] = g
            }
            g.parent.SetTitle(portTitle(p.Key, p.Entries))
            if !g.visible {
                g.parent.Show()
                g.visible = true
            }
        }
        <-ticker.C
    }
}
