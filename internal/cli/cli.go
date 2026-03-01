// Package cli provides the command-line interface for the application.
package cli

import (
    "context"
    "flag"
    "fmt"
    "io"
    "os"
    "os/exec"
    "os/signal"
    "sort"
    "strings"
    "syscall"
    "time"

    "github.com/olekukonko/tablewriter"
    "github.com/gosuri/uilive"

    "github.com/user/goports/internal/ports"
)

// Run executes the CLI using the provided unparsed arguments.
func Run(args []string) {
    // define flags
    var watch bool
    var killPort int
    var openPort int

    fs := flag.NewFlagSet("goports", flag.ExitOnError)
    fs.BoolVar(&watch, "watch", false, "refresh every 5s live")
    fs.BoolVar(&watch, "w", false, "shorthand for --watch")
    fs.IntVar(&killPort, "kill", 0, "port whose PIDs to kill (0=unset)")
    fs.IntVar(&openPort, "open", 0, "port to open in browser (0=unset)")

    _ = fs.Parse(args)

    // kill action takes precedence
    if killPort > 0 {
        data := ports.AppsByPort()
        for port, entries := range data {
            if port != killPort {
                continue
            }
            for _, entry := range entries {
                if err := syscall.Kill(int(entry.Pid), syscall.SIGKILL); err == nil {
                    fmt.Printf("Killed PID %d on port %d\n", entry.Pid, port)
                } else {
                    fmt.Fprintf(os.Stderr, "failed to kill PID %d on port %d: %v\n", entry.Pid, port, err)
                }
            }
        }
        return
    }

    if openPort > 0 {
        url := fmt.Sprintf("http://localhost:%d", openPort)
        fmt.Printf("Opening %s\n", url)
        if err := exec.Command("open", url).Run(); err != nil {
            fmt.Fprintf(os.Stderr, "failed to open %s: %v\n", url, err)
            os.Exit(1)
        }
        return
    }

    render := func(w io.Writer, data map[int][]ports.PortEntry) {
        renderTable(w, data)
    }

    if watch {
        writer := uilive.New()
        writer.Start()

        ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
        defer stop()

        loop := func() {
            data := ports.AppsByPort()
            render(writer, data)
            writer.Flush()
        }

        loop()
        for {
            select {
            case <-ctx.Done():
                writer.Stop()
                os.Exit(0)
            case <-time.After(5 * time.Second):
                loop()
            }
        }
    }

    // default one-shot
    data := ports.AppsByPort()
    renderTable(os.Stdout, data)
}

func renderTable(w io.Writer, data map[int][]ports.PortEntry) {
    // collect and sort ports
    portsList := make([]int, 0, len(data))
    for p := range data {
        portsList = append(portsList, p)
    }
    sort.Ints(portsList)

    table := tablewriter.NewWriter(w)
    table.Header("PORT", "HOST", "APP BUNDLE", "PID(s)", "COMMAND SNIPPET", "FULL CMD")
    // default styling is acceptable; snippets may wrap if very long

    for _, port := range portsList {
        entries := data[port]
        if len(entries) == 0 {
            continue
        }

        // aggregate
        var bundle string
        var host string
        var pids []string
        var cmdSnippet string
        var fullCmd string

        for i, e := range entries {
            if i == 0 {
                if e.AppBundle != "" {
                    bundle = e.AppBundle
                } else {
                    bundle = e.Name
                }
                host = e.Host
                fullCmd = e.Cmdline
                cmdSnippet = fullCmd
                if len(cmdSnippet) > 50 {
                    cmdSnippet = cmdSnippet[:50] + "…"
                }
            }
            pids = append(pids, fmt.Sprintf("%d", e.Pid))
        }
        row := []string{
            fmt.Sprintf("%d", port),
            host,
            bundle,
            strings.Join(pids, ","),
            cmdSnippet,
            fullCmd,
        }
        table.Append(row)
    }

    table.Render()
}
