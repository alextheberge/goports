//go:build darwin
// +build darwin

package ports

// discoverPorts uses lsof on Darwin to obtain a listing of listening
// sockets.  The output is intentionally the same format parsed by
// parseLsof so the common code need not change.
func discoverPorts() ([]byte, error) {
    return runCmd("lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-iUDP")
}
