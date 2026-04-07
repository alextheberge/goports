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
	webviewChild := false
	childURL := ""

	// GUI-specific overrides parsed from the command line
	var width, height int
	var x, y int
	var debug bool
	var title string

	var cliArgs []string
	// simple one-pass parser that both removes the --gui switch and
	// handles a few gui-specific options.  Non-webview flags are left in
	// cliArgs so that GUI invocation can later delegate to the CLI if needed.
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		switch arg {
		case "--gui":
			guiMode = true
		case "--webview-child":
			webviewChild = true
			if i+1 < len(os.Args) {
				childURL = os.Args[i+1]
				i++
			}
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
		case "--webview-x":
			if i+1 < len(os.Args) {
				fmt.Sscanf(os.Args[i+1], "%d", &x)
				i++
			}
		case "--webview-y":
			if i+1 < len(os.Args) {
				fmt.Sscanf(os.Args[i+1], "%d", &y)
				i++
			}
		case "--webview-title":
			if i+1 < len(os.Args) {
				title = os.Args[i+1]
				i++
			}
		case "--webview-debug":
			debug = true
		default:
			cliArgs = append(cliArgs, arg)
		}
	}

	if webviewChild {
		// child process just opens a webview and exits
		if childURL == "" {
			childURL = "http://localhost"
		}
		// apply any CLI overrides, config will supply the others
		if width > 0 || height > 0 {
			gui.SetWebviewSize(width, height)
		}
		if x != 0 || y != 0 {
			gui.SetWebviewPosition(x, y)
		}
		if title != "" {
			gui.SetWebviewTitle(title)
		}
		if debug {
			gui.SetWebviewDebug(true)
		}
		gui.OpenWebview(childURL)
		return
	}

	if guiMode {
		// apply any webview-related overrides before starting the menu app
		if width > 0 || height > 0 {
			gui.SetWebviewSize(width, height)
		}
		if x != 0 || y != 0 {
			gui.SetWebviewPosition(x, y)
		}
		if title != "" {
			gui.SetWebviewTitle(title)
		}
		if debug {
			gui.SetWebviewDebug(true)
		}
		gui.Run()
	} else {
		os.Exit(cli.Run(cliArgs))
	}
}
