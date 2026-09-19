package selfupdate

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Error is a failure that knows what the user should do next.
//
// The value of an update command is that it says what to do, so the guidance
// lives with the failure instead of being reconstructed by every caller.
type Error interface {
	error
	Remedy() string
}

// RemedyOf returns the remedy carried by err or any error it wraps, or "".
func RemedyOf(err error) string {
	if e, ok := errors.AsType[Error](err); ok {
		return e.Remedy()
	}

	return ""
}

// PrerequisiteError marks a failure that happened before anything was
// installed. The command maps it to a different exit code from an update that
// was attempted and broke, so a script can tell "fix and retry" from "damage
// may need attention".
type PrerequisiteError struct {
	Err error
}

func (e *PrerequisiteError) Error() string { return e.Err.Error() }

// Unwrap exposes the underlying failure, which carries the remedy.
func (e *PrerequisiteError) Unwrap() error { return e.Err }

// IsPrerequisite reports whether err is a prerequisite failure, so nothing was installed.
func IsPrerequisite(err error) bool {
	_, ok := errors.AsType[*PrerequisiteError](err)

	return ok
}

// NotComparableError is returned when the running version is not a release
// version and so cannot be ordered against one.
type NotComparableError struct {
	Version string
}

func (e *NotComparableError) Error() string {
	return fmt.Sprintf("the running version %q is not a release version, so it cannot be compared with a release", e.Version)
}

func (e *NotComparableError) Remedy() string {
	return "Name the release to install explicitly, e.g. " + ToolName + " update -version v0.2.0"
}

// UnsupportedPlatformError is returned when no release asset exists for the platform.
type UnsupportedPlatformError struct {
	GOOS   string
	GOARCH string
}

func (e *UnsupportedPlatformError) Error() string {
	return fmt.Sprintf("no release asset is published for %s/%s", e.GOOS, e.GOARCH)
}

func (e *UnsupportedPlatformError) Remedy() string {
	return "Rebuild from source instead: " + ToolName + " update -method go"
}

// MissingToolError is returned when a required external command is not on PATH.
type MissingToolError struct {
	Tool    string
	Purpose string
	Install string
}

func (e *MissingToolError) Error() string {
	return fmt.Sprintf("%s is not installed (%s)", e.Tool, e.Purpose)
}

func (e *MissingToolError) Remedy() string { return e.Install }

// NoConnectivityError is returned when GitHub could not be reached.
type NoConnectivityError struct {
	Err error
	URL string
}

func (e *NoConnectivityError) Error() string {
	return fmt.Sprintf("cannot reach %s: %v", e.URL, e.Err)
}

func (e *NoConnectivityError) Unwrap() error { return e.Err }

func (e *NoConnectivityError) Remedy() string {
	return "Check your network connection or proxy settings; github.com must be reachable over HTTPS."
}

// RateLimitedError is returned when GitHub asked us to back off.
type RateLimitedError struct {
	RetryAt time.Time
	Status  int
}

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("github.com answered HTTP %d (rate limited); retry after %s", e.Status, e.RetryAt.Local().Format(time.Kitchen))
}

func (e *RateLimitedError) Remedy() string {
	return "Wait until " + e.RetryAt.Local().Format(time.Kitchen) + " and run " + ToolName + " update again. Use -check in the meantime; it serves the cached answer."
}

// BudgetExhaustedError is returned when the local hourly lookup budget is spent.
type BudgetExhaustedError struct {
	NextAt time.Time
	Limit  int
	Window time.Duration
}

func (e *BudgetExhaustedError) Error() string {
	return fmt.Sprintf("the local limit of %d update checks per %s is reached; the next live check is allowed at %s",
		e.Limit, e.Window, e.NextAt.Local().Format(time.Kitchen))
}

func (e *BudgetExhaustedError) Remedy() string {
	return "Run " + ToolName + " update -check to see the cached result, or wait until " + e.NextAt.Local().Format(time.Kitchen) +
		". The limit protects github.com from repeated checks; " + EnvBudget + " raises it if you must."
}

