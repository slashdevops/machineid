package selfupdate

import (
	"context"
	"errors"
	"testing"
)

func TestVerifySigstore(t *testing.T) {
	ctx := context.Background()
	found := func(string) (string, error) { return "/usr/bin/cosign", nil }
	missing := func(string) (string, error) { return "", errors.New("not found") }

	if v := VerifySigstore(ctx, newFakeExec().run, found, "a.zip", ""); v.Status != VerifySkipped {
		t.Errorf("no bundle: %+v", v)
	}
	if v := VerifySigstore(ctx, newFakeExec().run, missing, "a.zip", "a.json"); v.Status != VerifySkipped {
		t.Errorf("no cosign: %+v", v)
	}

	fe := newFakeExec()
	fe.on("cosign verify-blob --bundle a.json --certificate-identity-regexp "+sigstoreIdentityRegex+" --certificate-oidc-issuer "+sigstoreIssuer+" a.zip", ExecResult{Stdout: "Verified OK"})
	if v := VerifySigstore(ctx, fe.run, found, "a.zip", "a.json"); v.Status != VerifyPassed {
		t.Errorf("valid: %+v", v)
	}

	fe.on("cosign verify-blob --bundle a.json --certificate-identity-regexp "+sigstoreIdentityRegex+" --certificate-oidc-issuer "+sigstoreIssuer+" a.zip", ExecResult{ExitCode: 1, Stderr: "Error: none of the expected identities matched"})
	if v := VerifySigstore(ctx, fe.run, found, "a.zip", "a.json"); v.Status != VerifyFailed || v.Detail == "" {
		t.Errorf("invalid: %+v", v)
	}
}

func TestVerifyPkg(t *testing.T) {
	ctx := context.Background()
	fe := newFakeExec()
	fe.on("pkgutil --check-signature x.pkg", ExecResult{Stdout: "Package \"x.pkg\":\n   Status: signed by a certificate trusted by macOS\n   Certificate Chain:\n    1. Developer ID Installer: SlashDevOps (TEAM)\n"})
	v := VerifyPkg(ctx, fe.run, "x.pkg")
	if v.Status != VerifyPassed || v.Detail != "Developer ID Installer: SlashDevOps (TEAM)" {
		t.Errorf("%+v", v)
	}

	fe.on("pkgutil --check-signature x.pkg", ExecResult{Stdout: "Status: no signature"})
	if v := VerifyPkg(ctx, fe.run, "x.pkg"); v.Status != VerifyFailed {
		t.Errorf("unsigned: %+v", v)
	}

	fe.fail("pkgutil", errors.New("not found"))
	if v := VerifyPkg(ctx, fe.run, "x.pkg"); v.Status != VerifySkipped {
		t.Errorf("missing tool: %+v", v)
	}
}

func TestVerifyCodesign(t *testing.T) {
	ctx := context.Background()
	fe := newFakeExec()
	fe.on("codesign --verify --strict --verbose=2 bin", ExecResult{})
	if v := VerifyCodesign(ctx, fe.run, "bin"); v.Status != VerifyPassed {
		t.Errorf("%+v", v)
	}
	fe.on("codesign --verify --strict --verbose=2 bin", ExecResult{ExitCode: 1, Stderr: "bin: code object is not signed at all"})
	if v := VerifyCodesign(ctx, fe.run, "bin"); v.Status != VerifyFailed {
		t.Errorf("%+v", v)
	}
}
