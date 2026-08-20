package updatecheck

import "testing"

func TestParseVersion(t *testing.T) {
	tests := []struct {
		in    string
		ok    bool
		major int
		minor int
		patch int
		pre   string
	}{
		{in: "v1.3.4", ok: true, major: 1, minor: 3, patch: 4},
		{in: "1.3.4", ok: true, major: 1, minor: 3, patch: 4},
		{in: " v10.0.11 ", ok: true, major: 10, minor: 0, patch: 11},
		{in: "1.3.4-rc.1", ok: true, major: 1, minor: 3, patch: 4, pre: "rc.1"},
		{in: "1.3.4+build.7", ok: true, major: 1, minor: 3, patch: 4},
		{in: "dev"},
		{in: "latest"},
		{in: "1.3"},
		{in: "1.3.4.5"},
		{in: "v1.3.x"},
		{in: "01.3.4"},
		{in: ""},
		{in: "sha-abc1234"},
	}

	for _, tt := range tests {
		got, ok := parseVersion(tt.in)
		if ok != tt.ok {
			t.Errorf("parseVersion(%q) ok = %v, want %v", tt.in, ok, tt.ok)
			continue
		}
		if !tt.ok {
			continue
		}
		if got.major != tt.major || got.minor != tt.minor || got.patch != tt.patch || got.prerelease != tt.pre {
			t.Errorf("parseVersion(%q) = %+v, want %d.%d.%d-%q",
				tt.in, got, tt.major, tt.minor, tt.patch, tt.pre)
		}
	}
}

func TestNewer(t *testing.T) {
	tests := []struct {
		current    string
		latest     string
		behind     bool
		comparable bool
	}{
		{current: "v1.3.4", latest: "v1.3.5", behind: true, comparable: true},
		{current: "v1.3.4", latest: "v1.4.0", behind: true, comparable: true},
		{current: "v1.3.4", latest: "v2.0.0", behind: true, comparable: true},
		{current: "v1.3.4", latest: "v1.3.4", comparable: true},
		{current: "v1.3.5", latest: "v1.3.4", comparable: true},
		{current: "v1.10.0", latest: "v1.9.0", comparable: true},
		{current: "v1.3.4-rc.1", latest: "v1.3.4", behind: true, comparable: true},
		{current: "v1.3.4", latest: "v1.3.4-rc.1", comparable: true},
		{current: "v1.3.4-rc.1", latest: "v1.3.4-rc.2", behind: true, comparable: true},
		{current: "v1.3.4-rc.2", latest: "v1.3.4-rc.10", behind: true, comparable: true},
		{current: "v1.3.4-alpha", latest: "v1.3.4-beta", behind: true, comparable: true},
		{current: "v1.3.4-rc.1", latest: "v1.3.4-rc.1.1", behind: true, comparable: true},
		{current: "dev", latest: "v1.3.5"},
		{current: "latest", latest: "v1.3.5"},
		{current: "v1.3.4", latest: "nightly"},
	}

	for _, tt := range tests {
		behind, comparable := newer(tt.current, tt.latest)
		if behind != tt.behind || comparable != tt.comparable {
			t.Errorf("newer(%q, %q) = (%v, %v), want (%v, %v)",
				tt.current, tt.latest, behind, comparable, tt.behind, tt.comparable)
		}
	}
}
