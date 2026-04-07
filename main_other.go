//go:build !darwin
// +build !darwin

package main

import (
	"os"

	"github.com/user/goports/internal/cli"
)

// non-darwin builds only provide the CLI; GUI dependencies are excluded so
// cross-compiles do not fail due to systray.
func main() {
	// if user requested GUI mode on non-darwin we warn and fall back.
	for _, a := range os.Args[1:] {
		if a == "--gui" {
			os.Stderr.WriteString("warning: GUI not available on this platform, running CLI instead\n")
			break
		}
	}
	os.Exit(cli.Run(os.Args[1:]))
}
