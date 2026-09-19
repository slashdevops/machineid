package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Options are the choices one update run was given.
type Options struct {
	CurrentVersion   string
	Method           Method
	Version          string // exact tag; "" means latest
	Check            bool
	Refresh          bool
	Force            bool
	AssumeYes        bool
	RequireSignature bool
}

func (o Options) method() Method {
	if o.Method == "" {
		return MethodAuto
	}

	return o.Method
}

// Result describes what an update did.
type Result struct {
	Method  Method
	From    string
	To      string
	Tag     string
	Changed bool
}

// Plan is what an update intends to do, decided before anything is downloaded.
type Plan struct {
	Source   string // where the tag came from: "live, 4 of 5 checks left this hour" / "cached 12m ago"
	Tag      string
	Method   Method
	Asset    Asset
	UpToDate bool
}

// Updater runs an update end to end. The command is a thin adapter over it.
type Updater struct {
	Checker *Checker
	Client  *Client
	Budget  *Budget
	Exec    Exec

	// LookPath finds external tools; nil uses exec.LookPath.
	LookPath func(string) (string, error)

	// Out receives the progress narrative; nil discards it.
	Out io.Writer

	// Confirm asks the user to proceed; nil means proceed (use AssumeYes to be explicit).
	Confirm func(prompt string) (bool, error)

	// TempDir creates the staging directory; nil uses os.MkdirTemp.
	TempDir func() (string, error)

	// Logger receives debug detail; nil disables it.
	Logger *slog.Logger
}

// Run performs the update. Cheap checks come first and nothing is downloaded
// until the plan is certain to land where the running binary lives.
func (u *Updater) Run(ctx context.Context, opts Options) (Result, error) {
	result := Result{From: opts.CurrentVersion}

	report := u.Checker.Run(ctx, opts.method())
	u.printChecks(report.Checks)

	if err := report.FirstEssentialError(); err != nil {
		return result, &PrerequisiteError{Err: err}
	}

	if report.ExecutablePath != "" {
		CleanStaleBackup(report.ExecutablePath)
	}

	method, err := u.Checker.Resolve(opts.method(), report)
	if err != nil {
		return result, &PrerequisiteError{Err: err}
	}
	result.Method = method

	plan, err := u.plan(ctx, method, report, opts)
	if err != nil {
		return result, &PrerequisiteError{Err: err}
	}
	result.Tag = plan.Tag

	u.printChecks([]Check{{Label: "latest release", Detail: plan.Tag + "  (" + plan.Source + ")"}})

	target := u.Checker.CheckInstallTarget(method, plan.Asset, report, opts.Force)
	u.printChecks([]Check{target})
	if !target.OK() {
		return result, &PrerequisiteError{Err: target.Err}
	}

	if plan.UpToDate && !opts.Force {
		u.printf("\n✅ %s %s is already the latest version\n", ToolName, opts.CurrentVersion)

		return result, nil
	}

	u.printf("\n→ %s\n", describePlan(plan, opts.CurrentVersion))

	if opts.Check {
		u.printf("   (-check: nothing was changed)\n")

		return result, nil
	}

	if !opts.AssumeYes && u.Confirm != nil {
		proceed, err := u.Confirm(fmt.Sprintf("Update %s now?", ToolName))
		if err != nil {
			return result, err
		}
		if !proceed {
			u.printf("   cancelled\n")

			return result, nil
		}
	}

	if err := u.install(ctx, plan, report, opts); err != nil {
		return result, err
	}

	result.Changed = true
	result.To = u.confirmInstalled(ctx, plan, report)

	return result, nil
}

// plan resolves the tag (through the cache and budget) and names the asset.
func (u *Updater) plan(ctx context.Context, method Method, report Report, opts Options) (Plan, error) {
	tag, source, err := u.resolveTag(ctx, opts)
	if err != nil {
		return Plan{}, err
	}

	plan := Plan{Tag: tag, Source: source, Method: method}

	if opts.Version != "" {
		plan.UpToDate = SameVersion(opts.CurrentVersion, tag)
	} else {
		upgrade, err := IsUpgrade(opts.CurrentVersion, tag)
		if err != nil {
			return Plan{}, err
		}
		plan.UpToDate = !upgrade
	}

	if method == MethodGo {
		return plan, nil
	}

	plan.Asset = u.chooseReleaseAsset(ctx, report, tag)

	exists, err := u.Client.AssetExists(ctx, tag, plan.Asset.Name)
	if err != nil {
		return Plan{}, err
	}
	if !exists {
		return Plan{}, &AssetNotFoundError{Asset: plan.Asset.Name, Tag: tag}
	}

	return plan, nil
}

