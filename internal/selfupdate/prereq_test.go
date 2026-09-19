package selfupdate

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

// newTestChecker returns a checker for a fake machine. exe is the running
// binary; gobin is what `go env` reports; uid 0 means root.
func newTestChecker(goos, exe, gobin string, uid int, fe *fakeExec) *Checker {
	fe.on("go version", ExecResult{Stdout: "go version go1.27.1 " + goos + "/arm64"})
	fe.on("go env GOBIN GOPATH", ExecResult{Stdout: gobin + "\n" + filepath.Dir(gobin) + "\n"})

	return &Checker{
		Exec:           fe.run,
		Executable:     func() (string, error) { return exe, nil },
		LookPath:       func(string) (string, error) { return "", errors.New("not found") },
		WritableProbe:  func(string) error { return nil },
		Getuid:         func() int { return uid },
		GOOS:           goos,
		GOARCH:         "arm64",
		CurrentVersion: "v0.2.0",
	}
}

func TestCheckerRunLinuxAuto(t *testing.T) {
	fe := newFakeExec()
	c := newTestChecker("linux", "/usr/local/bin/machineid", "/home/u/go/bin", 1000, fe)

	r := c.Run(context.Background(), MethodAuto)
	if err := r.FirstEssentialError(); err != nil {
		t.Fatal(err)
	}
	if r.ReleaseErr != nil || !r.HasGo || r.GoBinDir != "/home/u/go/bin" || r.IsRoot || r.HasCosign {
		t.Errorf("report = %+v", r)
	}

	m, err := c.Resolve(MethodAuto, r)
	if err != nil || m != MethodRelease {
		t.Errorf("Resolve(auto) = %s, %v", m, err)
	}
}

func TestCheckerLocalBuildIsNotEssential(t *testing.T) {
	fe := newFakeExec()
	c := newTestChecker("linux", "/tmp/machineid", "/home/u/go/bin", 1000, fe)
	c.CurrentVersion = "devel"

	r := c.Run(context.Background(), MethodAuto)
	if err := r.FirstEssentialError(); err != nil {
		t.Fatalf("a local build must not stop the run: %v", err)
	}
}

func TestCheckerExecutableFailureIsEssential(t *testing.T) {
	c := newTestChecker("linux", "", "/home/u/go/bin", 1000, newFakeExec())
	c.Executable = func() (string, error) { return "", errors.New("no exe") }

	r := c.Run(context.Background(), MethodAuto)
	if r.FirstEssentialError() == nil {
		t.Fatal("expected an essential failure")
	}
}

func TestCheckerResolveFallsBackToGo(t *testing.T) {
	fe := newFakeExec()
	c := newTestChecker("windows", `C:\Users\u\go\bin\machineid.exe`, `C:\Users\u\go\bin`, 1000, fe)

	r := c.Run(context.Background(), MethodAuto)
	if err := r.FirstEssentialError(); err != nil {
		t.Fatalf("unsupported release platform must not be essential under auto: %v", err)
	}
	m, err := c.Resolve(MethodAuto, r)
	if err != nil || m != MethodGo {
		t.Errorf("Resolve(auto on windows) = %s, %v", m, err)
	}

	if _, err := c.Resolve(MethodRelease, r); err == nil {
		t.Error("explicit release on windows should fail")
	}
}

func TestCheckerResolveNothingAvailable(t *testing.T) {
	fe := newFakeExec()
	fe.fail("go version", errors.New("not found"))
	c := newTestChecker("windows", `C:\machineid.exe`, `C:\go\bin`, 1000, fe)
	c.Exec = fe.run

	r := c.Run(context.Background(), MethodAuto)
	if _, err := c.Resolve(MethodAuto, r); err == nil {
		t.Fatal("expected an error when neither method works")
	}
	if _, err := c.Resolve(MethodGo, r); err == nil || RemedyOf(err) == "" {
		t.Errorf("explicit go without a toolchain should carry a remedy: %v", err)
	}
}

