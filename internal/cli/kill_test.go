package cli

import (
    "syscall"
    "testing"
)

// verify killPID is callable on all platforms; behaviour may vary.
func TestKillPIDSignature(t *testing.T) {
    // use a PID that is unlikely to exist
    err := killPID(0, syscall.SIGTERM)
    if err != nil {
        // on Unix this will probably be ESRCH; on Windows it may be an error
        t.Logf("killPID returned error (expected on some platforms): %v", err)
    }
}
