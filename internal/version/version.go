// Package version provides information about the current version of the applications.
// Any program in the cmd/ directory should import this package to access version information.
// The version information is set at build time using the -ldflags option.
// Example: go build -ldflags "-X 'github.com/yourusername/yourapp/internal/version.Version=1.0.0' -X 'github.com/yourusername/yourapp/internal/version.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)'"
// The variables are exported and can be accessed by importing this package.
// The variables are set to default values, which can be overridden at build time.
// The default values are:
// Version: "0.0.0"
// BuildDate: "1970-01-01T00:00:00Z"
// GitCommit: ""
// GitBranch: ""
// BuildUser: ""
// GoVersion: runtime.Version()
// GoVersionArch: runtime.GOARCH
// GoVersionOS: runtime.GOOS
// The GoVersion, GoVersionArch, and GoVersionOS variables are set to the current
// version of Go, architecture, and operating system used to build the application.
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