func TestCheckInstallTargetPkg(t *testing.T) {
	fe := newFakeExec()
	asset := mustAsset(t, "darwin", "arm64")

	// In place, root: ok.
	c := newTestChecker("darwin", "/usr/local/bin/machineid", "/Users/u/go/bin", 0, fe)
	r := c.Run(context.Background(), MethodRelease)
	if chk := c.CheckInstallTarget(MethodRelease, asset, r, false); !chk.OK() {
		t.Errorf("root in /usr/local/bin: %v", chk.Err)
	}

	// In place, not root: needs root with a sudo remedy.
	c = newTestChecker("darwin", "/usr/local/bin/machineid", "/Users/u/go/bin", 501, fe)
	r = c.Run(context.Background(), MethodRelease)
	chk := c.CheckInstallTarget(MethodRelease, asset, r, false)
	var needsRoot *NeedsRootError
	if !errors.As(chk.Err, &needsRoot) || RemedyOf(chk.Err) == "" {
		t.Errorf("non-root: %v", chk.Err)
	}

	// Elsewhere (a go install copy): wrong location, remedy names -method go.
	c = newTestChecker("darwin", "/Users/u/go/bin/machineid", "/Users/u/go/bin", 0, fe)
	r = c.Run(context.Background(), MethodAuto)
	chk = c.CheckInstallTarget(MethodRelease, asset, r, false)
	var wrong *WrongInstallLocationError
	if !errors.As(chk.Err, &wrong) || wrong.Alternative != "-method go" {
		t.Errorf("go-installed copy: %v", chk.Err)
	}

	// Elsewhere with -force: proceeds (root).
	if chk := c.CheckInstallTarget(MethodRelease, asset, r, true); !chk.OK() {
		t.Errorf("force: %v", chk.Err)
	}
}

func TestCheckInstallTargetGo(t *testing.T) {
	fe := newFakeExec()
	c := newTestChecker("linux", "/usr/local/bin/machineid", "/home/u/go/bin", 1000, fe)
	r := c.Run(context.Background(), MethodGo)

	chk := c.CheckInstallTarget(MethodGo, Asset{}, r, false)
	var wrong *WrongInstallLocationError
	if !errors.As(chk.Err, &wrong) || wrong.Alternative != "-method release" {
		t.Errorf("go method for a /usr/local/bin copy: %v", chk.Err)
	}

	c = newTestChecker("linux", "/home/u/go/bin/machineid", "/home/u/go/bin", 1000, fe)
	r = c.Run(context.Background(), MethodGo)
	if chk := c.CheckInstallTarget(MethodGo, Asset{}, r, false); !chk.OK() {
		t.Errorf("go method in GOBIN: %v", chk.Err)
	}

	c.WritableProbe = func(string) error { return errors.New("read-only") }
	r = c.Run(context.Background(), MethodGo)
	chk = c.CheckInstallTarget(MethodGo, Asset{}, r, false)
	if _, ok := errors.AsType[*NotWritableError](chk.Err); !ok {
		t.Errorf("unwritable GOBIN: %v", chk.Err)
	}
}

func TestCheckInstallTargetZipInPlace(t *testing.T) {
	fe := newFakeExec()
	asset := mustAsset(t, "linux", "amd64")
	c := newTestChecker("linux", "/opt/tools/machineid", "/home/u/go/bin", 1000, fe)
	r := c.Run(context.Background(), MethodRelease)

	if chk := c.CheckInstallTarget(MethodRelease, asset, r, false); !chk.OK() {
		t.Errorf("writable dir: %v", chk.Err)
	}

	c.WritableProbe = func(dir string) error { return errors.New("permission denied") }
	chk := c.CheckInstallTarget(MethodRelease, asset, r, false)
	var nw *NotWritableError
	if !errors.As(chk.Err, &nw) || RemedyOf(chk.Err) == "" {
		t.Errorf("unwritable dir: %v", chk.Err)
	}
}

func TestGoBinDirPrefersGOBIN(t *testing.T) {
	fe := newFakeExec()
	fe.on("go env GOBIN GOPATH", ExecResult{Stdout: "/custom/bin\n/home/u/go\n"})
	c := &Checker{Exec: fe.run}
	dir, err := c.goBinDir(context.Background())
	if err != nil || dir != "/custom/bin" {
		t.Errorf("got %q, %v", dir, err)
	}
}

func TestGoBinDirFallsBackToGOPATH(t *testing.T) {
	fe := newFakeExec()
	fe.on("go env GOBIN GOPATH", ExecResult{Stdout: "\n/home/u/go\n"})
	c := &Checker{Exec: fe.run}
	dir, err := c.goBinDir(context.Background())
	if err != nil || dir != filepath.Join("/home/u/go", "bin") {
		t.Errorf("got %q, %v", dir, err)
	}

	fe.on("go env GOBIN GOPATH", ExecResult{Stdout: "\n\n"})
	if _, err := c.goBinDir(context.Background()); err == nil {
		t.Error("empty GOPATH should be an error")
	}
}
