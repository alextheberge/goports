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
    // just forward everything to the CLI runner; the --gui flag is ignored
    // because no GUI exists on this platform.
    cli.Run(os.Args[1:])
}
