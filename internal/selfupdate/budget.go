package selfupdate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"
)

// Budget defaults. One "lookup" is a network round trip that resolves a
// release tag; downloads are not counted because they only follow a confirmed
// plan to install a newer version.
const (
	// DefaultLimit is how many live lookups are allowed per Window.
	DefaultLimit = 5

	// Window is the rolling period the limit applies to.
	Window = time.Hour

	// CacheTTL is how long a resolved "latest" tag is reused without a lookup.
	CacheTTL = time.Hour

	// EnvBudget overrides DefaultLimit. "0" disables the limit.
	EnvBudget = "MACHINEID_UPDATE_BUDGET"

	stateSchema = 1
)

// State is what the budget persists between runs.
type State struct {
	Latest    *CachedLatest `json:"latest,omitempty"`
	NotBefore time.Time     `json:"not_before,omitzero"`
	Lookups   []time.Time   `json:"lookups"`
	Schema    int           `json:"schema"`
}

// CachedLatest is the last "latest release" answer and when it was fetched.
type CachedLatest struct {
	CheckedAt time.Time `json:"checked_at"`
	Tag       string    `json:"tag"`
}

// Budget throttles live lookups per user per machine and caches their result.
//
// It is a courtesy to github.com, not a security control: an unreadable,
// corrupt or unwritable state file never blocks an update, it only means the
// limit is not enforced for that run.
type Budget struct {

	// loadErr is the informational error from the last Load, if any.
	loadErr error

	// Now supplies the clock; nil uses time.Now.
	Now func() time.Time

	// Path is the state file. "" disables persistence (in-memory only).
	Path string

	state State

	// Limit is the number of live lookups per Window; <= 0 disables the limit.
	Limit int

	loaded bool
}

// DefaultStatePath is <user cache dir>/machineid/update-state.json.
func DefaultStatePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, ToolName, "update-state.json"), nil
}

// LimitFromEnv returns the limit set by EnvBudget, or def when unset or invalid.
func LimitFromEnv(def int) int {
	v, ok := os.LookupEnv(EnvBudget)
	if !ok {
		return def
	}

	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return def
	}

	return n
}

// NewBudget returns a budget persisted at path with the given limit.
func NewBudget(path string, limit int) *Budget {
	return &Budget{Path: path, Limit: limit}
}

func (b *Budget) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}

	return time.Now()
}

// Load reads the state file. Missing or unreadable files yield empty state
// and no error; the returned error is informational for a debug log.
func (b *Budget) Load() error {
	b.loaded = true
	b.state = State{Schema: stateSchema}

	if b.Path == "" {
		return nil
	}

	data, err := os.ReadFile(b.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("reading %s: %w", b.Path, err)
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil || s.Schema != stateSchema {
		return fmt.Errorf("ignoring unreadable state in %s", b.Path)
	}

	b.state = s

	return nil
}

// Save writes the state file with 0600 permissions, creating the directory.
func (b *Budget) Save() error {
	if b.Path == "" {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(b.Path), 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(b.state, "", "  ")
	if err != nil {
		return err
	}

	tmp := b.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}

	return os.Rename(tmp, b.Path)
}

func (b *Budget) ensureLoaded() {
	if !b.loaded {
		b.loadErr = b.Load()
	}
}

// LoadError returns the informational error from the last Load, if any.
func (b *Budget) LoadError() error { return b.loadErr }

// Cached returns the cached latest tag when it is younger than CacheTTL.
func (b *Budget) Cached() (tag string, age time.Duration, ok bool) {
	tag, checkedAt, ok := b.CachedAny()
	if !ok {
		return "", 0, false
	}

	age = b.now().Sub(checkedAt)
	if age < 0 || age >= CacheTTL {
		return "", age, false
	}

	return tag, age, true
}

// CachedAny returns the cached latest tag regardless of age.
func (b *Budget) CachedAny() (tag string, checkedAt time.Time, ok bool) {
	b.ensureLoaded()

	if b.state.Latest == nil || b.state.Latest.Tag == "" {
		return "", time.Time{}, false
	}

	return b.state.Latest.Tag, b.state.Latest.CheckedAt, true
}

// prune drops lookups outside the rolling window.
func (b *Budget) prune() {
	cutoff := b.now().Add(-Window)
	b.state.Lookups = slices.DeleteFunc(b.state.Lookups, func(t time.Time) bool { return !t.After(cutoff) })
}

// Allow reports whether a live lookup may happen now. When it may not, nextAt
// is when it will be allowed again.
func (b *Budget) Allow() (ok bool, nextAt time.Time) {
	b.ensureLoaded()
	b.prune()

	now := b.now()
	if b.state.NotBefore.After(now) {
		return false, b.state.NotBefore
	}

	if b.Limit <= 0 || len(b.state.Lookups) < b.Limit {
		return true, now
	}

	oldest := slices.MinFunc(b.state.Lookups, time.Time.Compare)

	return false, oldest.Add(Window)
}

// Remaining is how many live lookups are left in the current window.
func (b *Budget) Remaining() int {
	b.ensureLoaded()
	b.prune()

	if b.Limit <= 0 {
		return -1
	}

	return max(b.Limit-len(b.state.Lookups), 0)
}

// RecordLookup notes that a live lookup happened now and, when tag is not
// empty, caches it as the latest release. Persisting failures are returned
// for logging and never fail the caller.
func (b *Budget) RecordLookup(tag string) error {
	b.ensureLoaded()
	b.prune()

	now := b.now()
	b.state.Lookups = append(b.state.Lookups, now)
	if tag != "" {
		b.state.Latest = &CachedLatest{Tag: tag, CheckedAt: now}
	}

	return b.Save()
}

// RecordBackoff records that GitHub asked us to wait until t.
func (b *Budget) RecordBackoff(t time.Time) error {
	b.ensureLoaded()
	b.state.NotBefore = t

	return b.Save()
}
