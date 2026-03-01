package main

import (
    "os"

    "github.com/user/goports/internal/cli"
    "github.com/user/goports/internal/gui"
)

// main parses only the top‑level --gui flag using a dedicated FlagSet.  Any
// unknown arguments (for future CLI flags such as --watch, --kill, --open)
// are left untouched and passed along to the CLI runner, which is expected to
// perform its own parsing.  This prevents early failures when unknown flags
// are provided.
func main() {
    guiMode := false

    // Manually scan arguments to isolate --gui without disturbing others.
    // Any appearance of "--gui" (boolean flag) switches guiMode and is
    // removed from the slice passed to the CLI runner.  All other arguments,
    // including unknown flags meant for the CLI, are forwarded verbatim.
    var cliArgs []string
    for i := 1; i < len(os.Args); i++ {
        arg := os.Args[i]
        if arg == "--gui" {
            guiMode = true
            continue
        }
        // If we later add a short form like -g we could handle it here as
        // well, but for now only --gui exists at top level.
        cliArgs = append(cliArgs, arg)
    }

    if guiMode {
        gui.Run()
    } else {
        cli.Run(cliArgs)
    }
}
