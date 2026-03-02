// Package gui contains helper code for visual components that are used by
// the macOS menu bar application.  Graph-related functionality lives here.
//
// Currently this file only provides the skeleton for real-time activity
// visualisation.  The implementation will listen on the ports activity stream
// (see `ports.SubscribeActivity`) and render a chart or sparkline alongside
// the existing menu items.  The concrete rendering technology is TBD; possible
// approaches include embedding a small webview, using a terminal-style UI
// library, or drawing directly with Cocoa APIs.

package gui

import (
    "github.com/user/goports/internal/ports"
)

// startGraphing kicks off a goroutine that consumes port activity events and
// updates internal state.  The caller (tickerLoop or onReady) can invoke this
// when the GUI initializes.  The implementation is currently a no-op, but the
// helper exists to make it easy to wire up later.
func startGraphing() {
    go func() {
        ch := ports.SubscribeActivity()
        for evt := range ch {
            // TODO: buffer timestamps and counts, then trigger a redraw
            _ = evt
        }
    }()
}
