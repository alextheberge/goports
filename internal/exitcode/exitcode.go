// Package exitcode defines stable process exit codes for the goports CLI.
package exitcode

// Exit codes for automation (in addition to Go's flag package using 2 on parse errors).
const (
	OK         = 0
	Error      = 1 // operational failure (I/O, kill, bind, open URL, unknown --signal)
	KillFailed = 3 // kill action ran but at least one target failed
)
