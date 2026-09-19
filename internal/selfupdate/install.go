package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// installerTimeout bounds `installer -pkg`; it verifies the signature and
// updates the receipt database, so it is not instant.
const installerTimeout = 5 * time.Minute

// goInstallTimeout bounds `go install`, which may populate a cold module cache.
const goInstallTimeout = 15 * time.Minute

// backupSuffix is appended to a running Windows executable before it is
// replaced: Windows refuses to overwrite a running .exe but allows renaming it.
const backupSuffix = ".old"

// ReplaceBinary atomically replaces the executable at target with source.
//
// The new binary is staged in target's own directory (os.Rename cannot cross
// filesystems, and /tmp often is one), made executable, then renamed over the
// target in one step. A failure leaves the old binary untouched and removes
// the staging file. The running process keeps its open inode; the next
// invocation is the new build.
func ReplaceBinary(source, target string) error {
	dir := filepath.Dir(target)

	staged, err := os.CreateTemp(dir, "."+filepath.Base(target)+".new-*")
	if err != nil {
		return fmt.Errorf("staging the new binary next to %s: %w", target, err)
	}
	stagedPath := staged.Name()

	cleanup := func(cause error) error {
		return errors.Join(cause, os.Remove(stagedPath))
	}

	src, err := os.Open(source)
	if err != nil {
		return cleanup(errors.Join(fmt.Errorf("opening the downloaded binary: %w", err), staged.Close()))
	}

	_, copyErr := io.Copy(staged, src)
	closeErrs := errors.Join(src.Close(), staged.Close())

	if copyErr != nil {
		return cleanup(errors.Join(fmt.Errorf("writing the new binary: %w", copyErr), closeErrs))
	}
	if closeErrs != nil {
		return cleanup(closeErrs)
	}

	if err := os.Chmod(stagedPath, 0o755); err != nil {
		return cleanup(fmt.Errorf("making the new binary executable: %w", err))
	}

	if runtime.GOOS == "windows" {
		if err := moveAsideRunning(target); err != nil {
			return cleanup(err)
		}
	}

	if err := os.Rename(stagedPath, target); err != nil {
		return cleanup(fmt.Errorf("replacing %s: %w", target, err))
	}

	return nil
}

// moveAsideRunning renames target to target.old so it can be replaced while
// it is executing (Windows only). The stale copy is removed by CleanStaleBackup
// on the next run.
func moveAsideRunning(target string) error {
	backup := target + backupSuffix
	_ = os.Remove(backup)

	if err := os.Rename(target, backup); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("moving the running binary aside: %w", err)
	}

	return nil
}

// CleanStaleBackup removes a leftover <target>.old from a previous Windows update.
func CleanStaleBackup(target string) {
	_ = os.Remove(target + backupSuffix)
}

// InstallPkg installs a macOS package with the system installer. It always
// writes PkgInstallDir and needs root; the caller establishes both.
func InstallPkg(ctx context.Context, run Exec, pkgPath string) error {
	ctx, cancel := context.WithTimeout(ctx, installerTimeout)
	defer cancel()

	res, err := run(ctx, "installer", "-pkg", pkgPath, "-target", "/")
	if err != nil {
		return fmt.Errorf("running the macOS installer: %w", err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("the macOS installer failed: %s", firstLine(res.Text()))
	}

	return nil
}

// GoInstall rebuilds the CLI from source. version "" means "latest".
func GoInstall(ctx context.Context, run Exec, version string) error {
	ctx, cancel := context.WithTimeout(ctx, goInstallTimeout)
	defer cancel()

	if version == "" {
		version = "latest"
	}

	res, err := run(ctx, "go", "install", CommandPath+"@"+version)
	if err != nil {
		return fmt.Errorf("running go install: %w", err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("go install failed: %s", firstLine(res.Text()))
	}

	return nil
}

// ConfirmVersion runs the installed binary and returns the version it reports.
// It is the only check that the update actually took effect.
func ConfirmVersion(ctx context.Context, run Exec, binary string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()

	res, err := run(ctx, binary, "-version")
	if err != nil {
		return "", fmt.Errorf("running the updated binary: %w", err)
	}
	if res.ExitCode != 0 {
		return "", fmt.Errorf("the updated binary did not report its version: %s", firstLine(res.Text()))
	}

	fields := strings.Fields(firstLine(res.Stdout))
	if len(fields) == 0 {
		return "", errors.New("the updated binary printed nothing for -version")
	}

	return fields[len(fields)-1], nil
}
