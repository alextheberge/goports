# Ports - Local Mac Port Management Tool

![Ports Screenshot](ports.png)

## Introduction
**Ports** is a streamlined Mac application designed to enhance your network
management experience. Implemented in Go, this open-source tool provides a
real-time overview of all local ports, allowing you to quickly identify and
manage the applications using them, right from your Mac's menu bar.

Whether you're a developer, network administrator, or just curious about your
system's network connections, Ports offers a user-friendly interface to monitor
and control your local ports efficiently.

## Features

Ports provides both a menu-bar GUI and a command-line interface built from the
same discovery engine. Key capabilities include:

- **Real-time port monitoring** – all listening TCP sockets are listed and
  updated every few seconds.
- **Host name resolution** – the local address is reverse‑DNS‑looked up and
  shown when available (e.g. `127.0.0.1` → `localhost`).
- **Application identification** – the process name and, when available, the
  macOS `CFBundleIdentifier` are displayed.
- **Process control** – kill any listener directly from the GUI or CLI.
- **Browser integration** – open a port in the default web browser via CLI flag
  or menu item.
- **Lightweight Go binary** – no Python runtime, simple `go build` and `make`
  wrappers.

## Getting Started

### Download

You can download a pre‑built bundle from the `dist` directory on the
repository, for example:

\`\`\`
https://raw.githubusercontent.com/ronreiter/ports/master/dist/Ports.zip
\`\`\`

Alternatively clone the repo and build locally (see **Building** below).

### Building

The project is now a pure Go application; there is no Python dependency.
Use the standard `make` targets to compile and package.

\`\`\`bash
# compile the command‑line binary
make build                    # produces bin/goports

# build a macOS .app bundle and leave it in Ports.app
make build-app

# build and immediately launch the app (handy while iterating)
# run-app starts the bundled binary with --gui.  Because the application
# sets `LSUIElement=true` in its Info.plist it does **not** show a Dock
# icon; look for the Ports icon in the menu bar rather than expecting a
# window or dock entry.
make run-app

# create a zip suitable for releases (dist/Ports.zip)
make dist
\`\`\`

Drop `Ports.app` in your `/Applications` folder after building, or unzip the
archive produced by `make dist`.

> ⚠️ `make python-build` and `setup.py` are maintained only for historic
> reference; they no longer produce a usable application.

### Usage

Ports uses `lsof` under the hood to enumerate listening TCP sockets on macOS.
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

# start the GUI explicitly (normally invoked by double-clicking Ports.app)
./bin/goports --gui
\`\`\`

The GUI mode is also the default when you launch `Ports.app` from Finder.

### Contribute

Pull requests and issues are welcome! If you're looking for low‑hanging fruit,
the codebase is in Go under `internal/` and there are TODOs scattered
throughout. I haven't yet implemented a startup/launch‑at‑login feature, so
contributions in that area would be especially appreciated.
