// Package version provides build-time metadata for the CLI application.
//
// All variables have sensible defaults and can be overridden at build time
// using -ldflags:
//
//	go build -ldflags "\
//	  -X 'github.com/slashdevops/machineid/internal/version.Version=1.0.0' \
//	  -X 'github.com/slashdevops/machineid/internal/version.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)'"
package version

import "runtime"

var (
	// Version is the current version of the application
	Version = "0.0.0"

	// BuildDate is the date the application was built
	BuildDate = "1970-01-01T00:00:00Z"

	// GitCommit is the commit hash the application was built from
	GitCommit = ""

	// GitBranch is the branch the application was built from
	GitBranch = ""

	// BuildUser is the user that built the application
	BuildUser = ""

	// GoVersion is the version of Go used to build the application
	GoVersion = runtime.Version()

	// GoVersionArch is the architecture of Go used to build the application
	GoVersionArch = runtime.GOARCH

	// GoVersionOS is the operating system of Go used to build the application
	GoVersionOS = runtime.GOOS
)