// ReleaseNotFoundError is returned when a named tag does not exist.
type ReleaseNotFoundError struct {
	Tag string
}

func (e *ReleaseNotFoundError) Error() string {
	return fmt.Sprintf("release %q was not found in github.com/%s", e.Tag, Repository)
}

func (e *ReleaseNotFoundError) Remedy() string {
	return "See the published releases at https://github.com/" + Repository + "/releases and pass one of those tags to -version."
}

// AssetNotFoundError is returned when a release does not carry the expected asset.
type AssetNotFoundError struct {
	Asset string
	Tag   string
}

func (e *AssetNotFoundError) Error() string {
	return fmt.Sprintf("release %s does not carry %s", e.Tag, e.Asset)
}

func (e *AssetNotFoundError) Remedy() string {
	return "That release was published without this platform's asset. Pick another with -version, or rebuild from source with -method go."
}

// ChecksumMismatchError is returned when a download does not match its published SHA-256.
type ChecksumMismatchError struct {
	Asset    string
	Expected string
	Actual   string
}

func (e *ChecksumMismatchError) Error() string {
	return fmt.Sprintf("%s does not match its published SHA-256 (expected %s…, got %s…); nothing was installed",
		e.Asset, prefix(e.Expected, 12), prefix(e.Actual, 12))
}

func (e *ChecksumMismatchError) Remedy() string {
	return "The download is corrupt or tampered with. Retry; if it persists, report it at https://github.com/" + Repository + "/issues."
}

// SignatureError is returned when a signature check ran and failed, or was
// required and could not run.
type SignatureError struct {
	Asset  string
	Tool   string
	Detail string
}

func (e *SignatureError) Error() string {
	return fmt.Sprintf("signature verification of %s with %s failed: %s; nothing was installed", e.Asset, e.Tool, e.Detail)
}

func (e *SignatureError) Remedy() string {
	if e.Tool == "cosign" {
		return "Install cosign (https://docs.sigstore.dev/cosign/system_config/installation/) to verify Linux releases, or drop -require-signature to rely on the SHA-256 only."
	}

	return "Do not install this artefact. Download it again; if it still fails, report it at https://github.com/" + Repository + "/issues."
}

// NeedsRootError is returned when the install step requires root and the
// process does not have it. The update never escalates on its own.
type NeedsRootError struct {
	Reason  string
	Command string
}

func (e *NeedsRootError) Error() string {
	return "root privileges are required: " + e.Reason
}

func (e *NeedsRootError) Remedy() string {
	return "Re-run as root: sudo " + e.Command
}

// NotWritableError is returned when the directory holding the binary cannot be written.
type NotWritableError struct {
	Err error
	Dir string
}

func (e *NotWritableError) Error() string {
	return fmt.Sprintf("%s is not writable: %v", e.Dir, e.Err)
}

func (e *NotWritableError) Unwrap() error { return e.Err }

func (e *NotWritableError) Remedy() string {
	return "Re-run with enough privileges to write " + e.Dir + " (for example: sudo " + ToolName + " update -yes)."
}

// WrongInstallLocationError is returned when the chosen method would install
// somewhere other than where the running binary lives.
type WrongInstallLocationError struct {
	Running     string
	Destination string
	Alternative string // the flag that would target Running, or ""
}

func (e *WrongInstallLocationError) Error() string {
	return fmt.Sprintf("this method installs to %s, but the binary you are running is %s", e.Destination, e.Running)
}

func (e *WrongInstallLocationError) Remedy() string {
	if e.Alternative != "" {
		return "Use " + ToolName + " update " + e.Alternative + " to update the copy you are running, or -force to install to " + e.Destination + " anyway."
	}

	return "Nothing installs to " + e.Running + ". Pass -force to install to " + e.Destination + " and then use that copy, or replace the file by hand."
}

// prefix returns the first n characters of s.
func prefix(s string, n int) string {
	if len(s) <= n {
		return s
	}

	return s[:n]
}

// firstLine returns the first non-empty line of s, trimmed.
func firstLine(s string) string {
	for line := range strings.SplitSeq(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}

	return ""
}
