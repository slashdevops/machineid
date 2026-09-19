package selfupdate

import (
	"fmt"
	"slices"
)

// Names shared by the whole package. They are the contract with the release
// pipeline: a test pins every asset name so a Makefile rename fails CI
// instead of breaking updates in the field.
const (
	// ToolName is the binary being updated.
	ToolName = "machineid"

	// Repository is the GitHub "owner/name" that publishes releases.
	Repository = "slashdevops/machineid"

	// Module is the Go module path, for `go install`.
	Module = "github.com/slashdevops/machineid"

	// CommandPath is the package `go install` builds.
	CommandPath = Module + "/cmd/machineid"

	// PkgInstallDir is where the macOS package always installs.
	PkgInstallDir = "/usr/local/bin"
)

// Kind is how a release asset has to be installed.
type Kind int

const (
	// KindZip is an archive holding the binary; installing means extract and replace in place.
	KindZip Kind = iota

	// KindPkg is a macOS installer package; installing means running `installer`,
	// which always writes to /usr/local/bin.
	KindPkg
)

// String implements fmt.Stringer.
func (k Kind) String() string {
	if k == KindPkg {
		return "pkg"
	}

	return "zip"
}

// Asset identifies one release artefact and the files that verify it.
type Asset struct {
	// Name is the filename within the GitHub release.
	Name string

	// ChecksumName holds the SHA-256 of Name (a bare hex digest).
	ChecksumName string

	// BundleName holds the Sigstore bundle for Name, or "" when none is published.
	BundleName string

	// InnerName is the filename inside the archive; "" for KindPkg.
	InnerName string

	// Kind is how to install it.
	Kind Kind
}

// releaseArches are the architectures the pipeline publishes; they match runtime.GOARCH.
var releaseArches = []string{"amd64", "arm64"}

// AssetFor returns the primary release asset for a platform.
//
// Linux gets the per-architecture zip. macOS gets the signed universal
// package, which installs to [PkgInstallDir]; a macOS binary living elsewhere
// is served by [DarwinZipAsset] instead, and the planner chooses between the
// two by looking at where the running binary is. Windows has no published
// asset and reports [UnsupportedPlatformError].
func AssetFor(goos, goarch string) (Asset, error) {
	if !slices.Contains(releaseArches, goarch) {
		return Asset{}, &UnsupportedPlatformError{GOOS: goos, GOARCH: goarch}
	}

	switch goos {
	case "linux":
		base := fmt.Sprintf("%s-linux-%s", ToolName, goarch)

		return Asset{
			Name:         base + ".zip",
			ChecksumName: base + ".sha256",
			BundleName:   base + ".sigstore.json",
			InnerName:    ToolName,
			Kind:         KindZip,
		}, nil

	case "darwin":
		base := ToolName + "-darwin-universal"

		return Asset{
			Name:         base + ".pkg",
			ChecksumName: base + ".sha256",
			Kind:         KindPkg,
		}, nil

	default:
		return Asset{}, &UnsupportedPlatformError{GOOS: goos, GOARCH: goarch}
	}
}

// DarwinZipAsset is the universal macOS zip, for updating a binary that does
// not live in [PkgInstallDir]. It carries the same signed and notarized
// binary as the package. Published from v0.2.0 on.
func DarwinZipAsset() Asset {
	base := ToolName + "-darwin-universal"

	return Asset{
		Name:         base + ".zip",
		ChecksumName: base + ".zip.sha256",
		InnerName:    ToolName,
		Kind:         KindZip,
	}
}
