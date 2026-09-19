package selfupdate

import (
	"errors"
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"v1.2.3", "v1.2.3", true},
		{"1.2.3", "v1.2.3", true},
		{"v1", "v1.0.0", true},
		{"v1.2", "v1.2.0", true},
		{"v0.2.0-rc.1", "v0.2.0-rc.1", true},
		{"v0.2.0-rc.1+build.5", "v0.2.0-rc.1", true},
		{"v0.2.0+meta", "v0.2.0", true},
		{" v0.1.3 ", "v0.1.3", true},
		{"0.0.0", "v0.0.0", true},
		{"devel", "", false},
		{"main", "", false},
		{"feat/DCT-101", "", false},
		{"", "", false},
		{"v", "", false},
		{"v1.2.3.4", "", false},
		{"v1.-2.3", "", false},
		{"v1.2.3-", "", false},
		{"v1.2.3-rc..1", "", false},
		{"v1.2.3-rc_1", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, ok := Canonical(tt.in)
			if ok != tt.ok || got != tt.want {
				t.Errorf("Canonical(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"v1.0.0", "v1.0.0", 0},
		{"v1.0.0", "1.0.0", 0},
		{"v1.0.0", "v1.0.1", -1},
		{"v1.1.0", "v1.0.9", 1},
		{"v2.0.0", "v1.99.99", 1},
		{"v1.0.0-rc.1", "v1.0.0", -1},
		{"v1.0.0", "v1.0.0-rc.1", 1},
		{"v1.0.0-rc.9", "v1.0.0-rc.10", -1},
		{"v1.0.0-alpha", "v1.0.0-beta", -1},
		{"v1.0.0-alpha", "v1.0.0-alpha.1", -1},
		{"v1.0.0-1", "v1.0.0-alpha", -1},
		{"v1.0.0-rc.1+a", "v1.0.0-rc.1+b", 0},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			va, _ := ParseVersion(tt.a)
			vb, _ := ParseVersion(tt.b)
			if got := Compare(va, vb); got != tt.want {
				t.Errorf("Compare(%s, %s) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
			if got := Compare(vb, va); got != -tt.want {
				t.Errorf("Compare(%s, %s) = %d, want %d", tt.b, tt.a, got, -tt.want)
			}
		})
	}
}

func TestIsUpgrade(t *testing.T) {
	up, err := IsUpgrade("v0.1.3", "v0.2.0")
	if err != nil || !up {
		t.Errorf("IsUpgrade(v0.1.3, v0.2.0) = %v, %v", up, err)
	}

	up, err = IsUpgrade("v0.2.0", "v0.2.0")
	if err != nil || up {
		t.Errorf("IsUpgrade(same) = %v, %v", up, err)
	}

	up, err = IsUpgrade("0.0.0", "v0.1.0")
	if err != nil || !up {
		t.Errorf("IsUpgrade(0.0.0, v0.1.0) = %v, %v; an un-injected build is older than everything", up, err)
	}

	_, err = IsUpgrade("devel", "v0.2.0")
	if _, ok := errors.AsType[*NotComparableError](err); !ok {
		t.Fatalf("IsUpgrade(devel) error = %v, want NotComparableError", err)
	}
	if RemedyOf(err) == "" {
		t.Error("NotComparableError should carry a remedy")
	}
}

func TestSameVersion(t *testing.T) {
	if !SameVersion("v0.2.0", "0.2.0") {
		t.Error("v-prefix should not matter")
	}
	if SameVersion("v0.2.0", "v0.2.1") {
		t.Error("different patch")
	}
	if !SameVersion("devel", "devel") {
		t.Error("non-versions compare by text")
	}
	if SameVersion("devel", "v0.2.0") {
		t.Error("non-version vs version")
	}
}
