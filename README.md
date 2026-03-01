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


`goports` lives in the menu bar and on the command line, giving you a
real‑time view of every TCP listener on your Mac.  It's written in pure Go and
has no runtime dependencies beyond the standard toolchain.

Ideal for developers, sysadmins or anyone who needs to know *what* is
listening on a given port, the application lets you inspect, open, or kill
processes without leaving the keyboard.

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
  app’s icon (requires Spotlight indexing and the `sips` tool; debug messages
  appear on stderr).
- **Process control** — terminate listeners directly from the menu or
  with `--kill`.
- **Browser integration** — `--open` or the GUI menu item launches
  `http://localhost:<port>` in the default browser.
- **Lightweight Go binary** — single executable produced with `go build`;
  legacy Python/py2app support is only retained for historical reference.
- **Configurable preferences** — a Settings submenu controls start‑at‑login,
  notifications, and refresh interval; preferences persist in
  `~/.config/goports/settings.json`.

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

goports uses `lsof` under the hood to enumerate listening TCP sockets on macOS.
Because of that it requires the host to be macOS and for `lsof` to be available
on the PATH (it is installed by default). The GUI and CLI share the same
discovery logic; once a listener is detected it will appear in the menu bar and
in the CLI table.

For convenience the tool also performs:

* **reverse DNS lookups** on the local address and shows the hostname if
  resolvable (e.g. `127.0.0.1` → `localhost`).
* **bundle identifier resolution** for macOS processes – the `APP BUNDLE`
  column in the table will show the `CFBundleIdentifier` when available.

The executable supports a handful of command‑line flags for both GUI and
headless workflows.

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

Pull requests and issues are welcome! If you're looking for low‑hanging fruit,
the codebase is in Go under `internal/` and there are TODOs scattered
throughout. I haven't yet implemented a startup/launch‑at‑login feature, so
contributions in that area would be especially appreciated.