// chooseReleaseAsset picks the package on macOS only when the running binary
// lives where the package installs; otherwise the universal zip updates the
// binary in place when the release carries it. -force does not change the
// choice: reinstalling an equal version in place must not suddenly require
// root. Only when the release has no zip does -force fall through to the
// package, which then installs to PkgInstallDir as the target guard states.
func (u *Updater) chooseReleaseAsset(ctx context.Context, report Report, tag string) Asset {
	asset := report.ReleaseAsset
	if asset.Kind != KindPkg || sameDir(filepath.Dir(report.ExecutablePath), PkgInstallDir) {
		return asset
	}

	zipAsset := DarwinZipAsset()
	if ok, err := u.Client.AssetExists(ctx, tag, zipAsset.Name); err == nil && ok {
		return zipAsset
	}

	return asset
}

// resolveTag answers "which tag?" from the cache when fresh, otherwise live
// within the hourly budget.
func (u *Updater) resolveTag(ctx context.Context, opts Options) (tag, source string, err error) {
	if opts.Version != "" {
		if ok, next := u.Budget.Allow(); !ok {
			return "", "", &BudgetExhaustedError{Limit: u.Budget.Limit, Window: Window, NextAt: next}
		}

		exists, err := u.Client.TagExists(ctx, opts.Version)
		u.record("")
		if err != nil {
			return "", "", u.backoff(err)
		}
		if !exists {
			return "", "", &ReleaseNotFoundError{Tag: opts.Version}
		}

		return opts.Version, "requested", nil
	}

	if !opts.Refresh {
		if tag, age, ok := u.Budget.Cached(); ok {
			return tag, "cached " + age.Round(time.Minute).String() + " ago", nil
		}
	}

	ok, next := u.Budget.Allow()
	if !ok {
		if opts.Check {
			if tag, at, ok := u.Budget.CachedAny(); ok {
				return tag, fmt.Sprintf("cached at %s, next live check at %s", at.Local().Format(time.Kitchen), next.Local().Format(time.Kitchen)), nil
			}
		}

		return "", "", &BudgetExhaustedError{Limit: u.Budget.Limit, Window: Window, NextAt: next}
	}

	tag, err = u.Client.Latest(ctx)
	if err != nil {
		u.record("")

		return "", "", u.backoff(err)
	}
	u.record(tag)

	if u.Budget.Limit <= 0 {
		return tag, "live", nil
	}

	return tag, fmt.Sprintf("live, %d of %d checks left this hour", u.Budget.Remaining(), u.Budget.Limit), nil
}

func (u *Updater) record(tag string) {
	if err := u.Budget.RecordLookup(tag); err != nil {
		u.logDebug("could not persist update state", "error", err)
	}
}

// backoff persists a GitHub rate-limit answer so later runs honour it.
func (u *Updater) backoff(err error) error {
	if rl, ok := errors.AsType[*RateLimitedError](err); ok {
		if saveErr := u.Budget.RecordBackoff(rl.RetryAt); saveErr != nil {
			u.logDebug("could not persist backoff", "error", saveErr)
		}
	}

	return err
}

