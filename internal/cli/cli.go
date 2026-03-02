// Package cli provides the command-line interface for the application.
package cli

import (
    "context"
    "encoding/csv"
    "encoding/json"
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
    var killName string
    var killBundle string
    var sigName string
    var openPort int
    var jsonOut bool
    var csvOut bool
    var filterProto string
    var filterName string
    var filterBundle string
    var filterFamily string
    var nativeOnly bool

    fs := flag.NewFlagSet("goports", flag.ExitOnError)
    fs.BoolVar(&watch, "watch", false, "refresh every 5s live")
    fs.BoolVar(&watch, "w", false, "shorthand for --watch")
    fs.IntVar(&killPort, "kill", 0, "port whose PIDs to kill (0=unset)")
    fs.IntVar(&openPort, "open", 0, "port to open in browser (0=unset)")
    fs.StringVar(&killName, "kill-name", "", "kill processes whose command contains this substring")
    fs.StringVar(&killBundle, "kill-bundle", "", "kill processes whose bundle identifier contains this substring")
    fs.StringVar(&sigName, "signal", "KILL", "signal to use when killing (e.g. TERM,INT,KILL)")
    fs.StringVar(&filterProto, "proto", "", "filter output to this protocol (tcp/udp)")
    fs.StringVar(&filterName, "name", "", "only show processes whose name contains this substring")
    fs.StringVar(&filterBundle, "bundle", "", "only show entries with bundle containing this")
    fs.StringVar(&filterFamily, "family", "", "only show entries with address family IPv4 or IPv6")
    fs.BoolVar(&jsonOut, "json", false, "output structured JSON")
    fs.BoolVar(&csvOut, "csv", false, "output CSV (PROTO,PORT,HOST,...)")
    fs.BoolVar(&nativeOnly, "native", false, "do not invoke lsof; use native discovery only")

    _ = fs.Parse(args)

    // apply the native-only flag to the discovery package
    if nativeOnly {
        ports.SetNativeOnly(true)
    }

    // kill actions take precedence.  we support port, name, and bundle
    // matching.  a signal name can be supplied via --signal.
    if killPort > 0 || killName != "" || killBundle != "" {
        data := ports.AppsByPort()
        // parse signal
        sig := syscall.SIGKILL
        if sigName != "" {
            if s, ok := signalMap[strings.ToUpper(sigName)]; ok {
                sig = s
            }
        }
        for key, entries := range data {
            for _, entry := range entries {
                if killPort > 0 && key.Port != killPort {
                    continue
                }
                if killName != "" && !strings.Contains(entry.Name, killName) {
                    continue
                }
                if killBundle != "" && !strings.Contains(entry.AppBundle, killBundle) {
                    continue
                }
                if err := syscall.Kill(int(entry.Pid), sig); err == nil {
                    fmt.Printf("Killed PID %d on %s (%s)\n", entry.Pid, key, sig)
                } else {
                    fmt.Fprintf(os.Stderr, "failed to kill PID %d on %s: %v\n", entry.Pid, key, err)
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

    render := func(w io.Writer, data map[ports.PortKey][]ports.PortEntry) {
        switch {
        case jsonOut:
            renderJSON(w, data)
        case csvOut:
            renderCSV(w, data)
        default:
            renderTable(w, data)
        }
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
    data = applyFilters(data, filterProto, filterName, filterBundle, filterFamily)
    render(os.Stdout, data)
}

func renderTable(w io.Writer, data map[ports.PortKey][]ports.PortEntry) {
    // collect and sort keys (protocol first, then port)
    keys := make([]ports.PortKey, 0, len(data))
    for k := range data {
        keys = append(keys, k)
    }
    sort.Slice(keys, func(i, j int) bool {
        if keys[i].Protocol == keys[j].Protocol {
            return keys[i].Port < keys[j].Port
        }
        return keys[i].Protocol < keys[j].Protocol
    })

    table := tablewriter.NewWriter(w)
    table.Header("PROTO", "PORT", "HOST", "APP BUNDLE", "FAMILY", "PID(s)", "COMMAND SNIPPET", "FULL CMD")
    // default styling is acceptable; snippets may wrap if very long

    for _, key := range keys {
        entries := data[key]
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
            key.Protocol,
            fmt.Sprintf("%d", key.Port),
            host,
            bundle,
            entries[0].Family,
            strings.Join(pids, ","),
            cmdSnippet,
            fullCmd,
        }
        table.Append(row)
    }

    table.Render()
}

// renderJSON writes the port data as a slice of JSON objects.
func renderJSON(w io.Writer, data map[ports.PortKey][]ports.PortEntry) {
    type record struct {
        Protocol  string   `json:"protocol"`
        Port      int      `json:"port"`
        Family    string   `json:"family,omitempty"`
        Host      string   `json:"host,omitempty"`
        AppBundle string   `json:"app_bundle,omitempty"`
        PIDs      []int32  `json:"pids"`
        Cmdline   string   `json:"cmdline,omitempty"`
    }
    var out []record
    for k, entries := range data {
        for _, e := range entries {
            rec := record{
                Protocol:  k.Protocol,
                Port:      k.Port,
                Family:    e.Family,
                Host:      e.Host,
                AppBundle: e.AppBundle,
                Cmdline:   e.Cmdline,
                PIDs:      []int32{e.Pid},
            }
            out = append(out, rec)
        }
    }
    enc := json.NewEncoder(w)
    enc.SetIndent("", "  ")
    _ = enc.Encode(out)
}

// renderCSV writes the port data as comma-separated values.  Header row is
// emitted and multi-PID entries are collapsed into semicolon-separated lists.
func renderCSV(w io.Writer, data map[ports.PortKey][]ports.PortEntry) {
    writer := csv.NewWriter(w)
    writer.Write([]string{"PROTO", "PORT", "FAMILY", "HOST", "APP_BUNDLE", "PIDS", "CMDLINE"})
    for k, entries := range data {
        for _, e := range entries {
            pids := fmt.Sprintf("%d", e.Pid)
            writer.Write([]string{k.Protocol, fmt.Sprintf("%d", k.Port), e.Family, e.Host, e.AppBundle, pids, e.Cmdline})
        }
    }
    writer.Flush()
}

// simple signal name -> value map used for --signal
var signalMap = map[string]syscall.Signal{
    "HUP": syscall.SIGHUP,
    "INT": syscall.SIGINT,
    "TERM": syscall.SIGTERM,
    "KILL": syscall.SIGKILL,
}

// applyFilters reduces the provided dataset according to filter parameters.
// Empty arguments are treated as wildcards.
func applyFilters(data map[ports.PortKey][]ports.PortEntry, proto, name, bundle, family string) map[ports.PortKey][]ports.PortEntry {
    if proto == "" && name == "" && bundle == "" && family == "" {
        return data
    }
    out := make(map[ports.PortKey][]ports.PortEntry)
    for k, entries := range data {
        if proto != "" && k.Protocol != proto {
            continue
        }
        if family != "" && !strings.EqualFold(entries[0].Family, family) {
            continue
        }
        for _, e := range entries {
            if name != "" && !strings.Contains(e.Name, name) {
                continue
            }
            if bundle != "" && !strings.Contains(e.AppBundle, bundle) {
                continue
            }
            out[k] = append(out[k], e)
        }
    }
    return out
}
