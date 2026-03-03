# goports: Roadmap & TODO

This document tracks planned improvements, ordered roughly by expected user impact
and implementation effort. Items marked **(high return)** should be tackled first.

## 1. High‑return enhancements

1. ✅ **Add UDP listeners and IPv6 support** – expand discovery to show UDP, IPv6
   and UNIX domain sockets. (cross‑platform groundwork)
2. ✅ **JSON output & scripting-friendly formats** – `--json`/`--csv` for CLI.
3. ✅ **Improve CLI filtering & search** – allow filtering by port, name, bundle,
   address family; implement `--signal` and kill-by-name options.
4. ✅ **Cache DNS/bundle lookups** – reduce cost of repeated reverse DNS and
   bundleID calls.
5. ✅ **Context/timeouts on external commands** – wrap `exec.Command` calls with
   contexts so slow `lsof`/`ps` don't hang.
6. ✅ **Cross-platform foundation** –
   * abstraction layer (`discoverPorts`) added so platform-specific
     implementations can replace `lsof` in future.
   * Linux backend now natively parses `/proc/net/*`, inspects `/proc/<pid>/fd`
     and enriches entries with PIDs and command-lines.  Tests verify the
     parser and support caching.
   * Windows backend uses the IP Helper API and `QueryFullProcessImageName` to
     enumerate TCP/UDP listeners and associated processes; IPv6 support is
     complete, and a stub ensures non-Windows builds compile.
   * macOS backend has three paths:
     * `appsBySysctl` – native sysctl/`libproc` scan with PID/command merge and
       a 1‑second cache.  A `GOPORTS_FAKE_SYSCTL` env var and tests exercise
       error conditions.
     * `appsByNetstat` – lightweight parser used when native calls fail.
     * `lsof` – fallback that guarantees PID/command info; the GUI/CLI now
       automatically falls back if native results contain no PIDs.  Users can
       toggle `--native` / "Use native discovery only" to disable this
       fallback deliberately.
   * GUI items now include PID(s) and a truncated command-line in the parent
     title, and application icons are shown when bundle IDs are known.
   * Native-only checkbox/flag documented and elaborated in README.  Debug
     logging (`GOPORTS_DEBUG`) added.
   * platform-tagged mains created; non-darwin builds produce CLI-only
     binaries to simplify cross-platform support.
   * Next step: further harden native paths, implement caching on Linux/Win,
     and remove external tool dependencies entirely where possible.

## 2. GUI polish & usability

- ✅ Add search/filter box in menu-bar GUI.
- ✅ Dark mode support (icon variants) and basic accessibility support.
- ✅ Notifications per-port toggle with persisted settings.
- ✅ Preferences pane for refresh interval and protocols to show;
  TCP/UDP visibility checkboxes implemented.
- ✅ Restore PID/command‑line information and icons in menu titles; add
  titles to submenu items as well.  Debug logging added for metadata
  discovery and environment variables allow simulating failures.

## 3. Developer experience & packaging

- Add unit tests for `ports.AppsByPort` (fake lsof output).  Tests now
  cover sysctl failure and missing-PID fallback logic as well.
- Add debug helpers (`GOPORTS_DEBUG`, `GOPORTS_FAKE_SYSCTL`) and document
  them.
- Extract public library API and document it.
- Homebrew formula and GitHub Actions release workflow.
- Sign/notarize macOS bundle in CI.

## 4. Nice-to-have / long-term

- Local HTTP server for external tooling.
- Remote host support (SSH).
- iOS/watchOS companion widget.
- Real‑time port activity graphs.
  * first step was adding infrastructure in `internal/ports` to record open/close
    events and expose them via `SubscribeActivity` – this yields a channel of
    `PortActivity` records.
  * ✅ next, define a **sophisticated, robust API** that supports multiple
    consumers and pluggable transports (HTTP SSE implementation completed with
    built‑in minimal web UI; CLI flag added).
  * ✅ extend the API with optional TLS support, Unix domain socket binding,
    WebSocket endpoint and an OpenAPI/Swagger description so third-party
    tools can integrate easily.  Provide a JavaScript client that draws real–
    time charts using history and live events.  Consider adding CLI modes for
    plotting in-terminal or exporting CSV.
    1. keep the in‑process channel for callers within the same binary (GUI,
       CLI helpers, tests).
    2. expose optional adapters that deliver events over:
         * a local HTTP streaming endpoint (`/events` with Server-Sent Events or
           WebSockets) so external applications can ingest activity data in
           real time.
         * a UNIX domain socket or TCP port for local clients.
         * a memory‑mapped file or shared database for high‑throughput logging.
    3. allow querying recent history (in‑memory circular buffer) so clients
       can catch up after connecting.
    4. document the API: event structure, URL paths, query parameters, and
       example consumers (e.g. JavaScript web page, Python script).
  * ✅ once the API was defined, build a simple **local web GUI** that connects to
    it and draws a graph (using Chart.js in `ui.html`).  The interface loads
    historical events on startup, queries `/status` for a baseline count, and
    shows a live table of current ports with PID/command/bundle details via a
    `/ports` snapshot.  Controls allow resetting or downloading the data.
    A menu item in the macOS app opens the UI automatically.  Added search
    within the table and per‑port modal graphs so each listener can be plotted
    individually.
  * ✅ add support for opening the web UI inside a native webview embedded in
    the menu app (with fallback to the system browser).  The handler now uses
    `github.com/webview/webview` and `View Activity Graph` launches an
    internal window on macOS.
    * ✅ report failure when the webview cannot be created and notify the user
      before falling back to the system browser.
    * ✅ expose webview dimensions and debug flag via CLI arguments and saved
      preferences; added settings menu entries for width/height.
    * ✅ allow customization of the embedded window title and provide a reset
      option in the settings menu.
    * ✅ robustly handle webview creation by spawning a separate helper
      process (`--webview-child`) so the main menu app cannot crash.  On
      failure the menu item is disabled and the browser is opened; diagnostics
      are logged.  The helper now locks its OS thread before creating/running
      the WebKit view to avoid silent, non‑appearing windows (this was the
      cause of earlier mysterious failures).  Added support for positioning
      the child window (`x` from left, `y` from top), bring-it-to-front, and
      actively activate the helper so the view floats above other apps.  The
      window now animates into place on open rather than appearing abruptly.
    * ✅ add darwin build tag to the GUI package so the dependency is ignored on
      non‑mac platforms.
  * alternative rendering options include a termui dashboard and/or an
    optional CLI flag (`--show-graph`) that spawns a TUI.
  * ensure tests exercise the API adapters and history buffer.
- Consolidate logging/diagnostics across platforms (already added debug
  helpers but could offer GUI log viewer).
- Continue reducing/eliminating external command dependencies by finishing
  native `sysctl`/`/proc`/Win32 implementations and caching on all platforms.


> Each section can be broken into issues/PRs. Start by tackling high-return
> items; they’ll also lay groundwork for cross‑platform and testing work.