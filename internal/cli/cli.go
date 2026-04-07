// Package cli provides the command-line interface for the application.
//
// @mvs-feature("port_monitoring")
// @mvs-feature("process_control")
// @mvs-protocol("goports-cli-v1")
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
	"text/tabwriter"
	"time"

	"github.com/gosuri/uilive"
	"github.com/guptarohit/asciigraph"
	"github.com/olekukonko/tablewriter"
	"golang.org/x/term"

	"github.com/user/goports/internal/applog"
	"github.com/user/goports/internal/exitcode"
	"github.com/user/goports/internal/ports"
	"github.com/user/goports/internal/version"
)

// Run executes the CLI using the provided unparsed arguments. Exit codes: 0 ok,
// 1 error, 3 kill failures; flag parse errors use 2 (from the flag package).
func Run(args []string) int {
	var showHelp bool
	var showVersion bool

	// define flags
	var watch bool
	var killPort int
	var killName string
	var killBundle string
	var sigName string
	var openPort int
	var jsonOut bool
	var csvOut bool
	var exportHist bool
	var tui bool
	var filterProto string
	var filterName string
	var filterBundle string
	var filterFamily string
	var nativeOnly bool
	var quiet bool

	fs := flag.NewFlagSet("goports", flag.ExitOnError)
	fs.SetOutput(os.Stdout)
	fs.Usage = func() {
		out := fs.Output()
		fmt.Fprintf(out, "Usage: goports [flags]\n\n")
		fmt.Fprintf(out, "List listening TCP/UDP ports and owning processes. On macOS, --gui starts the menu-bar app (handled by the binary, not shown below).\n\n")
		fmt.Fprintf(out, "Output & display\n")
		fmt.Fprintf(out, "  Default is a table. Use --json, --csv, or --tui for other views. --watch refreshes every 5s.\n\n")
		fmt.Fprintf(out, "Actions\n")
		fmt.Fprintf(out, "  --kill, --kill-name, --kill-bundle with --signal; --open; --export; --spec; --http-port for the local API.\n\n")
		fmt.Fprintf(out, "Filters\n")
		fmt.Fprintf(out, "  --proto, --name, --bundle, --family; --native for discovery mode.\n\n")
		fmt.Fprintf(out, "GUI-only flags (ignored here; used when the darwin binary runs the menu bar or webview helper)\n")
		fmt.Fprintf(out, "  --gui, --webview-*, --webview-child\n\n")
		fmt.Fprintf(out, "Exit codes: 0 success, 1 error, 2 invalid flags (from Go's flag package), 3 kill action had failures.\n\n")
		fmt.Fprintf(out, "Flags:\n")
		fs.PrintDefaults()
	}
	fs.BoolVar(&showHelp, "h", false, "show usage and exit")
	fs.BoolVar(&showHelp, "help", false, "show usage and exit")
	fs.BoolVar(&showVersion, "version", false, "print version and exit")

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
	fs.BoolVar(&exportHist, "export", false, "dump historical events as JSON and exit")
	fs.BoolVar(&tui, "tui", false, "show a simple ASCII graph of open ports in the terminal")
	fs.BoolVar(&nativeOnly, "native", false, "do not invoke lsof; use native discovery only")
	fs.BoolVar(&quiet, "q", false, "quiet table output (no heavy borders); for --watch, skip live cursor mode when stdout is not a TTY")
	fs.BoolVar(&quiet, "quiet", false, "same as -q")
	var httpPort int
	fs.IntVar(&httpPort, "http-port", 0, "if nonzero, start an HTTP server exposing /events on this port")
	var specFlag bool
	fs.BoolVar(&specFlag, "spec", false, "print the OpenAPI JSON and exit")

	// Parsed by package main on darwin before CLI args are forwarded. Registered
	// here so non-darwin mains, tests, and tooling can pass a shared argv.
	var guiUnused bool
	var webviewW, webviewH, webviewX, webviewY int
	var webviewDebug bool
	var webviewTitle string
	var webviewChild bool
	fs.BoolVar(&guiUnused, "gui", false, "menu-bar GUI (darwin only; ignored here)")
	fs.IntVar(&webviewW, "webview-width", 0, "embedded webview width (GUI only)")
	fs.IntVar(&webviewH, "webview-height", 0, "embedded webview height (GUI only)")
	fs.IntVar(&webviewX, "webview-x", 0, "embedded webview X position (GUI only)")
	fs.IntVar(&webviewY, "webview-y", 0, "embedded webview Y position (GUI only)")
	fs.BoolVar(&webviewDebug, "webview-debug", false, "webview debug (GUI only)")
	fs.StringVar(&webviewTitle, "webview-title", "", "embedded webview title (GUI only)")
	fs.BoolVar(&webviewChild, "webview-child", false, "internal webview helper process (GUI only)")

	_ = fs.Parse(args)

	if showHelp {
		fs.Usage()
		return exitcode.OK
	}
	if showVersion {
		fmt.Printf("goports %s\n", version.Version)
		return exitcode.OK
	}

	plainWatch := quiet || (watch && !jsonOut && !csvOut && !tui && !term.IsTerminal(int(os.Stdout.Fd())))

	// apply the native-only flag to the discovery package
	if nativeOnly {
		ports.SetNativeOnly(true)
	}

	// print spec and exit before starting server or doing other work
	if specFlag {
		fmt.Println(ports.OpenAPISpec())
		return exitcode.OK
	}

	// optionally start the event HTTP server
	if httpPort > 0 {
		addr := fmt.Sprintf(":%d", httpPort)
		_, cleanup, err := ports.StartEventServer(addr)
		if err != nil {
			applog.Logger().Error("event server failed", "addr", addr, "err", err)
			return exitcode.Error
		}
		if cleanup != nil {
			defer cleanup()
		}
	}

	// export history if requested
	if exportHist {
		hist := ports.History(time.Time{}, 0)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(hist)
		return exitcode.OK
	}

	// kill actions take precedence.  we support port, name, and bundle
	// matching.  a signal name can be supplied via --signal.
	if killPort > 0 || killName != "" || killBundle != "" {
		data := ports.AppsByPort()
		sig := syscall.SIGKILL
		if sigName != "" {
			s, ok := signalMap[strings.ToUpper(sigName)]
			if !ok {
				fmt.Fprintf(os.Stderr, "unknown signal %q (use HUP, INT, TERM, KILL)\n", sigName)
				return exitcode.Error
			}
			sig = s
		}
		var killOK, killFail int
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
				if entry.Pid == 0 {
					applog.Logger().Error("cannot kill: PID unknown (try without --native)", "key", key.String())
					killFail++
					continue
				}
				if err := killPID(int(entry.Pid), sig); err == nil {
					if !quiet {
						fmt.Printf("Killed PID %d on %s (%s)\n", entry.Pid, key, sig)
					}
					killOK++
				} else {
					applog.Logger().Error("kill failed", "pid", entry.Pid, "key", key.String(), "err", err)
					killFail++
				}
			}
		}
		if killOK == 0 && killFail == 0 {
			fmt.Fprintln(os.Stderr, "no matching processes to kill")
			return exitcode.Error
		}
		if killFail > 0 {
			return exitcode.KillFailed
		}
		return exitcode.OK
	}

	if openPort > 0 {
		url := fmt.Sprintf("http://localhost:%d", openPort)
		if !quiet {
			fmt.Printf("Opening %s\n", url)
		}
		if err := exec.Command("open", url).Run(); err != nil {
			applog.Logger().Error("open URL failed", "url", url, "err", err)
			return exitcode.Error
		}
		return exitcode.OK
	}

	if tui {
		runTUI()
		return exitcode.OK
	}

	render := func(w io.Writer, data map[ports.PortKey][]ports.PortEntry) {
		switch {
		case jsonOut:
			renderJSON(w, data)
		case csvOut:
			renderCSV(w, data)
		default:
			renderTable(w, data, quiet)
		}
	}

	if watch {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		if plainWatch {
			loop := func() {
				data := ports.AppsByPort()
				data = applyFilters(data, filterProto, filterName, filterBundle, filterFamily)
				if !quiet {
					fmt.Print("\033[H\033[2J")
				}
				render(os.Stdout, data)
			}
			loop()
			for {
				select {
				case <-ctx.Done():
					return exitcode.OK
				case <-time.After(5 * time.Second):
					loop()
				}
			}
		}

		writer := uilive.New()
		writer.Start()
		loop := func() {
			data := ports.AppsByPort()
			data = applyFilters(data, filterProto, filterName, filterBundle, filterFamily)
			render(writer, data)
			writer.Flush()
		}
		loop()
		for {
			select {
			case <-ctx.Done():
				writer.Stop()
				return exitcode.OK
			case <-time.After(5 * time.Second):
				loop()
			}
		}
	}

	// default one-shot
	data := ports.AppsByPort()
	data = applyFilters(data, filterProto, filterName, filterBundle, filterFamily)
	render(os.Stdout, data)
	return exitcode.OK
}

