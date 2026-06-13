// Package version exposes build version information, stamped via -ldflags at build time.
package version

import "fmt"

// These values are overridden at build time with -ldflags.
var (
	Version = "0.1.0-dev"
	Commit  = "unknown"
	Date    = "unknown"
)

// String returns a human-readable version string.
func String() string {
	return fmt.Sprintf("calcassist %s (commit %s, built %s)", Version, Commit, Date)
}
