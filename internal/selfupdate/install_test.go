package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReplaceBinaryAtomic(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "machineid")
	source := filepath.Join(dir, "new")
	_ = os.WriteFile(target, []byte("old"), 0o755)
	_ = os.WriteFile(source, []byte("new"), 0o600)

	if err := ReplaceBinary(source, target); err != nil {
		t.Fatal(err)
	}

	got := mustRead(t, target)
	if string(got) != "new" {
		t.Errorf("target = %q", got)
	}
	if info := mustStat(t, target); runtime.GOOS != "windows" && info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %o", info.Mode().Perm())
	}

	entries := mustReadDir(t, dir)
	for _, e := range entries {
		if e.Name() != "machineid" && e.Name() != "new" && e.Name() != "machineid.old" {
			t.Errorf("staging file left behind: %s", e.Name())
		}
	}
}

func TestReplaceBinaryFailureLeavesTargetUntouched(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "machineid")
	_ = os.WriteFile(target, []byte("old"), 0o755)

	err := ReplaceBinary(filepath.Join(dir, "does-not-exist"), target)
	if err == nil {
		t.Fatal("expected an error")
	}

	got := mustRead(t, target)
	if string(got) != "old" {
		t.Errorf("target changed to %q", got)
	}
	entries := mustReadDir(t, dir)
	if len(entries) != 1 {
		t.Errorf("expected only the target to remain, found %d entries", len(entries))
	}
}

func TestCleanStaleBackup(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "machineid")
	_ = os.WriteFile(target+backupSuffix, []byte("x"), 0o600)
	CleanStaleBackup(target)
	if _, err := os.Stat(target + backupSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Error("backup should be removed")
	}
	CleanStaleBackup(target) // idempotent
}

func TestInstallPkg(t *testing.T) {
	fe := newFakeExec()
	fe.on("installer -pkg /tmp/x.pkg -target /", ExecResult{})
	if err := InstallPkg(context.Background(), fe.run, "/tmp/x.pkg"); err != nil {
		t.Fatal(err)
	}

	fe.on("installer -pkg /tmp/x.pkg -target /", ExecResult{ExitCode: 1, Stderr: "installer: The install failed"})
	if err := InstallPkg(context.Background(), fe.run, "/tmp/x.pkg"); err == nil {
		t.Fatal("expected failure")
	}
}

func TestGoInstall(t *testing.T) {
	fe := newFakeExec()
	fe.on("go install "+CommandPath+"@v0.3.0", ExecResult{})
	fe.on("go install "+CommandPath+"@latest", ExecResult{})

	if err := GoInstall(context.Background(), fe.run, "v0.3.0"); err != nil {
		t.Fatal(err)
	}
	if err := GoInstall(context.Background(), fe.run, ""); err != nil {
		t.Fatal(err)
	}
	if !fe.called("go install " + CommandPath + "@latest") {
		t.Error("empty version should install @latest")
	}
}

func TestConfirmVersion(t *testing.T) {
	fe := newFakeExec()
	fe.on("/usr/local/bin/machineid -version", ExecResult{Stdout: "machineid v0.3.0\n"})

	v, err := ConfirmVersion(context.Background(), fe.run, "/usr/local/bin/machineid")
	if err != nil || v != "v0.3.0" {
		t.Errorf("ConfirmVersion = %q, %v", v, err)
	}

	fe.on("/bad -version", ExecResult{ExitCode: 1, Stderr: "boom"})
	if _, err := ConfirmVersion(context.Background(), fe.run, "/bad"); err == nil {
		t.Error("expected error")
	}
}

func TestDefaultExec(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	res, err := DefaultExec(context.Background(), "sh", "-c", "echo out; echo err >&2; exit 3")
	if err != nil {
		t.Fatal(err)
	}
	if res.Stdout != "out" || res.Stderr != "err" || res.ExitCode != 3 {
		t.Errorf("res = %+v", res)
	}
	if _, err := DefaultExec(context.Background(), "definitely-not-a-command-xyz"); err == nil {
		t.Error("a missing command should be an error, not an exit code")
	}
}