func runTUI() {
	var counts []float64
	for {
		data := ports.AppsByPort()
		counts = append(counts, float64(len(data)))
		if len(counts) > 50 {
			counts = counts[len(counts)-50:]
		}
		fmt.Print("\033[H\033[2J") // clear screen
		fmt.Println(asciigraph.Plot(counts, asciigraph.Height(10), asciigraph.Caption("open ports")))
		time.Sleep(5 * time.Second)
	}
}

func renderTable(w io.Writer, data map[ports.PortKey][]ports.PortEntry, quiet bool) {
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

	if quiet {
		tw := tabwriter.NewWriter(w, 0, 8, 2, ' ', 0)
		fmt.Fprintln(tw, "PROTO\tPORT\tHOST\tBUNDLE\tFAMILY\tPID(s)\tCMD")
		for _, key := range keys {
			entries := data[key]
			if len(entries) == 0 {
				continue
			}
			var bundle string
			if entries[0].AppBundle != "" {
				bundle = entries[0].AppBundle
			} else {
				bundle = entries[0].Name
			}
			var pids []string
			for _, e := range entries {
				pids = append(pids, fmt.Sprintf("%d", e.Pid))
			}
			cmd := entries[0].Cmdline
			if len(cmd) > 60 {
				cmd = cmd[:60] + "…"
			}
			fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\t%s\t%s\n",
				key.Protocol, key.Port, entries[0].Host, bundle, entries[0].Family,
				strings.Join(pids, ","), cmd)
		}
		_ = tw.Flush()
		return
	}

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
		Protocol  string  `json:"protocol"`
		Port      int     `json:"port"`
		Family    string  `json:"family,omitempty"`
		Host      string  `json:"host,omitempty"`
		AppBundle string  `json:"app_bundle,omitempty"`
		PIDs      []int32 `json:"pids"`
		Cmdline   string  `json:"cmdline,omitempty"`
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
	"HUP":  syscall.SIGHUP,
	"INT":  syscall.SIGINT,
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
