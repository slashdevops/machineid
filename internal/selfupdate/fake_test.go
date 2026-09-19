package selfupdate

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// fakeGitHub serves the three endpoints the client uses, in memory.
type fakeGitHub struct {
	mu          sync.Mutex
	latest      string
	assets      map[string]map[string][]byte // tag -> name -> bytes
	latestHeads atomic.Int32
	rateLimit   bool
	retryAfter  string
}

func newFakeGitHub(latest string) *fakeGitHub {
	return &fakeGitHub{latest: latest, assets: map[string]map[string][]byte{}}
}

func (g *fakeGitHub) addAsset(tag, name string, content []byte) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.assets[tag] == nil {
		g.assets[tag] = map[string][]byte{}
	}
	g.assets[tag][name] = content
}

// addArchive publishes a zip holding one file plus its bare .sha256.
func (g *fakeGitHub) addArchive(tag, name, sumName, inner string, content []byte) {
	archive := makeZip(inner, content)
	g.addAsset(tag, name, archive)
	g.addAsset(tag, sumName, []byte(sha256Hex(archive)+"\n"))
}

func (g *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()

	const repo = "/slashdevops/machineid"

	if g.rateLimit {
		if g.retryAfter != "" {
			w.Header().Set("Retry-After", g.retryAfter)
		}
		w.WriteHeader(http.StatusTooManyRequests)

		return
	}

	p := r.URL.Path
	switch {
	case p == repo+"/releases/latest":
		g.latestHeads.Add(1)
		if g.latest == "" {
			w.WriteHeader(http.StatusNotFound)

			return
		}
		w.Header().Set("Location", repo+"/releases/tag/"+g.latest)
		w.WriteHeader(http.StatusFound)

	case strings.HasPrefix(p, repo+"/releases/tag/"):
		tag := strings.TrimPrefix(p, repo+"/releases/tag/")
		if _, ok := g.assets[tag]; ok || tag == g.latest {
			w.WriteHeader(http.StatusOK)

			return
		}
		w.WriteHeader(http.StatusNotFound)

	case strings.HasPrefix(p, repo+"/releases/download/"):
		rest := strings.TrimPrefix(p, repo+"/releases/download/")
		tag, name, ok := strings.Cut(rest, "/")
		if !ok || g.assets[tag] == nil || g.assets[tag][name] == nil {
			w.WriteHeader(http.StatusNotFound)

			return
		}
		w.Header().Set("Location", "/objects/"+tag+"/"+name)
		w.WriteHeader(http.StatusFound)

	case strings.HasPrefix(p, "/objects/"):
		rest := strings.TrimPrefix(p, "/objects/")
		tag, name, _ := strings.Cut(rest, "/")
		content := g.assets[tag][name]
		if content == nil {
			w.WriteHeader(http.StatusNotFound)

			return
		}
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(content); err != nil {
			return
		}

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// newTestClient wires a Client to an in-memory fake GitHub.
func newTestClient(t *testing.T, g *fakeGitHub) *Client {
	t.Helper()
	srv := httptest.NewTestServer(t, g)

	return &Client{
		HTTP:      srv.Client(),
		BaseURL:   srv.URL,
		UserAgent: "machineid/test",
		AllowHost: func(string) bool { return true },
	}
}

func makeZip(inner string, content []byte) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create(inner)
	if err != nil {
		panic(err)
	}
	if _, err := f.Write(content); err != nil {
		panic(err)
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}

	return buf.Bytes()
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)

	return hex.EncodeToString(sum[:])
}

// fakeExec answers commands from a table keyed by "name arg1 arg2…" prefix.
type fakeExec struct {
	mu      sync.Mutex
	answers map[string]ExecResult
	errs    map[string]error
	calls   []string
}

func newFakeExec() *fakeExec {
	return &fakeExec{answers: map[string]ExecResult{}, errs: map[string]error{}}
}

func (f *fakeExec) on(prefix string, res ExecResult) { f.answers[prefix] = res }
func (f *fakeExec) fail(prefix string, err error)    { f.errs[prefix] = err }

func (f *fakeExec) run(_ context.Context, name string, args ...string) (ExecResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	line := strings.Join(append([]string{name}, args...), " ")
	f.calls = append(f.calls, line)

	// Longest matching prefix wins, so a specific answer can be overridden by
	// a shorter, later one only when it is the longest match.
	if prefix := longestPrefix(f.errs, line); prefix != "" {
		return ExecResult{}, f.errs[prefix]
	}
	if prefix := longestPrefix(f.answers, line); prefix != "" {
		return f.answers[prefix], nil
	}

	return ExecResult{ExitCode: 127, Stderr: fmt.Sprintf("fake: no answer for %q", line)}, nil
}

func (f *fakeExec) called(prefix string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}

	return false
}

// longestPrefix returns the longest key of m that prefixes line, or "".
func longestPrefix[V any](m map[string]V, line string) string {
	best := ""
	for prefix := range m {
		if strings.HasPrefix(line, prefix) && len(prefix) > len(best) {
			best = prefix
		}
	}

	return best
}

// mustOK fails the test on err.
func mustOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return b
}

func mustStat(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	return info
}

func mustReadDir(t *testing.T, dir string) []os.DirEntry {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	return entries
}

func mustAsset(t *testing.T, goos, goarch string) Asset {
	t.Helper()
	a, err := AssetFor(goos, goarch)
	if err != nil {
		t.Fatal(err)
	}

	return a
}
