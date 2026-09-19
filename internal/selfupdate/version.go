package selfupdate

import (
	"strconv"
	"strings"
)

// Version is a parsed semantic version. Build metadata is discarded on parse
// because it does not participate in ordering (SemVer §10).
type Version struct {
	Pre   []string
	Major int
	Minor int
	Patch int
}

// ParseVersion parses a release tag or version string.
//
// A leading "v" is optional. Minor and patch may be omitted and default to
// zero, so "v1" and "v1.0.0" are the same version. A pre-release suffix
// ("-rc.1") is kept; build metadata ("+abc") is dropped. Anything else, such
// as the "devel" or branch names a local build reports, is not a version and
// yields ok == false.
func ParseVersion(s string) (Version, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	if s == "" {
		return Version{}, false
	}

	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}

	core, pre, hasPre := strings.Cut(s, "-")

	var v Version

	parts := strings.Split(core, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return Version{}, false
	}

	nums := make([]int, 3)
	for i, part := range parts {
		n, ok := parseNumericIdentifier(part)
		if !ok {
			return Version{}, false
		}
		nums[i] = n
	}
	v.Major, v.Minor, v.Patch = nums[0], nums[1], nums[2]

	if hasPre {
		if pre == "" {
			return Version{}, false
		}
		for id := range strings.SplitSeq(pre, ".") {
			if id == "" || !isIdentifier(id) {
				return Version{}, false
			}
			v.Pre = append(v.Pre, id)
		}
	}

	return v, true
}

// parseNumericIdentifier parses a non-negative decimal without a sign.
func parseNumericIdentifier(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}

	return n, true
}

// isIdentifier reports whether s is [0-9A-Za-z-]+.
func isIdentifier(s string) bool {
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '-':
		default:
			return false
		}
	}

	return s != ""
}

// String renders the canonical form: "v1.2.3" or "v1.2.3-rc.1".
func (v Version) String() string {
	s := "v" + strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor) + "." + strconv.Itoa(v.Patch)
	if len(v.Pre) > 0 {
		s += "-" + strings.Join(v.Pre, ".")
	}

	return s
}

// Compare orders two versions per SemVer §11: -1 if a < b, 0 if equal, +1 if a > b.
func Compare(a, b Version) int {
	switch {
	case a.Major != b.Major:
		return cmpInt(a.Major, b.Major)
	case a.Minor != b.Minor:
		return cmpInt(a.Minor, b.Minor)
	case a.Patch != b.Patch:
		return cmpInt(a.Patch, b.Patch)
	}

	// A version without pre-release is higher than the same version with one.
	switch {
	case len(a.Pre) == 0 && len(b.Pre) == 0:
		return 0
	case len(a.Pre) == 0:
		return 1
	case len(b.Pre) == 0:
		return -1
	}

	for i := 0; i < len(a.Pre) && i < len(b.Pre); i++ {
		if c := comparePreIdentifier(a.Pre[i], b.Pre[i]); c != 0 {
			return c
		}
	}

	return cmpInt(len(a.Pre), len(b.Pre))
}

// comparePreIdentifier orders two pre-release identifiers: numeric ones
// numerically, alphanumeric ones lexically, and numeric below alphanumeric.
func comparePreIdentifier(a, b string) int {
	an, aNum := parseNumericIdentifier(a)
	bn, bNum := parseNumericIdentifier(b)

	switch {
	case aNum && bNum:
		return cmpInt(an, bn)
	case aNum:
		return -1
	case bNum:
		return 1
	default:
		return strings.Compare(a, b)
	}
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// Canonical normalises a version string to its "v"-prefixed canonical form.
// ok is false when s is not a version.
func Canonical(s string) (string, bool) {
	v, ok := ParseVersion(s)
	if !ok {
		return "", false
	}

	return v.String(), true
}

// IsUpgrade reports whether candidate is strictly newer than current.
//
// A current version that cannot be parsed (a local build reporting "devel" or
// a branch name) cannot be ordered and returns [NotComparableError], whose
// remedy is to name a tag explicitly.
func IsUpgrade(current, candidate string) (bool, error) {
	cur, ok := ParseVersion(current)
	if !ok {
		return false, &NotComparableError{Version: current}
	}

	cand, ok := ParseVersion(candidate)
	if !ok {
		return false, &NotComparableError{Version: candidate}
	}

	return Compare(cand, cur) > 0, nil
}

// SameVersion reports whether two version strings denote the same version.
// Strings that are not versions compare by exact text.
func SameVersion(a, b string) bool {
	va, okA := ParseVersion(a)
	vb, okB := ParseVersion(b)
	if okA && okB {
		return Compare(va, vb) == 0
	}

	return strings.TrimSpace(a) == strings.TrimSpace(b)
}
