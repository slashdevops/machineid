package selfupdate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestBudget(t *testing.T, limit int) (*Budget, *time.Time) {
	t.Helper()
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	b := NewBudget(filepath.Join(t.TempDir(), "state.json"), limit)
	b.Now = func() time.Time { return now }

	return b, &now
}

func TestBudgetAllowsUpToLimitThenRefuses(t *testing.T) {
	b, now := newTestBudget(t, 3)

	for i := range 3 {
		ok, _ := b.Allow()
		if !ok {
			t.Fatalf("lookup %d should be allowed", i+1)
		}
		if err := b.RecordLookup("v0.3.0"); err != nil {
			t.Fatal(err)
		}
		*now = now.Add(time.Minute)
	}

	ok, next := b.Allow()
	if ok {
		t.Fatal("fourth lookup within the hour should be refused")
	}
	wantNext := time.Date(2026, 9, 19, 13, 0, 0, 0, time.UTC)
	if !next.Equal(wantNext) {
		t.Errorf("next = %v, want %v", next, wantNext)
	}
	if b.Remaining() != 0 {
		t.Errorf("Remaining = %d", b.Remaining())
	}

	// The window slides: once the oldest lookup is an hour old, one slot frees up.
	*now = wantNext.Add(time.Second)
	if ok, _ := b.Allow(); !ok {
		t.Fatal("a slot should free up after the window slides")
	}
	if b.Remaining() != 1 {
		t.Errorf("Remaining after slide = %d, want 1", b.Remaining())
	}
}

func TestBudgetCacheTTL(t *testing.T) {
	b, now := newTestBudget(t, 5)
	if _, _, ok := b.Cached(); ok {
		t.Fatal("empty budget should have no cache")
	}

	mustOK(t, b.RecordLookup("v0.3.0"))
	*now = now.Add(30 * time.Minute)
	tag, age, ok := b.Cached()
	if !ok || tag != "v0.3.0" || age != 30*time.Minute {
		t.Errorf("Cached = %q, %v, %v", tag, age, ok)
	}

	*now = now.Add(31 * time.Minute)
	if _, _, ok := b.Cached(); ok {
		t.Error("cache older than CacheTTL should not be served")
	}
	if tag, _, ok := b.CachedAny(); !ok || tag != "v0.3.0" {
		t.Error("CachedAny should still return the stale answer")
	}
}

func TestBudgetPersistsAcrossInstances(t *testing.T) {
	b, now := newTestBudget(t, 2)
	mustOK(t, b.RecordLookup("v0.3.0"))
	mustOK(t, b.RecordLookup(""))
	again := NewBudget(b.Path, 2)
	again.Now = func() time.Time { return *now }
	if ok, _ := again.Allow(); ok {
		t.Fatal("a fresh instance must see the persisted lookups")
	}
	if tag, _, ok := again.Cached(); !ok || tag != "v0.3.0" {
		t.Errorf("cached tag not persisted: %q %v", tag, ok)
	}

	info, err := os.Stat(b.Path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("state file mode = %o, want 600", perm)
	}
}

func TestBudgetBackoff(t *testing.T) {
	b, now := newTestBudget(t, 5)
	until := now.Add(10 * time.Minute)
	mustOK(t, b.RecordBackoff(until))
	ok, next := b.Allow()
	if ok || !next.Equal(until) {
		t.Errorf("Allow during backoff = %v, %v", ok, next)
	}

	*now = until.Add(time.Second)
	if ok, _ := b.Allow(); !ok {
		t.Error("backoff should expire")
	}
}

func TestBudgetCorruptStateIsIgnored(t *testing.T) {
	b, _ := newTestBudget(t, 5)
	if err := os.WriteFile(b.Path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := b.Load(); err == nil {
		t.Error("Load should report the corrupt file")
	}
	if ok, _ := b.Allow(); !ok {
		t.Error("corrupt state must not block lookups")
	}
}

func TestBudgetUnwritablePathDoesNotBlock(t *testing.T) {
	b := NewBudget(filepath.Join(t.TempDir(), "missing", "deeper", "state.json"), 5)
	// The directory is created on save; make the parent a file so MkdirAll fails.
	parent := filepath.Dir(filepath.Dir(b.Path))
	if err := os.WriteFile(parent, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := b.RecordLookup("v0.3.0"); err == nil {
		t.Error("expected a save error")
	}
	if ok, _ := b.Allow(); !ok {
		t.Error("a failed save must not block lookups")
	}
}

func TestBudgetLimitZeroDisables(t *testing.T) {
	b, _ := newTestBudget(t, 0)
	for range 50 {
		if ok, _ := b.Allow(); !ok {
			t.Fatal("limit 0 must always allow")
		}
		mustOK(t, b.RecordLookup("v0.3.0"))
	}
	if b.Remaining() != -1 {
		t.Errorf("Remaining with no limit = %d, want -1", b.Remaining())
	}
}

func TestLimitFromEnv(t *testing.T) {
	t.Setenv(EnvBudget, "10")
	if got := LimitFromEnv(5); got != 10 {
		t.Errorf("got %d", got)
	}
	t.Setenv(EnvBudget, "0")
	if got := LimitFromEnv(5); got != 0 {
		t.Errorf("got %d", got)
	}
	t.Setenv(EnvBudget, "nope")
	if got := LimitFromEnv(5); got != 5 {
		t.Errorf("invalid should fall back, got %d", got)
	}
	t.Setenv(EnvBudget, "-1")
	if got := LimitFromEnv(5); got != 5 {
		t.Errorf("negative should fall back, got %d", got)
	}
}

func TestDefaultStatePath(t *testing.T) {
	p, err := DefaultStatePath()
	if err != nil {
		t.Skip("no user cache dir:", err)
	}
	if filepath.Base(p) != "update-state.json" || filepath.Base(filepath.Dir(p)) != ToolName {
		t.Errorf("DefaultStatePath = %s", p)
	}
}

func TestBudgetInMemoryOnly(t *testing.T) {
	b := NewBudget("", 1)
	if err := b.RecordLookup("v1.0.0"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := b.Allow(); ok {
		t.Error("limit should apply in memory too")
	}
	if err := b.Load(); err != nil {
		t.Error(err)
	}
	if !errors.Is(nil, nil) { // keep errors imported for symmetry with other tests
		t.Fatal()
	}
}
