package selfupdate

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testRig is a fully faked machine + GitHub for end-to-end runs.
type testRig struct {
	gh     *fakeGitHub
	exec   *fakeExec
	budget *Budget
	now    *time.Time
	out    bytes.Buffer
	target string // the running binary on disk
	u      *Updater
}

func newRig(t *testing.T, goos, latest string) *testRig {
	t.Helper()

	dir := t.TempDir()
	target := filepath.Join(dir, "machineid")
	if err := os.WriteFile(target, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	rig := &testRig{gh: newFakeGitHub(latest), exec: newFakeExec(), target: target}
	rig.budget, rig.now = newTestBudget(t, DefaultLimit)

	checker := newTestChecker(goos, target, filepath.Join(dir, "gobin"), 1000, rig.exec)
	rig.exec.on(target+" -version", ExecResult{Stdout: "machineid " + latest})

	rig.u = &Updater{
		Checker:  checker,
		Client:   newTestClient(t, rig.gh),
		Budget:   rig.budget,
		Exec:     rig.exec.run,
		LookPath: func(string) (string, error) { return "", errors.New("not found") },
		Out:      &rig.out,
		TempDir:  func() (string, error) { return os.MkdirTemp(dir, "stage-*") },
	}

	return rig
}

func (r *testRig) publishLinux(tag string, content []byte) {
	r.gh.addArchive(tag, "machineid-linux-arm64.zip", "machineid-linux-arm64.sha256", "machineid", content)
}

func TestUpdaterLinuxZipEndToEnd(t *testing.T) {
	rig := newRig(t, "linux", "v0.3.0")
	rig.publishLinux("v0.3.0", []byte("new-binary"))

	res, err := rig.u.Run(context.Background(), Options{CurrentVersion: "v0.2.0", AssumeYes: true})
	if err != nil {
		t.Fatalf("Run: %v\n%s", err, rig.out.String())
	}
	if !res.Changed || res.Method != MethodRelease || res.Tag != "v0.3.0" || res.To != "v0.3.0" {
		t.Errorf("result = %+v", res)
	}

	got := mustRead(t, rig.target)
	if string(got) != "new-binary" {
		t.Errorf("target = %q", got)
	}

	out := rig.out.String()
	for _, want := range []string{"✓ SHA-256 verified", "! signature not verified: no Sigstore bundle is published", "live, 4 of 5 checks left", "✓ installed to " + rig.target} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestUpdaterAlreadyCurrent(t *testing.T) {
	rig := newRig(t, "linux", "v0.2.0")
	rig.publishLinux("v0.2.0", []byte("same"))

	res, err := rig.u.Run(context.Background(), Options{CurrentVersion: "v0.2.0", AssumeYes: true})
	if err != nil || res.Changed {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if !strings.Contains(rig.out.String(), "already the latest version") {
		t.Error(rig.out.String())
	}
	if got := mustRead(t, rig.target); string(got) != "old-binary" {
		t.Error("target must not change")
	}
}

func TestUpdaterCheckChangesNothing(t *testing.T) {
	rig := newRig(t, "linux", "v0.3.0")
	rig.publishLinux("v0.3.0", []byte("new"))

	res, err := rig.u.Run(context.Background(), Options{CurrentVersion: "v0.2.0", Check: true})
	if err != nil || res.Changed {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if got := mustRead(t, rig.target); string(got) != "old-binary" {
		t.Error("-check must not change the target")
	}
	if !strings.Contains(rig.out.String(), "-check: nothing was changed") {
		t.Error(rig.out.String())
	}
}

func TestUpdaterDeclinedPrompt(t *testing.T) {
	rig := newRig(t, "linux", "v0.3.0")
	rig.publishLinux("v0.3.0", []byte("new"))
	asked := false
	rig.u.Confirm = func(string) (bool, error) { asked = true; return false, nil }

	res, err := rig.u.Run(context.Background(), Options{CurrentVersion: "v0.2.0"})
	if err != nil || res.Changed || !asked {
		t.Fatalf("res = %+v, err = %v, asked = %v", res, err, asked)
	}
	if got := mustRead(t, rig.target); string(got) != "old-binary" {
		t.Error("declining must not change the target")
	}
}

func TestUpdaterUsesCacheThenBudget(t *testing.T) {
	rig := newRig(t, "linux", "v0.3.0")
	rig.publishLinux("v0.3.0", []byte("new"))
	ctx := context.Background()

	// First -check: live.
	if _, err := rig.u.Run(ctx, Options{CurrentVersion: "v0.2.0", Check: true}); err != nil {
		t.Fatal(err)
	}
	if rig.gh.latestHeads.Load() != 1 {
		t.Fatalf("expected 1 live lookup, got %d", rig.gh.latestHeads.Load())
	}

	// Second -check within the hour: cached, no request.
	rig.out.Reset()
	*rig.now = rig.now.Add(10 * time.Minute)
	if _, err := rig.u.Run(ctx, Options{CurrentVersion: "v0.2.0", Check: true}); err != nil {
		t.Fatal(err)
	}
	if rig.gh.latestHeads.Load() != 1 {
		t.Errorf("cached run made a request")
	}
	if !strings.Contains(rig.out.String(), "cached 10m0s ago") {
		t.Error(rig.out.String())
	}

	// -refresh forces live lookups until the budget is spent.
	for range DefaultLimit - 1 {
		if _, err := rig.u.Run(ctx, Options{CurrentVersion: "v0.2.0", Check: true, Refresh: true}); err != nil {
			t.Fatal(err)
		}
	}
	if int(rig.gh.latestHeads.Load()) != DefaultLimit {
		t.Errorf("expected %d live lookups, got %d", DefaultLimit, rig.gh.latestHeads.Load())
	}

	// Budget spent: -check still answers from the stale cache…
	rig.out.Reset()
	if _, err := rig.u.Run(ctx, Options{CurrentVersion: "v0.2.0", Check: true, Refresh: true}); err != nil {
		t.Fatalf("-check over budget should serve the cache: %v", err)
	}
	if !strings.Contains(rig.out.String(), "next live check at") {
		t.Error(rig.out.String())
	}

	// …but a real update with -refresh refuses with a prerequisite error.
	_, err := rig.u.Run(ctx, Options{CurrentVersion: "v0.2.0", Refresh: true, AssumeYes: true})
	var exhausted *BudgetExhaustedError
	if !errors.As(err, &exhausted) || !IsPrerequisite(err) {
		t.Fatalf("err = %v", err)
	}
	if int(rig.gh.latestHeads.Load()) != DefaultLimit {
		t.Error("a refused lookup must not hit the network")
	}
}

func TestUpdaterExplicitVersionDowngrades(t *testing.T) {
	rig := newRig(t, "linux", "v0.3.0")
	rig.publishLinux("v0.1.0", []byte("older"))
	rig.exec.on(rig.target+" -version", ExecResult{Stdout: "machineid v0.1.0"})

	res, err := rig.u.Run(context.Background(), Options{CurrentVersion: "v0.2.0", Version: "v0.1.0", AssumeYes: true})
	if err != nil {
		t.Fatalf("%v\n%s", err, rig.out.String())
	}
	if !res.Changed || res.Tag != "v0.1.0" {
		t.Errorf("res = %+v", res)
	}
	if got := mustRead(t, rig.target); string(got) != "older" {
		t.Errorf("target = %q", got)
	}
}

func TestUpdaterExplicitVersionMissing(t *testing.T) {
	rig := newRig(t, "linux", "v0.3.0")
	_, err := rig.u.Run(context.Background(), Options{CurrentVersion: "v0.2.0", Version: "v9.9.9", AssumeYes: true})
	var nf *ReleaseNotFoundError
	if !errors.As(err, &nf) || !IsPrerequisite(err) {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdaterLocalBuildNeedsExplicitVersion(t *testing.T) {
	rig := newRig(t, "linux", "v0.3.0")
	rig.publishLinux("v0.3.0", []byte("new"))

	_, err := rig.u.Run(context.Background(), Options{CurrentVersion: "devel", AssumeYes: true})
	var nc *NotComparableError
	if !errors.As(err, &nc) || !IsPrerequisite(err) {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdaterChecksumMismatchIsNotPrerequisite(t *testing.T) {
	rig := newRig(t, "linux", "v0.3.0")
	rig.gh.addAsset("v0.3.0", "machineid-linux-arm64.zip", makeZip("machineid", []byte("new")))
	rig.gh.addAsset("v0.3.0", "machineid-linux-arm64.sha256", []byte(strings.Repeat("0", 64)))

	_, err := rig.u.Run(context.Background(), Options{CurrentVersion: "v0.2.0", AssumeYes: true})
	if _, ok := errors.AsType[*ChecksumMismatchError](err); !ok {
		t.Fatalf("err = %v", err)
	}
	if IsPrerequisite(err) {
		t.Error("a checksum failure happens after the download and is not a prerequisite failure")
	}
	if got := mustRead(t, rig.target); string(got) != "old-binary" {
		t.Error("target must be untouched")
	}
}

func TestUpdaterRequireSignatureWithoutCosign(t *testing.T) {
	rig := newRig(t, "linux", "v0.3.0")
	rig.publishLinux("v0.3.0", []byte("new"))

	_, err := rig.u.Run(context.Background(), Options{CurrentVersion: "v0.2.0", AssumeYes: true, RequireSignature: true})
	if _, ok := errors.AsType[*SignatureError](err); !ok {
		t.Fatalf("err = %v", err)
	}
	if got := mustRead(t, rig.target); string(got) != "old-binary" {
		t.Error("target must be untouched")
	}
}

func TestUpdaterSigstoreVerifiedWhenCosignPresent(t *testing.T) {
	rig := newRig(t, "linux", "v0.3.0")
	rig.publishLinux("v0.3.0", []byte("new"))
	rig.gh.addAsset("v0.3.0", "machineid-linux-arm64.sigstore.json", []byte("{}"))
	rig.u.LookPath = func(string) (string, error) { return "/usr/bin/cosign", nil }
	rig.exec.on("cosign verify-blob", ExecResult{Stdout: "Verified OK"})

	res, err := rig.u.Run(context.Background(), Options{CurrentVersion: "v0.2.0", AssumeYes: true, RequireSignature: true})
	if err != nil || !res.Changed {
		t.Fatalf("res = %+v, err = %v\n%s", res, err, rig.out.String())
	}
	if !rig.exec.called("cosign verify-blob") || !strings.Contains(rig.out.String(), "✓ cosign") {
		t.Error(rig.out.String())
	}
}

func TestUpdaterDarwinPkgPath(t *testing.T) {
	rig := newRig(t, "darwin", "v0.3.0")
	// The running binary is the package's fixed location, as root.
	rig.u.Checker.Executable = func() (string, error) { return "/usr/local/bin/machineid", nil }
	rig.u.Checker.Getuid = func() int { return 0 }
	pkg := []byte("pkg-bytes")
	rig.gh.addAsset("v0.3.0", "machineid-darwin-universal.pkg", pkg)
	rig.gh.addAsset("v0.3.0", "machineid-darwin-universal.sha256", []byte(sha256Hex(pkg)))
	rig.exec.on("pkgutil --check-signature", ExecResult{Stdout: "1. Developer ID Installer: SlashDevOps"})
	rig.exec.on("installer -pkg", ExecResult{})
	rig.exec.on("/usr/local/bin/machineid -version", ExecResult{Stdout: "machineid v0.3.0"})

	res, err := rig.u.Run(context.Background(), Options{CurrentVersion: "v0.2.0", AssumeYes: true})
	if err != nil {
		t.Fatalf("%v\n%s", err, rig.out.String())
	}
	if !res.Changed || res.To != "v0.3.0" {
		t.Errorf("res = %+v", res)
	}
	if !rig.exec.called("installer -pkg") || !strings.Contains(rig.out.String(), "signed macOS package") {
		t.Error(rig.out.String())
	}
}

func TestUpdaterDarwinZipWhenNotInUsrLocalBin(t *testing.T) {
	rig := newRig(t, "darwin", "v0.3.0")
	// Running from a temp dir; the release carries the universal zip.
	z := DarwinZipAsset()
	rig.gh.addArchive("v0.3.0", z.Name, z.ChecksumName, z.InnerName, []byte("new-mac-binary"))
	rig.exec.on("codesign --verify", ExecResult{})

	res, err := rig.u.Run(context.Background(), Options{CurrentVersion: "v0.2.0", AssumeYes: true})
	if err != nil {
		t.Fatalf("%v\n%s", err, rig.out.String())
	}
	if !res.Changed {
		t.Errorf("res = %+v", res)
	}
	if got := mustRead(t, rig.target); string(got) != "new-mac-binary" {
		t.Errorf("target = %q", got)
	}
	if rig.exec.called("installer -pkg") {
		t.Error("the package must not be used for a binary outside /usr/local/bin")
	}
}

// TestUpdaterDarwinForceReinstallsInPlace covers `update -force -version <same>`
// from a go-installed copy: the universal zip must be used in place, without
// root and without the installer. Found by the first real run against v0.2.0.
func TestUpdaterDarwinForceReinstallsInPlace(t *testing.T) {
	rig := newRig(t, "darwin", "v0.2.0")
	z := DarwinZipAsset()
	rig.gh.addArchive("v0.2.0", z.Name, z.ChecksumName, z.InnerName, []byte("same-version-fresh-copy"))
	rig.gh.addAsset("v0.2.0", "machineid-darwin-universal.pkg", []byte("pkg"))
	rig.exec.on("codesign --verify", ExecResult{})
	rig.exec.on(rig.target+" -version", ExecResult{Stdout: "machineid v0.2.0"})

	res, err := rig.u.Run(context.Background(), Options{CurrentVersion: "v0.2.0", Version: "v0.2.0", Force: true, AssumeYes: true})
	if err != nil {
		t.Fatalf("%v\n%s", err, rig.out.String())
	}
	if !res.Changed {
		t.Errorf("-force should reinstall an equal version: %+v", res)
	}
	if got := mustRead(t, rig.target); string(got) != "same-version-fresh-copy" {
		t.Errorf("target = %q", got)
	}
	if rig.exec.called("installer -pkg") {
		t.Error("the package must not be used when the zip exists, even with -force")
	}
}

func TestUpdaterDarwinNoZipRefusesOutsidePkgDir(t *testing.T) {
	rig := newRig(t, "darwin", "v0.3.0")
	rig.gh.addAsset("v0.3.0", "machineid-darwin-universal.pkg", []byte("pkg"))

	_, err := rig.u.Run(context.Background(), Options{CurrentVersion: "v0.2.0", AssumeYes: true})
	var wrong *WrongInstallLocationError
	if !errors.As(err, &wrong) || !IsPrerequisite(err) {
		t.Fatalf("err = %v\n%s", err, rig.out.String())
	}
}

func TestUpdaterGoMethod(t *testing.T) {
	rig := newRig(t, "linux", "v0.3.0")
	gobin := filepath.Dir(rig.target)
	rig.exec.on("go env GOBIN GOPATH", ExecResult{Stdout: gobin + "\n/x\n"})
	rig.exec.on("go install "+CommandPath+"@latest", ExecResult{})

	res, err := rig.u.Run(context.Background(), Options{CurrentVersion: "v0.2.0", Method: MethodGo, AssumeYes: true})
	if err != nil {
		t.Fatalf("%v\n%s", err, rig.out.String())
	}
	if !res.Changed || res.Method != MethodGo || !rig.exec.called("go install "+CommandPath+"@latest") {
		t.Errorf("res = %+v\n%s", res, rig.out.String())
	}
	if rig.gh.latestHeads.Load() != 1 {
		t.Error("the go method still resolves the tag to compare versions")
	}
}

func TestUpdaterRateLimitPersistsBackoff(t *testing.T) {
	rig := newRig(t, "linux", "v0.3.0")
	rig.gh.rateLimit = true
	rig.gh.retryAfter = "600"

	_, err := rig.u.Run(context.Background(), Options{CurrentVersion: "v0.2.0", AssumeYes: true})
	if _, ok := errors.AsType[*RateLimitedError](err); !ok {
		t.Fatalf("err = %v", err)
	}

	rig.gh.rateLimit = false
	_, err = rig.u.Run(context.Background(), Options{CurrentVersion: "v0.2.0", AssumeYes: true})
	if _, ok := errors.AsType[*BudgetExhaustedError](err); !ok {
		t.Fatalf("the backoff should be honoured on the next run: %v", err)
	}
}

func TestUpdaterAssetMissingFromRelease(t *testing.T) {
	rig := newRig(t, "linux", "v0.3.0") // release exists, no linux asset
	_, err := rig.u.Run(context.Background(), Options{CurrentVersion: "v0.2.0", AssumeYes: true})
	var nf *AssetNotFoundError
	if !errors.As(err, &nf) || !IsPrerequisite(err) {
		t.Fatalf("err = %v", err)
	}
}
