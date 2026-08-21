package updatecheck

import (
	"strconv"
	"strings"
)

// version is a parsed semantic version. Build metadata is discarded.
type version struct {
	major, minor, patch int
	prerelease          string
}

// parseVersion parses "v1.2.3", "1.2.3", "1.2.3-rc.1" or "1.2.3+build".
// The second result is false for anything else, including "dev", "latest" and
// bare commit SHAs.
func parseVersion(s string) (version, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	if s == "" {
		return version{}, false
	}

	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}

	var pre string
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre, s = s[i+1:], s[:i]
	}

	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return version{}, false
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || (len(p) > 1 && p[0] == '0') {
			return version{}, false
		}
		nums[i] = n
	}
	return version{major: nums[0], minor: nums[1], patch: nums[2], prerelease: pre}, true
}

// compare returns -1 if a sorts before b, 1 if after, 0 if equal.
func compare(a, b version) int {
	for _, pair := range [][2]int{
		{a.major, b.major}, {a.minor, b.minor}, {a.patch, b.patch},
	} {
		if pair[0] != pair[1] {
			if pair[0] < pair[1] {
				return -1
			}
			return 1
		}
	}
	return comparePrerelease(a.prerelease, b.prerelease)
}

// comparePrerelease orders release above prerelease, then identifier by
// identifier: numeric identifiers compare numerically and sort below
// alphanumeric ones.
func comparePrerelease(a, b string) int {
	switch {
	case a == b:
		return 0
	case a == "":
		return 1
	case b == "":
		return -1
	}

	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		an, aErr := strconv.Atoi(as[i])
		bn, bErr := strconv.Atoi(bs[i])
		switch {
		case aErr == nil && bErr == nil:
			if an != bn {
				return sign(an - bn)
			}
		case aErr == nil:
			return -1
		case bErr == nil:
			return 1
		default:
			if c := strings.Compare(as[i], bs[i]); c != 0 {
				return c
			}
		}
	}
	return sign(len(as) - len(bs))
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}

// newer reports whether latest sorts above current. Both must parse.
func newer(current, latest string) (bool, bool) {
	c, okC := parseVersion(current)
	l, okL := parseVersion(latest)
	if !okC || !okL {
		return false, false
	}
	return compare(l, c) > 0, true
}