// install carries out the plan.
func (u *Updater) install(ctx context.Context, plan Plan, report Report, opts Options) error {
	if plan.Method == MethodGo {
		u.printf("   building from source with go install (this can take a minute)…\n")

		if err := GoInstall(ctx, u.Exec, opts.Version); err != nil {
			return err
		}
		u.printf("   ✓ installed into %s\n", report.GoBinDir)

		return nil
	}

	dir, cleanup, err := u.stagingDir()
	if err != nil {
		return err
	}
	defer cleanup()

	u.printf("   downloading %s…\n", plan.Asset.Name)

	archive, bundle, err := Fetch(ctx, u.Client, plan.Tag, plan.Asset, dir)
	if err != nil {
		return err
	}
	u.printf("   ✓ SHA-256 verified\n")

	if plan.Asset.Kind == KindPkg {
		if err := u.requireSignature(VerifyPkg(ctx, u.Exec, archive), plan.Asset.Name, opts); err != nil {
			return err
		}
		if err := InstallPkg(ctx, u.Exec, archive); err != nil {
			return err
		}
		u.printf("   ✓ installed to %s\n", PkgInstallDir)

		return nil
	}

	binary, err := ExtractBinary(archive, plan.Asset.InnerName, dir)
	if err != nil {
		return err
	}

	var v Verification
	if u.Checker.GOOS == "darwin" {
		v = VerifyCodesign(ctx, u.Exec, binary)
	} else {
		v = VerifySigstore(ctx, u.Exec, u.lookPath(), archive, bundle)
	}
	if err := u.requireSignature(v, plan.Asset.Name, opts); err != nil {
		return err
	}

	if err := ReplaceBinary(binary, report.ExecutablePath); err != nil {
		return err
	}
	u.printf("   ✓ installed to %s\n", report.ExecutablePath)

	return nil
}

// requireSignature reports a verification result and turns it into an error
// when it failed, or when it was skipped and the caller demanded a signature.
func (u *Updater) requireSignature(v Verification, asset string, opts Options) error {
	switch v.Status {
	case VerifyPassed:
		u.printf("   ✓ %s: %s\n", v.Tool, v.Detail)

		return nil
	case VerifyFailed:
		return &SignatureError{Asset: asset, Tool: v.Tool, Detail: v.Detail}
	default:
		if opts.RequireSignature {
			return &SignatureError{Asset: asset, Tool: v.Tool, Detail: v.Detail + " (-require-signature)"}
		}
		u.printf("   ! signature not verified: %s\n", v.Detail)

		return nil
	}
}

// confirmInstalled runs the installed binary and reports its version. A
// failure here is not an update failure, but a version that did not move is
// the one sign a fixed-destination install landed somewhere else.
func (u *Updater) confirmInstalled(ctx context.Context, plan Plan, report Report) string {
	path := report.ExecutablePath
	if plan.Method == MethodGo && report.GoBinDir != "" {
		path = filepath.Join(report.GoBinDir, ToolName+exeSuffix())
	}

	installed, err := ConfirmVersion(ctx, u.Exec, path)
	if err != nil {
		u.printf("   ! could not confirm the installed version: %v\n", err)

		return ""
	}

	if plan.Tag != "" && !SameVersion(installed, plan.Tag) {
		u.printf("   ! %s reports %s, not the %s just installed\n", path, installed, plan.Tag)
	}

	return installed
}

func exeSuffix() string {
	if os.PathSeparator == '\\' {
		return ".exe"
	}

	return ""
}

func (u *Updater) stagingDir() (string, func(), error) {
	maker := u.TempDir
	if maker == nil {
		maker = func() (string, error) { return os.MkdirTemp("", ToolName+"-update-*") }
	}

	dir, err := maker()
	if err != nil {
		return "", func() {}, fmt.Errorf("creating a staging directory: %w", err)
	}

	return dir, func() { _ = os.RemoveAll(dir) }, nil
}

func (u *Updater) lookPath() func(string) (string, error) {
	if u.LookPath != nil {
		return u.LookPath
	}

	return exec.LookPath
}

func (u *Updater) printChecks(checks []Check) {
	for _, c := range checks {
		if c.OK() {
			u.printf("  ✓ %-26s %s\n", c.Label, c.Detail)

			continue
		}
		u.printf("  ✗ %-26s %v\n", c.Label, c.Err)
	}
}

func (u *Updater) printf(format string, args ...any) {
	if u.Out != nil {
		fmt.Fprintf(u.Out, format, args...)
	}
}

func (u *Updater) logDebug(msg string, args ...any) {
	if u.Logger != nil {
		u.Logger.Debug(msg, args...)
	}
}

// describePlan renders what is about to happen.
func describePlan(plan Plan, current string) string {
	switch {
	case plan.Method == MethodGo:
		return fmt.Sprintf("Updating %s %s → %s from source with go install", ToolName, current, plan.Tag)
	case plan.Asset.Kind == KindPkg:
		return fmt.Sprintf("Updating %s %s → %s using the signed macOS package", ToolName, current, plan.Tag)
	default:
		return fmt.Sprintf("Updating %s %s → %s using the release archive %s", ToolName, current, plan.Tag, plan.Asset.Name)
	}
}
