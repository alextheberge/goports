//go:build darwin
// +build darwin

package main

import (
    "os"

    "github.com/user/goports/internal/cli"
    "github.com/user/goports/internal/gui"
)

// darwin-specific main allows the menu-bar GUI.  The logic mirrors the original
// main.go: it scans for a top-level --gui flag and delegates accordingly.
func main() {
    guiMode := false

    var cliArgs []string
    for i := 1; i < len(os.Args); i++ {
        arg := os.Args[i]
        if arg == "--gui" {
            guiMode = true
            continue
        }
        cliArgs = append(cliArgs, arg)
    }

    if guiMode {
        gui.Run()
    } else {
        cli.Run(cliArgs)
    }
}
