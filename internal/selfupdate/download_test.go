package selfupdate

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFetchVerifiesChecksum(t *testing.T) {
	g := newFakeGitHub("v0.3.0")
	g.addArchive("v0.3.0", "machineid-linux-amd64.zip", "machineid-linux-amd64.sha256", "machineid", []byte("binary"))
	c := newTestClient(t, g)
	asset := mustAsset(t, "linux", "amd64")

	archive, bundle, err := Fetch(context.Background(), c, "v0.3.0", asset, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(archive) != asset.Name {
		t.Errorf("archive = %s", archive)
	}
	if bundle != "" {
		t.Errorf("no bundle was published, got %q", bundle)
	}
}

func TestFetchDownloadsBundleWhenPublished(t *testing.T) {
	g := newFakeGitHub("v0.3.0")
	g.addArchive("v0.3.0", "machineid-linux-amd64.zip", "machineid-linux-amd64.sha256", "machineid", []byte("binary"))
	g.addAsset("v0.3.0", "machineid-linux-amd64.sigstore.json", []byte("{}"))
	c := newTestClient(t, g)
	asset := mustAsset(t, "linux", "amd64")

	_, bundle, err := Fetch(context.Background(), c, "v0.3.0", asset, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(bundle) != asset.BundleName {
		t.Errorf("bundle = %q", bundle)
	}
}

func TestFetchChecksumMismatch(t *testing.T) {
	g := newFakeGitHub("v0.3.0")
	g.addAsset("v0.3.0", "machineid-linux-amd64.zip", makeZip("machineid", []byte("binary")))
	g.addAsset("v0.3.0", "machineid-linux-amd64.sha256", []byte(strings.Repeat("0", 64)))
	c := newTestClient(t, g)
	asset := mustAsset(t, "linux", "amd64")

	_, _, err := Fetch(context.Background(), c, "v0.3.0", asset, t.TempDir())
	if _, ok := errors.AsType[*ChecksumMismatchError](err); !ok {
		t.Fatalf("error = %v, want ChecksumMismatchError", err)
	}
	if RemedyOf(err) == "" {
		t.Error("expected a remedy")
	}
}

func TestReadChecksumFileForms(t *testing.T) {
	dir := t.TempDir()
	sum := strings.Repeat("ab", 32)

	for name, content := range map[string]string{
		"bare":   sum + "\n",
		"shasum": sum + "  machineid-linux-amd64.zip\n",
	} {
		p := filepath.Join(dir, name)
		_ = os.WriteFile(p, []byte(content), 0o600)
		got, err := readChecksumFile(p)
		if err != nil || got != sum {
			t.Errorf("%s: got %q, %v", name, got, err)
		}
	}

	for name, content := range map[string]string{
		"empty":  "",
		"short":  "abcd\n",
		"nonhex": strings.Repeat("zz", 32),
	} {
		p := filepath.Join(dir, name)
		_ = os.WriteFile(p, []byte(content), 0o600)
		if _, err := readChecksumFile(p); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestExtractBinary(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.zip")
	_ = os.WriteFile(archive, makeZip("machineid", []byte("hello")), 0o600)

	out, err := ExtractBinary(archive, "machineid", dir)
	if err != nil {
		t.Fatal(err)
	}
	got := mustRead(t, out)
	if string(got) != "hello" {
		t.Errorf("extracted %q", got)
	}
	info := mustStat(t, out)
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0 {
		t.Error("extracted binary should be executable")
	}
}

func TestExtractBinaryFallsBackToSingleFile(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.zip")
	_ = os.WriteFile(archive, makeZip("machineid-linux-amd64", []byte("x")), 0o600)

	out, err := ExtractBinary(archive, "machineid", dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(out) != "machineid-linux-amd64" {
		t.Errorf("out = %s", out)
	}
}

func TestExtractBinaryAmbiguous(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range []string{"a", "b"} {
		f, err := zw.Create(n)
		mustOK(t, err)
		_, err = f.Write([]byte(n))
		mustOK(t, err)
	}
	mustOK(t, zw.Close())
	archive := filepath.Join(dir, "a.zip")
	_ = os.WriteFile(archive, buf.Bytes(), 0o600)

	if _, err := ExtractBinary(archive, "machineid", dir); err == nil {
		t.Fatal("expected ambiguity error")
	}
}

func TestExtractBinaryZipSlip(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "dest")
	mustOK(t, os.Mkdir(dest, 0o755))

	archive := filepath.Join(dir, "evil.zip")
	_ = os.WriteFile(archive, makeZip("../../escaped", []byte("x")), 0o600)

	out, err := ExtractBinary(archive, "machineid", dest)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(out) != dest {
		t.Errorf("entry escaped the destination: %s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "escaped")); err == nil {
		t.Error("a file was written outside the destination")
	}
}
