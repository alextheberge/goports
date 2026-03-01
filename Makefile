# legacy helper for the old Python/py2app workflow; kept only for
# historical reference.  New development should ignore this target.
setup:
	python3 -m venv venv
	venv/bin/pip install -r requirements.txt

# Primary build target for the Go project.  Delegates to go-build so that
# `make build` produces the Go binary as expected during the bootstrap.
build: go-build

# Legacy Python packaging left for reference; developers can still invoke
# `make python-build` if they absolutely need the old py2app workflow.
python-build:
	venv/bin/python setup.py py2app
	zip -r dist/Ports.zip dist/Ports.app
	rm -rf dist/Ports.app

clean:
	rm -rf build dist
	$(MAKE) go-clean


# Go-specific helpers

go-build:
	go build -o bin/goports ./

# macOS bundle creation using the Go binary.  The resulting `Ports.app`
# mirrors the old py2app output but drops the Python dependency entirely.
#
# Intended to be run from the project root; `go build` is invoked first so
# the binary exists before we start assembling the bundle structure.
build-app: go-build
	rm -rf Ports.app
	mkdir -p Ports.app/Contents/{MacOS,Resources}
	# copy the compiled binary; name it `goports` so CFBundleExecutable
	# matches the name used in Info.plist below.
	cp bin/goports Ports.app/Contents/MacOS/goports
	# add the icon
	cp icon.icns Ports.app/Contents/Resources/
	# write a minimal plist.  version strings are hard‑coded; bump when
	# you release a new bundle.
	printf '%s\n' \
		'<?xml version="1.0" encoding="UTF-8"?>' \
		'<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">' \
		'<plist version="1.0">' \
		'<dict>' \
		'    <key>CFBundleName</key><string>Ports</string>' \
		'    <key>CFBundleDisplayName</key><string>Ports</string>' \
		'    <key>CFBundleIdentifier</key><string>com.user.goports</string>' \
		'    <key>CFBundleExecutable</key><string>goports</string>' \
		'    <key>CFBundlePackageType</key><string>APPL</string>' \
		'    <key>CFBundleShortVersionString</key><string>0.2.0</string>' \
		'    <key>CFBundleVersion</key><string>0.2.0</string>' \
		'    <key>LSUIElement</key><true/>' \
		'    <key>CFBundleIconFile</key><string>icon.icns</string>' \
		'</dict>' \
		'</plist>' \
	> Ports.app/Contents/Info.plist
# after the bundle is ready you can call `make dist` to zip it up

dist: build-app
	mkdir -p dist
	zip -r dist/Ports.zip Ports.app

# build the bundle and immediately launch it by running the
# contained executable.  `open` may be unreliable in headless sessions
# (or hide the app because LSUIElement suppresses the Dock icon), so we
# invoke the binary directly.  The menu‑bar icon should appear when the
# process starts.
run-app: build-app
	./Ports.app/Contents/MacOS/goports --gui &


go-clean:
	rm -rf bin/


