//go:build linux
// +build linux

package ports


// discoverPorts for Linux currently shells out to lsof.  This avoids the
// need to replicate /proc parsing logic while still providing working
// functionality; the abstraction means the implementation can be replaced
// later with a native parser without touching the rest of the code.
func discoverPorts() ([]byte, error) {
    return runCmd("lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-iUDP")
}
