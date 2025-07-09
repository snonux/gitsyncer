package version

import (
	"fmt"
	"runtime"
)

var (
	// Version is the current version of gitsyncer
	Version = "0.5.0"

	// GitCommit is the git commit hash at build time
	GitCommit = "unknown"

	// BuildDate is the date when the binary was built
	BuildDate = "unknown"

	// GoVersion is the Go version used to build
	GoVersion = runtime.Version()
)

// GetVersion returns the full version string
func GetVersion() string {
	return fmt.Sprintf("gitsyncer version %s (commit: %s, built: %s, go: %s)",
		Version, GitCommit, BuildDate, GoVersion)
}

// GetShortVersion returns just the version number
func GetShortVersion() string {
	return Version
}
