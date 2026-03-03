//go:build darwin
// +build darwin

// Package gui implements the macOS menu bar interface.
package gui

/*
#cgo CFLAGS: -mmacosx-version-min=10.12 -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// goports_webview_position moves the given NSWindow to the specified (x,y)
// position and makes it frontmost.  The `x` value is an offset from the left
// edge of the screen.  The `y` value is treated as a distance from the top of
// the main display (measured in points) – this makes it easy to request a
// window that sits just beneath the menu bar.  A negative `y` means "use the
// default" (roughly 50 points from the top).  The function computes the
// appropriate origin taking the window's height into account.
void goports_webview_position(void *winPtr, int x, int y) {
    NSWindow *w = (NSWindow*)winPtr;
    if (w == nil) return;
    NSScreen *s = [NSScreen mainScreen];
    NSRect screenFrame = [s frame];
    NSRect winFrame = [w frame];
    CGFloat newX = x < 0 ? 100 : x;
    CGFloat newY;
    if (y >= 0) {
        newY = screenFrame.size.height - winFrame.size.height - y;
    } else {
        newY = screenFrame.size.height - winFrame.size.height - 50;
    }
    NSRect dest = NSMakeRect(newX, newY, winFrame.size.width, winFrame.size.height);
    // animate the move so the window slides into place rather than popping up
    // abruptly; Cocoa will also implicitly show the window as part of the
    // animation if it's not already visible.
    [[NSApplication sharedApplication] activateIgnoringOtherApps:YES];
    [w setFrame:dest display:YES animate:YES];
    [w makeKeyAndOrderFront:nil];
}

// goports_webview_get_frame returns the current frame of the window in the
// coordinate system used by our configuration (x from left, y from top).  All
// pointers are optional; missing values are left unchanged.
void goports_webview_get_frame(void *winPtr, int *x, int *y, int *w, int *h) {
    NSWindow *wptr = (NSWindow*)winPtr;
    if (wptr == nil) return;
    NSRect f = [wptr frame];
    NSScreen *s = [NSScreen mainScreen];
    NSRect sf = [s frame];
    if (x) *x = (int)f.origin.x;
    if (w) *w = (int)f.size.width;
    if (h) *h = (int)f.size.height;
    if (y) *y = (int)(sf.size.height - f.origin.y - f.size.height);
}
*/
import "C"

