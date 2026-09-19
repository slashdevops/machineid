package selfupdate

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// execWaitDelay bounds how long a killed command may hold its pipes open.
const execWaitDelay = time.Second

// ExecResult is what an external command produced.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Text returns stdout when there is any, else stderr.
func (r ExecResult) Text() string {
	if strings.TrimSpace(r.Stdout) != "" {
		return r.Stdout
	}

	return r.Stderr
}

// Exec runs an external command. A non-zero exit is reported through
// ExecResult.ExitCode; err is set only when the command could not run at
// all (not found, context cancelled). Tests inject a fake.
type Exec func(ctx context.Context, name string, args ...string) (ExecResult, error)

// DefaultExec runs commands with os/exec. Arguments are passed as a slice;
// nothing goes through a shell.
func DefaultExec(ctx context.Context, name string, args ...string) (ExecResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = execWaitDelay

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	res := ExecResult{Stdout: strings.TrimSpace(stdout.String()), Stderr: strings.TrimSpace(stderr.String())}

	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			res.ExitCode = exitErr.ExitCode()

			return res, nil
		}

		return res, err
	}

	return res, nil
}

// VerifyStatus is the outcome of a signature check.
type VerifyStatus int

const (
	// VerifySkipped means the check could not run (tool absent, no bundle).
	VerifySkipped VerifyStatus = iota
	// VerifyPassed means the signature is valid.
	VerifyPassed
	// VerifyFailed means the check ran and the artefact is not trusted.
	VerifyFailed
)

// Verification is the result of one signature check.
type Verification struct {
	Tool   string
	Detail string
	Status VerifyStatus
}

// Sigstore identity constraints for release artefacts signed by the release
// workflow of this repository.
const (
	sigstoreIssuer        = "https://token.actions.githubusercontent.com"
	sigstoreIdentityRegex = `^https://github\.com/slashdevops/machineid/`
)

// VerifySigstore checks a Linux archive against its Sigstore bundle with
// cosign. Without cosign on PATH, or without a bundle, the check is skipped
// and the caller decides whether that is acceptable.
func VerifySigstore(ctx context.Context, run Exec, lookPath func(string) (string, error), archive, bundle string) Verification {
	v := Verification{Tool: "cosign"}

	if bundle == "" {
		v.Detail = "no Sigstore bundle is published for this asset"

		return v
	}

	if _, err := lookPath("cosign"); err != nil {
		v.Detail = "cosign is not installed"

		return v
	}

	res, err := run(ctx, "cosign", "verify-blob",
		"--bundle", bundle,
		"--certificate-identity-regexp", sigstoreIdentityRegex,
		"--certificate-oidc-issuer", sigstoreIssuer,
		archive)
	if err != nil {
		v.Status = VerifyFailed
		v.Detail = err.Error()

		return v
	}

	if res.ExitCode != 0 {
		v.Status = VerifyFailed
		v.Detail = firstLine(res.Text())

		return v
	}

	v.Status = VerifyPassed
	v.Detail = "Sigstore bundle verified against the release workflow identity"

	return v
}

// VerifyPkg checks a macOS package's Developer ID signature with pkgutil.
func VerifyPkg(ctx context.Context, run Exec, pkgPath string) Verification {
	v := Verification{Tool: "pkgutil"}

	res, err := run(ctx, "pkgutil", "--check-signature", pkgPath)
	if err != nil {
		v.Detail = "pkgutil is not available: " + err.Error()

		return v
	}

	if res.ExitCode != 0 || !strings.Contains(res.Text(), "Developer ID Installer") {
		v.Status = VerifyFailed
		v.Detail = firstLine(res.Text())
		if v.Detail == "" {
			v.Detail = "the package is not signed with a Developer ID Installer certificate"
		}

		return v
	}

	v.Status = VerifyPassed
	v.Detail = signerLine(res.Text())

	return v
}

// VerifyCodesign checks a macOS binary's code signature.
func VerifyCodesign(ctx context.Context, run Exec, binary string) Verification {
	v := Verification{Tool: "codesign"}

	res, err := run(ctx, "codesign", "--verify", "--strict", "--verbose=2", binary)
	if err != nil {
		v.Detail = "codesign is not available: " + err.Error()

		return v
	}

	if res.ExitCode != 0 {
		v.Status = VerifyFailed
		v.Detail = firstLine(res.Text())

		return v
	}

	v.Status = VerifyPassed
	v.Detail = "code signature valid"

	return v
}

// signerLine extracts the "Developer ID Installer: …" line from pkgutil output.
func signerLine(out string) string {
	for line := range strings.SplitSeq(out, "\n") {
		if t := strings.TrimSpace(line); strings.Contains(t, "Developer ID Installer") {
			return strings.TrimPrefix(t, "1. ")
		}
	}

	return "signed by a Developer ID Installer certificate"
}
