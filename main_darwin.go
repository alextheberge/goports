//go:build darwin
// +build darwin

package main

import (
    "fmt"
    "os"

    "github.com/user/goports/internal/cli"
    "github.com/user/goports/internal/gui"
)

// darwin-specific main allows the menu-bar GUI.  The logic mirrors the original
// main.go: it scans for a top-level --gui flag and delegates accordingly.
func main() {
    guiMode := false

    // GUI-specific overrides parsed from the command line
    var width, height int
    var debug bool

    var cliArgs []string
    // simple one-pass parser that both removes the --gui switch and
    // handles a few gui-specific options.  Non-webview flags are left in
    // cliArgs so that GUI invocation can later delegate to the CLI if needed.
    for i := 1; i < len(os.Args); i++ {
        arg := os.Args[i]
        switch arg {
        case "--gui":
            guiMode = true
        case "--webview-width":
            if i+1 < len(os.Args) {
                // ignore parse error, defaults suffice
                fmt.Sscanf(os.Args[i+1], "%d", &width)
                i++
            }
        case "--webview-height":
            if i+1 < len(os.Args) {
                fmt.Sscanf(os.Args[i+1], "%d", &height)
                i++
            }
        case "--webview-debug":
            debug = true
        default:
            cliArgs = append(cliArgs, arg)
        }
    }

    if guiMode {
        // apply any webview-related overrides before starting the menu app
        if width > 0 || height > 0 {
            gui.SetWebviewSize(width, height)
        }
        if debug {
            gui.SetWebviewDebug(true)
        }
        gui.Run()
    } else {
        cli.Run(cliArgs)
    }
}