import (
    "fmt"
    "net"
    "net/http"
    "os"
    "os/exec"
    "path/filepath"
    "runtime"
    "sort"
    "strconv"
    "strings"
    "syscall"
    "time"
    "unsafe"

    "github.com/getlantern/systray"
    "github.com/gen2brain/beeep"
    "github.com/webview/webview"

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

// handleGraphClick encapsulates the behaviour that occurs when the user
// selects the "View Activity Graph" menu item.  It is factored out so
// unit tests can exercise the failure path without needing to spin up the
// entire systray environment.
// execCommand is a variable so tests can stub out command execution.
var execCommand = exec.Command

func handleGraphClick(item graphMenuItem, url string) {
    logf("graph menu clicked, eventURL=%s\n", url)
    if url == "" {
        url = "http://localhost"
    }
    // spawn a helper process that creates the webview on its own main thread.
    cmd := execCommand(os.Args[0], "--webview-child", url)
    // forward any output from the child to our stderr so logs are visible
    cmd.Stdout = os.Stderr
    cmd.Stderr = os.Stderr
    if err := cmd.Start(); err != nil {
        // fall back to external browser and disable menu item
        if alertErr := beeep.Alert("goports", "Unable to spawn embedded webview; opening default browser instead.", ""); alertErr != nil {
            logf("beeep.Alert error: %v\n", alertErr)
        }
        logf("starting child webview process failed: %v\n", err)
        item.Disable()
        item.SetTitle("Activity Graph (unavailable)")
        if err := execCommand("open", url).Run(); err != nil {
            logf("open command failed: %v\n", err)
        }
        return
    }
    // asynchronously wait for the helper to exit and log any error.
    go func() {
        if err := cmd.Wait(); err != nil {
            logf("webview child exited with error: %v\n", err)
        } else {
            logf("webview child exited normally\n")
        }
    }()
}

// webviewNew is a package-level variable pointing at webview.New.  tests can
// replace it to simulate failure.
var webviewNew = webview.New

// logf writes to stderr and also appends the same message to a
// log file under the config directory.  This ensures diagnostics are
// available even when the app is started from the Dock or a launchd job.
func logf(format string, a ...interface{}) {
    msg := fmt.Sprintf(format, a...)
    fmt.Fprint(os.Stderr, msg)
    // attempt to append to log file but ignore errors
    if path, err := config.Path(); err == nil {
        logPath := path + ".log"
        f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
        if err == nil {
            f.WriteString(msg)
            f.Close()
        }
    }
}

// getFrame returns the current window frame values.  It is a variable so
// unit tests may substitute a fake implementation without linking the C
// helper.
var getFrame = func(ptr unsafe.Pointer) (x, y, w, h int) {
    var cx, cy, cw, ch C.int
    C.goports_webview_get_frame(ptr, &cx, &cy, &cw, &ch)
    return int(cx), int(cy), int(cw), int(ch)
}

// positionWindow is the function used to move/animate the window.  tests
// can replace it with a no-op to avoid dereferencing fake pointers.
var positionWindow = func(ptr unsafe.Pointer, x, y int) {
    C.goports_webview_position(ptr, C.int(x), C.int(y))
}

// OpenWebview is like handleGraphClick but intended for a standalone process
// (child mode).  It only attempts to create the webview once and returns any
// error to the caller so the process can exit appropriately.
func OpenWebview(url string) error {
    logf("webview child starting with URL=%s\n", url)
    if url == "" {
        url = "http://localhost"
    }
    // as with the menu process we must lock the OS thread before creating or
    // running the webview.  Failing to do so will generally result in no
    // window appearing, which is exactly the silent failure we were
    // diagnosing earlier.
    runtime.LockOSThread()
    defer runtime.UnlockOSThread()

    w := webviewNew(webviewDebug)
    if w == nil {
        logf("webviewNew returned nil in child\n")
        return fmt.Errorf("webview.New returned nil")
    }
    defer func() {
        if r := recover(); r != nil {
            logf("panic inside webview child: %v\n", r)
        }
    }()
    w.SetTitle(webviewTitle)
    w.SetSize(webviewWidth, webviewHeight, webview.HintNone)
    w.Navigate(url)
    // position the window if user/config provided coordinates; otherwise
    // leave defaults.  The CGO helper also makes the window key/front so it
    // cannot sit behind other applications.
    if ptr := w.Window(); ptr != nil {
        if webviewX >= 0 && webviewY >= 0 {
            positionWindow(ptr, webviewX, webviewY)
        } else {
            // still bring it to front even if we don't move it explicitly
            positionWindow(ptr, webviewX, webviewY)
        }
    }
    logf("webview.Run about to execute\n")
    w.Run()
    // after the window closes record its final geometry so future openings
    // can restore the size/position automatically.  we do this in the child
    // because the parent has no easy way to inspect the webview pointer.
    if ptr := w.Window(); ptr != nil {
        if x, y, ww, hh := getFrame(ptr); ww > 0 && hh > 0 {
            cfg := config.Load()
            cfg.WebviewWidth = ww
            cfg.WebviewHeight = hh
            cfg.WebviewX = x
            cfg.WebviewY = y
            _ = config.Save(cfg)
            logf("saved webview geometry %dx%d at %d,%d\n", ww, hh, x, y)
        }
    }
    logf("webview.Run returned, exiting child\n")
    return nil
}

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

// promptNumber asks the user to enter a numeric value using the AppleScript
// dialog.  If the response cannot be parsed or the dialog is cancelled, the
// supplied default is returned unchanged.
func promptNumber(prompt string, old int) int {
    resp := promptFilter(fmt.Sprintf("%s (current %d)", prompt, old))
    if resp == "" {
        return old
    }
    if n, err := strconv.Atoi(resp); err == nil && n > 0 {
        return n
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
        logf("iconForBundle: mdfind failed for %s: %v\n", bundle, err)
        iconCache[bundle] = nil
        return nil
    }
    lines := strings.Split(strings.TrimSpace(string(out)), "\n")
    if len(lines) == 0 || lines[0] == "" {
        logf("iconForBundle: no path found for %s\n", bundle)
        iconCache[bundle] = nil
        return nil
    }
    appPath := lines[0]
    icnsPath := findIcns(appPath)
    if icnsPath == "" {
        logf("iconForBundle: no icns under %s\n", appPath)
        iconCache[bundle] = nil
        return nil
    }
    // sips sometimes refuses to write to stdout (exit code 13), especially
    // when the source file is protected.  Work around by writing to a temp
    // file and reading that back.
    tmp, err := os.CreateTemp("", "goports-icon-*.png")
    if err != nil {
        logf("iconForBundle: temp file create failed: %v\n", err)
        iconCache[bundle] = nil
        return nil
    }
    tmpPath := tmp.Name()
    tmp.Close()
    defer os.Remove(tmpPath)

    cmd := exec.Command("sips", "-s", "format", "png", icnsPath, "--out", tmpPath)
    if err := cmd.Run(); err != nil {
        logf("iconForBundle: sips failed for %s: %v\n", icnsPath, err)
        iconCache[bundle] = nil
        return nil
    }
    png, err := os.ReadFile(tmpPath)
    if err != nil {
        logf("iconForBundle: read temp icon failed: %v\n", err)
        iconCache[bundle] = nil
        return nil
    }
    logf("iconForBundle: loaded icon for %s (%d bytes)\n", bundle, len(png))
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

// portTitle builds the menu item title for a port key and its associated
// entries.  It includes the protocol/port, host, executable name, bundle
// identifier, and — crucially — any process IDs and a short command-line
// snippet.  Including the PID/CMD directly in the title makes the information
// visible without opening the submenu, matching the behaviour of the legacy
// macOS app and addressing the user's request to "bring back" that useful
// metadata.
func portTitle(key ports.PortKey, entries []ports.PortEntry) string {
    // collect PIDs and a command snippet
    var pidStrs []string
    for _, e := range entries {
        if e.Pid != 0 {
            pidStrs = append(pidStrs, fmt.Sprintf("%d", e.Pid))
        }
    }
    cmdSnippet := ""
    if len(entries) > 0 && entries[0].Cmdline != "" {
        cmdSnippet = entries[0].Cmdline
        if len(cmdSnippet) > 40 {
            cmdSnippet = cmdSnippet[:40] + "…"
        }
    }

    title := fmt.Sprintf("%s %d", strings.ToUpper(key.Protocol), key.Port)
    if entries[0].Host != "" {
        title += fmt.Sprintf(" (%s)", entries[0].Host)
    }
    if len(pidStrs) > 0 {
        title += fmt.Sprintf(" [%s]", strings.Join(pidStrs, ","))
    }
    if cmdSnippet != "" {
        title += fmt.Sprintf(" - %s", cmdSnippet)
    } else if entries[0].Name != "" {
        title += fmt.Sprintf(" - %s", entries[0].Name)
    }
    if entries[0].AppBundle != "" {
        title += fmt.Sprintf(" (%s)", entries[0].AppBundle)
    }
    return title
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

var (
    eventSrv     *http.Server
    eventCleanup func()
    eventAddr    string
    eventURL     string // normalized address for browser
    // these variables control the embedded webview window; they are set by
    // onReady using the user configuration and may be overridden by the CLI
    // layer via the exported setters below.
    webviewWidth  = 800
    webviewHeight = 600
    webviewDebug  = false
    webviewTitle  = "goports Activity"
    // optional initial position for the window (points from left, bottom).
    // zero values are valid but negative values mean "leave whatever the
    // system chooses".  defaults give a location roughly under the menu bar.
    webviewX = 100
    webviewY = 50
)

// SetWebviewSize allows the caller (typically main_darwin) to override the
// default dimensions for the embedded webview window.  A value of zero leaves
// the existing setting unchanged.
func SetWebviewSize(w, h int) {
    if w > 0 {
        webviewWidth = w
    }
    if h > 0 {
        webviewHeight = h
    }
}

// SetWebviewDebug enables or disables debug mode for the webview.New call.
func SetWebviewDebug(d bool) {
    webviewDebug = d
}

// SetWebviewTitle allows the caller to override the embedded window title.
// An empty string leaves the current title untouched.
func SetWebviewTitle(t string) {
    if t != "" {
        webviewTitle = t
    }
}

// SetWebviewPosition lets callers specify an initial origin for the webview
// window.  The `x` value is an offset from the left edge of the screen.  The
// `y` value is a distance from the top of the screen; this makes it easy to
// open the window just below the menu bar.  Negative coordinates are ignored
// (the runtime will choose a sensible default).
func SetWebviewPosition(x, y int) {
    if x >= 0 {
        webviewX = x
    }
    if y >= 0 {
        webviewY = y
    }
}

// normalizeAddr converts a listener address (possibly "[::]:port" or
// "0.0.0.0:port") into a URL string suitable for opening in a browser.
// Unspecified hosts become "localhost".
func normalizeAddr(addr string) string {
    host, port, err := net.SplitHostPort(addr)
    if err != nil {
        return "http://" + addr
    }
    if host == "" || host == "::" || host == "[::]" || host == "0.0.0.0" {
        host = "localhost"
    }
    // bracket IPv6 addresses if not already
    if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
        host = "[" + host + "]"
    }
    return "http://" + host + ":" + port
}

func onReady() {
    // announce where diagnostic log lives (macOS uses Library/Application Support).
    if p, err := config.Path(); err == nil {
        logf("diagnostic log file: %s.log\n", p)
    }

    // load configuration and apply any stored webview preferences
    if cfg := config.Load(); true {
        if cfg.WebviewWidth > 0 {
            webviewWidth = cfg.WebviewWidth
        }
        if cfg.WebviewHeight > 0 {
            webviewHeight = cfg.WebviewHeight
        }
        webviewDebug = cfg.WebviewDebug
        if cfg.WebviewTitle != "" {
            webviewTitle = cfg.WebviewTitle
        }
    }
    // configure tray icon and tooltip.  the icon may change based on dark
    // mode; setTrayIcon handles the decision and will be re-run periodically.
    setTrayIcon()

    // start the embedded HTTP server on a random port so the menu can open
    // the WebUI without requiring the user to manually specify --http-port.
    if srv, cleanup, err := ports.StartEventServer(":0"); err == nil {
        eventSrv = srv
        eventCleanup = cleanup
        eventAddr = srv.Addr
        // compute a browser-friendly URL
        eventURL = normalizeAddr(eventAddr)
        logf("event server listening on %s (url %s)\n", eventAddr, eventURL)
    }

    // begin ingesting port activity events; the graphing implementation is
    // currently a stub but subscribing here ensures the channel is active.
    startGraphing()
    systray.SetTitle("") // no title, just an icon
    systray.SetTooltip("Ports")

    // static items at the bottom of the menu
    aboutItem := systray.AddMenuItem("About goports", "Open project page")
    // add a general activity graph entry if server started
    var graphItem *systray.MenuItem
    if eventAddr != "" {
        graphItem = systray.AddMenuItem("View Activity Graph", "Open live activity web UI")
    }
    systray.AddSeparator()
    quitItem := systray.AddMenuItem("Quit", "Quit goports")

    // handle graph menu clicks
    if graphItem != nil {
        go func() {
            defer func() {
                if r := recover(); r != nil {
                    logf("graphItem handler panic: %v\n", r)
                }
            }()
            for range graphItem.ClickedCh {
                // webview requires running on a locked OS thread (often the main
                // thread).  Lock here before invoking so w.Run won't crash the
                // process when it attempts to pump the native event loop.
                runtime.LockOSThread()
                handleGraphClick(graphItem, eventURL)
                runtime.UnlockOSThread()
            }
        }()
    }

    // settings submenu
    settingsItem := systray.AddMenuItem("Settings", "Preferences")
    startItem := settingsItem.AddSubMenuItemCheckbox("Start at Login", "Launch goports when you log in", false)
    notifItem := settingsItem.AddSubMenuItemCheckbox("Enable Notifications", "Notify when ports open/close", false)
    tcpItem := settingsItem.AddSubMenuItemCheckbox("Show TCP", "Display TCP listening ports", true)
    udpItem := settingsItem.AddSubMenuItemCheckbox("Show UDP", "Display UDP listeners", true)
    nativeItem := settingsItem.AddSubMenuItemCheckbox("Use native discovery only", "Do not invoke lsof or other helpers", false)
    filterItem := settingsItem.AddSubMenuItem("Filter...", "Show only ports matching text")
    refreshItem := settingsItem.AddSubMenuItem("Refresh interval", "Cycle between 5/10/15s")
    widthItem := settingsItem.AddSubMenuItem("Webview width...", "Pixels for embedded window width")
    heightItem := settingsItem.AddSubMenuItem("Webview height...", "Pixels for embedded window height")
    xItem := settingsItem.AddSubMenuItem("Webview X pos...", "Horizontal position of window")
    yItem := settingsItem.AddSubMenuItem("Webview Y pos...", "Vertical position of window")
    titleItem := settingsItem.AddSubMenuItem("Webview title...", "Custom title for embedded window")
    resetItem := settingsItem.AddSubMenuItem("Reset webview settings", "Restore defaults for embedded window")

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
    widthItem.SetTitle(fmt.Sprintf("Webview width (%d)", cfg.WebviewWidth))
    heightItem.SetTitle(fmt.Sprintf("Webview height (%d)", cfg.WebviewHeight))
    xItem.SetTitle(fmt.Sprintf("Webview X (%d)", cfg.WebviewX))
    yItem.SetTitle(fmt.Sprintf("Webview Y (%d)", cfg.WebviewY))
    titleItem.SetTitle(fmt.Sprintf("Webview title (%s)", cfg.WebviewTitle))

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
    go func() {
        for range widthItem.ClickedCh {
            neww := promptNumber("Webview width", cfg.WebviewWidth)
            if neww != cfg.WebviewWidth {
                cfg.WebviewWidth = neww
                widthItem.SetTitle(fmt.Sprintf("Webview width (%d)", neww))
                _ = config.Save(cfg)
            }
        }
    }()
    go func() {
        for range heightItem.ClickedCh {
            newh := promptNumber("Webview height", cfg.WebviewHeight)
            if newh != cfg.WebviewHeight {
                cfg.WebviewHeight = newh
                heightItem.SetTitle(fmt.Sprintf("Webview height (%d)", newh))
                _ = config.Save(cfg)
            }
        }
    }()
    go func() {
        for range xItem.ClickedCh {
            newx := promptNumber("Webview X position", cfg.WebviewX)
            if newx != cfg.WebviewX {
                cfg.WebviewX = newx
                xItem.SetTitle(fmt.Sprintf("Webview X (%d)", newx))
                _ = config.Save(cfg)
            }
        }
    }()
    go func() {
        for range yItem.ClickedCh {
            newy := promptNumber("Webview Y position", cfg.WebviewY)
            if newy != cfg.WebviewY {
                cfg.WebviewY = newy
                yItem.SetTitle(fmt.Sprintf("Webview Y (%d)", newy))
                _ = config.Save(cfg)
            }
        }
    }()
    go func() {
        for range titleItem.ClickedCh {
            newt := promptFilter(cfg.WebviewTitle)
            if newt != cfg.WebviewTitle {
                cfg.WebviewTitle = newt
                titleItem.SetTitle(fmt.Sprintf("Webview title (%s)", newt))
                _ = config.Save(cfg)
            }
        }
    }()
    go func() {
        for range resetItem.ClickedCh {
            cfg.WebviewWidth = 800
            cfg.WebviewHeight = 600
            cfg.WebviewTitle = "goports Activity"
            cfg.WebviewDebug = false
            cfg.WebviewX = 0
            cfg.WebviewY = 0
            widthItem.SetTitle(fmt.Sprintf("Webview width (%d)", cfg.WebviewWidth))
            heightItem.SetTitle(fmt.Sprintf("Webview height (%d)", cfg.WebviewHeight))
            xItem.SetTitle(fmt.Sprintf("Webview X (%d)", cfg.WebviewX))
            yItem.SetTitle(fmt.Sprintf("Webview Y (%d)", cfg.WebviewY))
            titleItem.SetTitle(fmt.Sprintf("Webview title (%s)", cfg.WebviewTitle))
            _ = config.Save(cfg)
        }
    }()

    // start the ticker loop; it will mutate the menu and therefore must be
    // launched from the same goroutine as onReady. We intentionally start it
    // as a goroutine so that onReady may return immediately.
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
            title := portTitle(key, entries)
            // we still need pid and command strings for the submenu when
            // creating or updating an existing group
            var pidStrs []string
            var cmdStrs []string
            for _, e := range entries {
                pidStrs = append(pidStrs, fmt.Sprintf("%d", e.Pid))
                cmdStrs = append(cmdStrs, e.Cmdline)
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
