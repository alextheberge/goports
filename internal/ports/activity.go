// Package ports provides an event stream that clients can subscribe to
// in order to build real‑time graphs or other visualisations of port activity.
//
// The implementation is intentionally minimal: callers receive a channel of
// PortActivity records and are responsible for buffering/aggregating as
// appropriate for their UI.
//
// @mvs-feature("activity_stream")

package ports

import (
    "sync"
    "time"
)

// PortActivity represents a change in the set of listening ports.  When a
// port appears in the scan results it is reported with Open=true; when it
// disappears the same key is emitted with Open=false.  Timestamps reflect the
// time the scan detected the change, not the actual kernel event time.
//
// Future enhancements may include per-port counters (bytes sent/received) or
// per-entry details, but the current goal is to supply a simple stream of
// open/close events so that a consumer can plot counts over time.
type PortActivity struct {
    Key       PortKey
    Timestamp time.Time
    Open      bool
}

// activityCh is the shared channel for delivery of events.  It is buffered
// to avoid blocking the discovery path; consumers should drain it promptly or
// use a separate goroutine.
var activityCh = make(chan PortActivity, 256)

// history of recent events.  A simple slice is maintained with a mutex; when
// the capacity is exceeded the oldest entries are dropped.  This allows HTTP
// clients to request recent history without having been connected when the
// events occurred.
var (
    historyMu   sync.Mutex
    historyBuf  []PortActivity
    historyCap  = 1024 // keep the last 1024 events
)

// clearHistory wipes the history buffer.  It is intended for use in tests.
func clearHistory() {
    historyMu.Lock()
    historyBuf = nil
    historyMu.Unlock()
}

// SubscribeActivity returns a read-only channel that will receive port
// open/close events.  The channel is never closed; callers may safely keep a
// long-lived goroutine reading from it.  Multiple subscribers may coexist.
func SubscribeActivity() <-chan PortActivity {
    return activityCh
}

// publishActivity is used internally to send an event.  It drops the event if
// the buffer is full so that discovery is never blocked.  It also appends the
// event to the history buffer.
func publishActivity(evt PortActivity) {
    // send to channel
    select {
    case activityCh <- evt:
    default:
        // drop on full buffer
    }
    // append to history
    historyMu.Lock()
    if len(historyBuf) >= historyCap {
        // drop oldest
        historyBuf = historyBuf[1:]
    }
    historyBuf = append(historyBuf, evt)
    historyMu.Unlock()
}

// History returns up to `limit` events that occurred since the optional
// `since` time.  If limit==0 the full buffer is returned.  The results are in
// chronological order.
func History(since time.Time, limit int) []PortActivity {
    historyMu.Lock()
    defer historyMu.Unlock()
    var out []PortActivity
    for _, evt := range historyBuf {
        if !since.IsZero() && evt.Timestamp.Before(since) {
            continue
        }
        out = append(out, evt)
    }
    if limit > 0 && len(out) > limit {
        out = out[len(out)-limit:]
    }
    return out
}
