# goports – Local macOS Port Management Utility

![goports Screenshot](goports.png)

## Table of Contents

1. [Features](#features)
2. [Requirements](#requirements)
3. [Getting Started](#getting-started)
   * [Download](#download)
   * [Building](#building)
   * [Usage](#usage)
4. [Contribute](#contribute)


`goports` runs as a macOS menu‑bar app and CLI utility that shows every
listening TCP socket.  It's a single Go binary with no external runtime
requirements.

Whether you’re developing servers, debugging networking issues or simply
curious, goports lets you inspect, open or kill port owners without leaving
the keyboard.

---

## Features

goports exposes the same discovery engine to both a menu-bar GUI and a
command-line interface.  Highlights:

- **Real-time port monitoring** – all listening TCP sockets are listed and
  updated every few seconds.
- **Host name resolution** — reverse DNS is performed on each address, so
  `127.0.0.1` may appear as `localhost`.
- **Application identification** — see the executable name and, when
  possible, its `CFBundleIdentifier`.  The GUI also attempts to display the
  app’s icon (requires Spotlight indexing and the `sips` tool; the code now
  converts `.icns` via a temporary file, avoiding previous `sips` "exit status
  13" errors).  Run the GUI from a terminal and look for `iconForBundle:`
  messages on stderr if icons fail to load.
- **Process control** — terminate listeners directly from the menu or
  with `--kill`.
- **Browser integration** — `--open` or the GUI menu item launches
  `http://localhost:<port>` in the default browser.
- **Lightweight Go binary** — single executable produced with `go build`;
  legacy Python/py2app support is only retained for historical reference.
- **Configurable preferences** — a Settings submenu controls start‑at‑login
  (launch automatically when you sign in), notifications, and refresh
  interval; preferences persist in `~/.config/goports/settings.json`.

## Requirements

- macOS (apps use `lsof` and Cocoa menu bar APIs)
- `lsof` must be present on your PATH (installed by default on macOS)
- `sips` required if you want application icons in the GUI
- optional: Spotlight indexing enabled for icon resolution

## Getting Started

### Download

You can download a pre‑built bundle from the `dist` directory on the
repository, for example:

\`\`\`
https://raw.githubusercontent.com/alextheberge/goports/master/dist/goports.zip
\`\`\`

Alternatively clone the repo and build locally (see **Building** below).  
Legacy Python source has been moved to `legacy/python` for historical reference; the modern code is all Go.

### Building

The project is now a pure Go application; there is no Python dependency.
Use the standard `make` targets to compile and package.

\`\`\`bash
# compile the command‑line binary
make build                    # produces bin/goports

# build a macOS .app bundle and leave it in goports.app
make build-app

# build and immediately launch the app (handy while iterating)
# run-app starts the bundled binary with --gui.  Because the application
# sets `LSUIElement=true` in its Info.plist it does **not** show a Dock
# icon; look for the goports icon in the menu bar rather than expecting a
# window or dock entry.
make run-app

# create a zip suitable for releases (dist/goports.zip)
make dist
\`\`\`

Drop `goports.app` in your `/Applications` folder after building, or unzip the
archive produced by `make dist`.

> ⚠️ `make python-build` and `setup.py` are maintained only for historic
> reference; they no longer produce a usable application.

### Usage

`goports` enumerates listening TCP sockets via `lsof` and requires macOS.
Both GUI and CLI share the same engine; items appear in the menu bar and the
table automatically.

Behaviors you get “for free”:

* reverse DNS on local addresses (`127.0.0.1` → `localhost`)
* process bundle ID and icon lookups when available

#### CLI flags

```sh
--gui          # launch menu‑bar app (default)
--watch, -w    # refresh every N seconds
--kill PORT    # kill processes on PORT
--open PORT    # open http://localhost:PORT in browser
```

#### GUI Settings

When running in GUI mode a "Settings" submenu is available.  The following
preferences are persisted across launches:

* **Start at Login** – toggles whether goports is added to your macOS login
  items.  Enabling will attempt to create/delete a System Events login item
  via AppleScript; failures are non‑fatal.
* **Enable Notifications** – when on, the menu will send native notifications
  for ports opening and closing.  Turning it off silences those alerts.
* **Refresh interval** – click repeatedly to cycle the polling interval
  between 5, 10 and 15 seconds; the current value is shown in the menu label.

These settings are stored in `~/.config/goports/settings.json`.

* `--gui` — launch the menu‑bar GUI (default when no flags are provided).
* `--watch`, `-w` — refresh the CLI output every 5 seconds.
* `--kill <port>` — terminate all processes listening on `<port>`.
* `--open <port>` — open `http://localhost:<port>` in the default browser.

Examples:

\`\`\`bash
# show a one‑shot table of current ports
./bin/goports

# continuously update the table until interrupted
./bin/goports --watch

# kill whatever is bound to 8080
./bin/goports --kill 8080

# open a local web server on 3000
./bin/goports --open 3000

# start the GUI explicitly (normally invoked by double-clicking goports.app)
./bin/goports --gui
\`\`\`

The GUI mode is also the default when you launch `goports.app` from Finder.

### Contribute

Feel free to open issues or PRs.  The implementation lives under `internal/` and
is intentionally small — adding new features such as cross-platform support,
stats collection, or UI polish is straightforward.
