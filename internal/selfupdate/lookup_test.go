package selfupdate

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestClientLatest(t *testing.T) {
	g := newFakeGitHub("v0.3.0")
	c := newTestClient(t, g)

	tag, err := c.Latest(context.Background())
	if err != nil || tag != "v0.3.0" {
		t.Fatalf("Latest = %q, %v", tag, err)
	}
	if g.latestHeads.Load() != 1 {
		t.Errorf("expected exactly one HEAD, got %d", g.latestHeads.Load())
	}
}

func TestClientLatestNoRelease(t *testing.T) {
	c := newTestClient(t, newFakeGitHub(""))
	if _, err := c.Latest(context.Background()); err == nil {
		t.Fatal("expected an error when there is no latest release")
	}
}

func TestClientTagAndAssetExists(t *testing.T) {
	g := newFakeGitHub("v0.3.0")
	g.addAsset("v0.2.0", "machineid-linux-amd64.zip", []byte("zip"))
	c := newTestClient(t, g)
	ctx := context.Background()

	for tag, want := range map[string]bool{"v0.3.0": true, "v0.2.0": true, "v9.9.9": false} {
		ok, err := c.TagExists(ctx, tag)
		if err != nil || ok != want {
			t.Errorf("TagExists(%s) = %v, %v; want %v", tag, ok, err, want)
		}
	}

	ok, err := c.AssetExists(ctx, "v0.2.0", "machineid-linux-amd64.zip")
	if err != nil || !ok {
		t.Errorf("AssetExists(existing) = %v, %v", ok, err)
	}
	ok, err = c.AssetExists(ctx, "v0.2.0", "machineid-linux-arm64.zip")
	if err != nil || ok {
		t.Errorf("AssetExists(missing) = %v, %v", ok, err)
	}
}

func TestClientDownloadFollowsRedirectAndWrites(t *testing.T) {
	g := newFakeGitHub("v0.3.0")
	g.addAsset("v0.3.0", "machineid-linux-amd64.sha256", []byte("abc\n"))
	c := newTestClient(t, g)

	dest := filepath.Join(t.TempDir(), "sum")
	if err := c.Download(context.Background(), "v0.3.0", "machineid-linux-amd64.sha256", dest); err != nil {
		t.Fatal(err)
	}
	got := mustRead(t, dest)
	if string(got) != "abc\n" {
		t.Errorf("downloaded %q", got)
	}
}

func TestClientDownloadMissingAsset(t *testing.T) {
	c := newTestClient(t, newFakeGitHub("v0.3.0"))
	err := c.Download(context.Background(), "v0.3.0", "nope.zip", filepath.Join(t.TempDir(), "x"))
	if _, ok := errors.AsType[*AssetNotFoundError](err); !ok {
		t.Fatalf("error = %v, want AssetNotFoundError", err)
	}
}

func TestClientDownloadRefusesForeignRedirect(t *testing.T) {
	g := newFakeGitHub("v0.3.0")
	g.addAsset("v0.3.0", "a.zip", []byte("zip"))
	c := newTestClient(t, g)
	c.AllowHost = func(string) bool { return false }

	err := c.Download(context.Background(), "v0.3.0", "a.zip", filepath.Join(t.TempDir(), "x"))
	if err == nil {
		t.Fatal("expected the redirect to be refused")
	}
}

func TestClientRateLimited(t *testing.T) {
	g := newFakeGitHub("v0.3.0")
	g.rateLimit = true
	g.retryAfter = "120"
	c := newTestClient(t, g)

	_, err := c.Latest(context.Background())
	rl, ok := errors.AsType[*RateLimitedError](err)
	if !ok {
		t.Fatalf("error = %v, want RateLimitedError", err)
	}
	if rl.RetryAt.IsZero() || RemedyOf(err) == "" {
		t.Error("RateLimitedError should carry a retry time and a remedy")
	}
}

func TestDefaultAllowHost(t *testing.T) {
	c := &Client{}
	for host, want := range map[string]bool{
		"github.com":                           true,
		"github.com:443":                       true,
		"objects.githubusercontent.com":        true,
		"release-assets.githubusercontent.com": true,
		"evil.example.com":                     false,
		"github.com.evil.example.com":          false,
	} {
		if got := c.allowHost(host); got != want {
			t.Errorf("allowHost(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestTagFromReleasePath(t *testing.T) {
	if tag, ok := tagFromReleasePath("/slashdevops/machineid/releases/tag/v0.3.0"); !ok || tag != "v0.3.0" {
		t.Errorf("got %q, %v", tag, ok)
	}
	if _, ok := tagFromReleasePath("/slashdevops/machineid/releases"); ok {
		t.Error("no tag should not parse")
	}
}
