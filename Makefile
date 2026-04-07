# legacy helper for the old Python/py2app workflow; files have been
# moved to `legacy/python` for posterity.  New development should ignore this target.
setup:
	python3 -m venv venv
	venv/bin/pip install -r legacy/python/requirements.txt

# Primary build target for the Go project.  Delegates to go-build so that
# `make build` produces the Go binary as expected during the bootstrap.
build: go-build

# Legacy Python packaging left for reference; developers can still invoke
# `make python-build` if they absolutely need the old py2app workflow.
python-build:
	# run the old py2app pipeline from its new location; this target exists
	# solely for archival purposes and will likely break in modern environments.
	venv/bin/python legacy/python/setup.py py2app
	zip -r dist/goports.zip dist/goports.app
	rm -rf dist/goports.app

clean:
	rm -rf build dist Ports.app goports.app
	$(MAKE) go-clean


# Go-specific helpers

go-build:
	go build -o bin/goports ./

# macOS bundle creation using the Go binary.  The resulting `goports.app`
# mirrors the old py2app output but drops the Python dependency entirely.
#
# Intended to be run from the project root; `go build` is invoked first so
# the binary exists before we start assembling the bundle structure.
build-app: go-build
	rm -rf goports.app
	mkdir -p goports.app/Contents/{MacOS,Resources}
	# copy the compiled binary; name it `goports` so CFBundleExecutable
	# matches the name used in Info.plist below.
	cp bin/goports goports.app/Contents/MacOS/goports
	# add the icon
	cp icon.icns goports.app/Contents/Resources/
	# write a minimal plist.  version strings are hard‑coded; bump when
	# you release a new bundle.
	printf '%s\n' \
		'<?xml version="1.0" encoding="UTF-8"?>' \
		'<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">' \
		'<plist version="1.0">' \
		'<dict>' \
		'    <key>CFBundleName</key><string>goports</string>' \
		'    <key>CFBundleDisplayName</key><string>goports</string>' \
		'    <key>CFBundleIdentifier</key><string>com.user.goports</string>' \
		'    <key>CFBundleExecutable</key><string>goports</string>' \
		'    <key>CFBundlePackageType</key><string>APPL</string>' \
		'    <key>CFBundleShortVersionString</key><string>0.2.0</string>' \
		'    <key>CFBundleVersion</key><string>0.2.0</string>' \
		'    <key>LSUIElement</key><true/>' \
		'    <key>CFBundleIconFile</key><string>icon.icns</string>' \
		'</dict>' \
		'</plist>' \
	> goports.app/Contents/Info.plist
# after the bundle is ready you can call `make dist` to zip it up

dist: build-app
	mkdir -p dist
	zip -r dist/goports.zip goports.app

# build the bundle and immediately launch it by running the
# contained executable.  `open` may be unreliable in headless sessions
# (or hide the app because LSUIElement suppresses the Dock icon), so we
# invoke the binary directly.  The menu‑bar icon should appear when the
# process starts.
run-app: build-app
	./goports.app/Contents/MacOS/goports --gui &


go-clean:
	rm -rf bin/

# Multidimensional versioning (MVS): https://github.com/alextheberge/MVSengine
# Install: curl -fsSL https://raw.githubusercontent.com/alextheberge/MVSengine/master/scripts/install.sh | bash
MVS_MANAGER ?= mvs-manager

.PHONY: vet test lint-mvs mvs-generate install-hooks ci

vet:
	go vet ./...

test:
	go test ./...

lint-mvs:
	$(MVS_MANAGER) lint --root . --manifest mvs.json --format text

mvs-generate:
	$(MVS_MANAGER) generate --root . --manifest mvs.json --context cli \
		--public-api-root internal/cli/cli.go \
		--go-export-following package-only \
		--exclude-path legacy/python

# Mirrors .github/workflows/ci.yml (vet, test, build, MVS). Requires mvs-manager on PATH.
ci: vet test build lint-mvs

install-hooks:
	git config core.hooksPath .githooks
	chmod +x .githooks/pre-commit


