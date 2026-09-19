package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Method is how an update is performed.
type Method string

const (
	// MethodAuto prefers MethodRelease where an asset exists, else MethodGo.
	MethodAuto Method = "auto"
	// MethodRelease installs the signed asset from the GitHub release.
	MethodRelease Method = "release"
	// MethodGo rebuilds from source with `go install`.
	MethodGo Method = "go"
)

// Check is one prerequisite probe and its outcome.
type Check struct {
	Err       error
	Label     string
	Detail    string
	Essential bool // no method can work around a failure
}

// OK reports whether the check passed.
func (c Check) OK() bool { return c.Err == nil }

// Report is everything the probes learned.
type Report struct {
	ReleaseErr     error
	GoBinErr       error
	Checks         []Check
	ExecutablePath string
	GoBinDir       string
	ReleaseAsset   Asset
	IsRoot         bool
	HasGo          bool
	HasCosign      bool
}

// FirstEssentialError returns the first failure no method can work around.
func (r Report) FirstEssentialError() error {
	for _, c := range r.Checks {
		if c.Essential && c.Err != nil {
			return c.Err
		}
	}

	return nil
}

// Checker probes the machine. Every touchpoint is a field so tests can fake
// a machine that has none of the tools.
type Checker struct {
	Exec           Exec
	Executable     func() (string, error)
	LookPath       func(string) (string, error)
	WritableProbe  func(dir string) error
	Getuid         func() int
	GOOS           string
	GOARCH         string
	CurrentVersion string
}

// NewChecker returns a checker for this machine.
func NewChecker(currentVersion string, run Exec) *Checker {
	return &Checker{
		Exec:           run,
		Executable:     runningExecutable,
		LookPath:       exec.LookPath,
		WritableProbe:  probeWritable,
		Getuid:         os.Getuid,
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
		CurrentVersion: currentVersion,
	}
}

// runningExecutable resolves the running binary through symlinks.
func runningExecutable() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}

	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}

	return p, nil
}

// probeWritable checks writability by writing, not by reading mode bits:
// ACLs, read-only mounts and protected directories all let a mode check pass
// where a real write fails.
func probeWritable(dir string) error {
	f, err := os.CreateTemp(dir, "."+ToolName+"-probe-*")
	if err != nil {
		return err
	}
	name := f.Name()

	return errors.Join(f.Close(), os.Remove(name))
}

// Run performs the probes relevant to method.
func (c *Checker) Run(ctx context.Context, method Method) Report {
	var r Report

	r.Checks = append(r.Checks, c.checkVersion())

	exe, execCheck := c.checkExecutable()
	r.ExecutablePath = exe
	r.Checks = append(r.Checks, execCheck)

	asset, err := AssetFor(c.GOOS, c.GOARCH)
	r.ReleaseAsset, r.ReleaseErr = asset, err
	if method != MethodGo {
		check := Check{Label: "platform supported", Detail: c.GOOS + "/" + c.GOARCH, Essential: method == MethodRelease}
		if err != nil {
			check.Err = err
		}
		r.Checks = append(r.Checks, check)
	}

	if c.Getuid != nil {
		r.IsRoot = c.Getuid() == 0
	}

	if method != MethodRelease {
		goCheck, goBin := c.checkGo(ctx)
		r.HasGo = goCheck.Err == nil
		r.GoBinDir = goBin
		goCheck.Essential = method == MethodGo
		r.Checks = append(r.Checks, goCheck)

		if r.HasGo && goBin != "" {
			binCheck := Check{Label: "go bin directory writable", Detail: goBin}
			if err := c.WritableProbe(goBin); err != nil && !r.IsRoot {
				binCheck.Err = &NotWritableError{Dir: goBin, Err: err}
				r.GoBinErr = binCheck.Err
			}
			r.Checks = append(r.Checks, binCheck)
		}
	}

	if method != MethodGo && c.GOOS == "linux" && err == nil {
		check := Check{Label: "cosign", Detail: "available, Sigstore bundle will be verified"}
		if _, lookErr := c.LookPath("cosign"); lookErr != nil {
			check.Detail = "not installed, Sigstore verification will be skipped"
		} else {
			r.HasCosign = true
		}
		r.Checks = append(r.Checks, check)
	}

	return r
}

func (c *Checker) checkVersion() Check {
	check := Check{Label: "current version", Detail: c.CurrentVersion}
	if _, ok := ParseVersion(c.CurrentVersion); !ok {
		check.Detail = c.CurrentVersion + " (local build, not a release version)"
	}

	return check
}

func (c *Checker) checkExecutable() (string, Check) {
	check := Check{Label: "running binary", Essential: true}

	exe, err := c.Executable()
	if err != nil {
		check.Err = fmt.Errorf("cannot determine the running binary: %w", err)

		return "", check
	}

	check.Detail = exe

	return exe, check
}

