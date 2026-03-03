//go:build !darwin
// +build !darwin

package gui

import "testing"

// simple smoke test to ensure the non-darwin GUI symbols compile.
func TestNonDarwinSymbols(t *testing.T) {
    // not executed; we just reference the symbols so the linker includes them.
    _ = Run
    _ = handleGraphClick
}
