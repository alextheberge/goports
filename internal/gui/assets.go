// Package gui holds resources embedded for the GUI.
package gui

import _ "embed"

//go:embed icon.icns
var iconData []byte

//go:embed icon-dark.icns
var iconDarkData []byte
