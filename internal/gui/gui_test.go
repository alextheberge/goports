package gui

import (
    "strings"
    "testing"
    "os/exec"
    "unsafe"

    "github.com/webview/webview"
    "github.com/user/goports/internal/ports"
)

func makeEntry(name string) ports.PortEntry {
    return ports.PortEntry{Name: name, AppBundle: "", Host: "", Pid: 1, Family: "IPv4", Protocol: "tcp"}
}

func TestMatchesFilter(t *testing.T) {
    key := ports.PortKey{Protocol: "tcp", Port: 80}
    entries := []ports.PortEntry{makeEntry("nginx")}

    if !matchesFilter(key, entries, "") {
        t.Error("empty filter should match")
    }
    if !matchesFilter(key, entries, "nginx") {
        t.Error("should match by name")
    }
    if matchesFilter(key, entries, "foo") {
        t.Error("wrongly matched unrelated text")
    }
    if !matchesFilter(key, entries, "tcp") {
        t.Error("should match protocol")
    }
    if !matchesFilter(key, entries, "80") {
        t.Error("should match port number string")
    }
}

func TestIsDarkMode(t *testing.T) {
    // just ensure it runs without crashing; value depends on system appearance
    _ = isDarkMode()
}

func TestPortTitle(t *testing.T) {
    key := ports.PortKey{Protocol: "tcp", Port: 8080}
    entries := []ports.PortEntry{
        {Pid: 1234, Cmdline: "/usr/bin/foo --bar", Name: "foo", AppBundle: "com.example.foo", Host: "127.0.0.1"},
    }
    title := portTitle(key, entries)
    if !strings.Contains(title, "TCP 8080") {
        t.Errorf("title %q missing protocol/port", title)
    }
    if !strings.Contains(title, "[1234]") {
        t.Errorf("title %q missing pid", title)
    }
    if strings.Contains(title, "foo") == false {
        t.Errorf("title %q missing name", title)
    }
    if !strings.Contains(title, "com.example.foo") {
        t.Errorf("title %q missing bundle", title)
    }
    // ensure long cmdline is truncated
    entries[0].Cmdline = strings.Repeat("x", 100)
    longTitle := portTitle(key, entries)
    if strings.Contains(longTitle, strings.Repeat("x", 50)) {
        t.Errorf("title %q should have truncated command", longTitle)
    }
}

func TestNormalizeAddr(t *testing.T) {
    cases := map[string]string{
        ":1234":        "http://localhost:1234",
        "[::]:9999":     "http://localhost:9999",
        "0.0.0.0:80":     "http://localhost:80",
        "127.0.0.1:8080": "http://127.0.0.1:8080",
        "[::1]:22":      "http://[::1]:22",
        "foo:123":        "http://foo:123",
    }
    for in, want := range cases {
        if got := normalizeAddr(in); got != want {
            t.Errorf("normalizeAddr(%q) = %q; want %q", in, got, want)
        }
    }
}

// Exercise the setters to ensure they adjust package variables as expected.
func TestWebviewSetters(t *testing.T) {
    // save originals
    ow, oh, od, ot := webviewWidth, webviewHeight, webviewDebug, webviewTitle
    defer func() {
        webviewWidth = ow
        webviewHeight = oh
        webviewDebug = od
        webviewTitle = ot
    }()

    SetWebviewSize(0, 0) // should be no-op
    if webviewWidth != ow || webviewHeight != oh {
        t.Errorf("expected no change, got %dx%d", webviewWidth, webviewHeight)
    }
    SetWebviewSize(640, 480)
    if webviewWidth != 640 || webviewHeight != 480 {
        t.Errorf("size not set properly: %dx%d", webviewWidth, webviewHeight)
    }
    SetWebviewDebug(true)
    if !webviewDebug {
        t.Errorf("debug flag not set")
    }
    SetWebviewTitle("foo")
    if webviewTitle != "foo" {
        t.Errorf("title not set, got %q", webviewTitle)
    }
}

func TestWebviewPositionSetters(t *testing.T) {
    ox, oy := webviewX, webviewY
    defer func() { webviewX, webviewY = ox, oy }()

    SetWebviewPosition(-1, -1) // should be ignored
    if webviewX != ox || webviewY != oy {
        t.Errorf("negative coords changed values: %d,%d", webviewX, webviewY)
    }
    SetWebviewPosition(10, 20)
    if webviewX != 10 || webviewY != 20 {
        t.Errorf("position not updated, got %d,%d", webviewX, webviewY)
    }
}

// fakeMenu is a simple implementer of graphMenuItem for testing.
type fakeMenu struct {
    disabled bool
    title    string
}

func (f *fakeMenu) Disable() { f.disabled = true }
func (f *fakeMenu) SetTitle(s string) { f.title = s }

func TestHandleGraphClickFailure(t *testing.T) {
    // simulate exec.Command always returning a stub that fails to start/run
    orig := execCommand
    execCommand = func(name string, args ...string) *exec.Cmd {
        // simply return a command with zero Path so Start/Run report an error.
        return &exec.Cmd{}
    }
    defer func() { execCommand = orig }()

    f := &fakeMenu{title: "initial"}
    handleGraphClick(f, "http://example")
    if !f.disabled {
        t.Error("expected menu to be disabled after failing to spawn child")
    }
    if f.title != "Activity Graph (unavailable)" {
        t.Errorf("unexpected title %q", f.title)
    }
}

// stubType implements the full webview.WebView interface with no-op
// methods.  Run() is the only one that does anything; we record whether it
// was invoked so tests can assert that OpenWebview actually executed the
// loop.
type stubType struct{
    ran bool
}

func (s *stubType) Run()                   { s.ran = true }
func (s *stubType) Terminate()             {}
func (s *stubType) Dispatch(f func())      {}
func (s *stubType) Destroy()               {}
func (s *stubType) Window() unsafe.Pointer { return nil }
func (s *stubType) SetTitle(string)        {}
func (s *stubType) SetSize(w, h int, hint webview.Hint) {}
func (s *stubType) Navigate(url string)    {}
func (s *stubType) SetHtml(html string)    {}
func (s *stubType) Init(js string)         {}
func (s *stubType) Eval(js string)         {}
func (s *stubType) Bind(name string, f interface{}) error { return nil }


func TestOpenWebview(t *testing.T) {
    // case1: webviewNew returns nil → error
    origNew := webviewNew
    webviewNew = func(debug bool) webview.WebView { return nil }
    if err := OpenWebview(""); err == nil {
        t.Error("expected error when webviewNew returns nil")
    }

    // case2: successful creation and Run invocation
    stub := &stubType{ran: false}
    webviewNew = func(debug bool) webview.WebView { return stub }
    if err := OpenWebview("http://foo"); err != nil {
        t.Errorf("unexpected error: %v", err)
    }
    if !stub.ran {
        t.Error("Run() was not called on the stubbed webview")
    }

    // restore original
    webviewNew = origNew
}
