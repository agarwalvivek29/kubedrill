// Package buildinfo carries version metadata stamped at release time.
//
// Values are overridden via -ldflags "-X ...=" by goreleaser (Story 4.3);
// the defaults below apply to `go build` and dev runs.
package buildinfo

import "runtime/debug"

var (
	// Version is the semantic version, set at release via -ldflags.
	Version = "0.0.0-dev"
	// Commit is the git SHA, set at release via -ldflags.
	Commit = "none"
	// Date is the build date, set at release via -ldflags.
	Date = "unknown"
)

// String returns a single-line human-readable version string. When built from
// a module with VCS stamping and no explicit ldflags, it falls back to the
// Go-embedded build info commit.
func String() string {
	commit := Commit
	if commit == "none" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, s := range bi.Settings {
				if s.Key == "vcs.revision" && s.Value != "" {
					commit = s.Value
				}
			}
		}
	}
	return "kubedrill " + Version + " (commit " + commit + ", built " + Date + ")"
}