// checkGo probes the Go toolchain and where `go install` would write.
func (c *Checker) checkGo(ctx context.Context) (Check, string) {
	check := Check{Label: "go toolchain"}

	res, err := c.Exec(ctx, "go", "version")
	if err != nil || res.ExitCode != 0 {
		check.Err = &MissingToolError{
			Tool:    "go",
			Purpose: "needed to rebuild from source with -method go",
			Install: "Install Go from https://go.dev/dl/ or use -method release.",
		}

		return check, ""
	}
	check.Detail = firstLine(res.Stdout)

	bin, err := c.goBinDir(ctx)
	if err != nil {
		check.Err = fmt.Errorf("cannot determine where go install writes: %w", err)

		return check, ""
	}

	return check, bin
}

// goBinDir returns GOBIN, or GOPATH/bin when GOBIN is unset.
func (c *Checker) goBinDir(ctx context.Context) (string, error) {
	res, err := c.Exec(ctx, "go", "env", "GOBIN", "GOPATH")
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", errors.New(firstLine(res.Text()))
	}

	lines := strings.Split(strings.ReplaceAll(res.Stdout, "\r\n", "\n"), "\n")
	if len(lines) < 2 {
		return "", fmt.Errorf("unexpected go env output %q", res.Stdout)
	}

	if gobin := strings.TrimSpace(lines[0]); gobin != "" {
		return gobin, nil
	}

	gopath := strings.TrimSpace(lines[1])
	if gopath == "" {
		return "", errors.New("GOPATH is empty")
	}

	// GOPATH can be a list; go install uses the first entry.
	if i := strings.IndexByte(gopath, filepath.ListSeparator); i >= 0 {
		gopath = gopath[:i]
	}

	return filepath.Join(gopath, "bin"), nil
}

// Resolve picks the method to use.
func (c *Checker) Resolve(method Method, r Report) (Method, error) {
	switch method {
	case MethodRelease:
		if r.ReleaseErr != nil {
			return "", r.ReleaseErr
		}

		return MethodRelease, nil

	case MethodGo:
		if !r.HasGo {
			return "", missingGo()
		}

		return MethodGo, nil

	default:
		if r.ReleaseErr == nil {
			return MethodRelease, nil
		}
		if r.HasGo {
			return MethodGo, nil
		}

		return "", &UnsupportedPlatformError{GOOS: c.GOOS, GOARCH: c.GOARCH}
	}
}

func missingGo() error {
	return &MissingToolError{
		Tool:    "go",
		Purpose: "needed to rebuild from source with -method go",
		Install: "Install Go from https://go.dev/dl/ or use -method release.",
	}
}

// CheckInstallTarget confirms the chosen method installs where the running
// binary lives, and that the process may write there.
//
// Neither `installer -pkg` nor `go install` consults the running binary: the
// package always writes PkgInstallDir and go install always writes GOBIN. An
// update landing elsewhere would report success while the copy on PATH
// stays old, so both directions are guarded. force overrides the guard.
func (c *Checker) CheckInstallTarget(method Method, asset Asset, r Report, force bool) Check {
	check := Check{Label: "install target"}
	running := filepath.Dir(r.ExecutablePath)

	switch {
	case method == MethodGo:
		check.Detail = r.GoBinDir
		if r.GoBinDir == "" {
			check.Err = errors.New("go install destination is unknown")

			return check
		}
		if !sameDir(running, r.GoBinDir) && !force {
			alt := ""
			if r.ReleaseErr == nil {
				alt = "-method release"
			}
			check.Err = &WrongInstallLocationError{Running: r.ExecutablePath, Destination: r.GoBinDir, Alternative: alt}

			return check
		}
		if r.GoBinErr != nil {
			check.Err = r.GoBinErr
		}

	case asset.Kind == KindPkg:
		check.Detail = PkgInstallDir
		if !sameDir(running, PkgInstallDir) && !force {
			alt := ""
			if r.HasGo && sameDir(running, r.GoBinDir) {
				alt = "-method go"
			}
			check.Err = &WrongInstallLocationError{Running: r.ExecutablePath, Destination: PkgInstallDir, Alternative: alt}

			return check
		}
		if !r.IsRoot {
			check.Err = &NeedsRootError{
				Reason:  "the macOS installer updates the system receipt database",
				Command: ToolName + " update -yes",
			}

			return check
		}
		check.Detail += " (running as root)"

	default: // zip, in place
		check.Detail = running + " (in place)"
		if err := c.WritableProbe(running); err != nil {
			check.Err = &NotWritableError{Dir: running, Err: err}
		}
	}

	return check
}

// sameDir compares two directories after cleaning, case-insensitively on
// case-insensitive platforms.
func sameDir(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}

	return a == b
}
