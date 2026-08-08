package main

import (
	"math"
	"strconv"
	"testing"
)

func TestEnvIntNarrow(t *testing.T) {
	const key = "NINERLOG_TEST_ENV_INT_NARROW"
	const def = 10

	cases := []struct {
		name string
		set  bool
		val  string
		want int
	}{
		{name: "unset keeps default", set: false, want: def},
		{name: "empty keeps default", set: true, val: "", want: def},
		{name: "valid value is used", set: true, val: "42", want: 42},
		{name: "one is allowed", set: true, val: "1", want: 1},
		{name: "zero keeps default", set: true, val: "0", want: def},
		{name: "negative keeps default", set: true, val: "-5", want: def},
		{name: "unparseable keeps default", set: true, val: "many", want: def},
		{name: "max int32 is used", set: true, val: strconv.Itoa(math.MaxInt32), want: math.MaxInt32},

		// The reason this helper exists: a value past the 32-bit range must not
		// be truncated into a small or negative cap on any platform. Parsing at
		// a 32-bit width rejects it outright instead.
		{name: "above int32 keeps default", set: true, val: strconv.FormatInt(math.MaxInt32+1, 10), want: def},
		{name: "far above int32 keeps default", set: true, val: "9223372036854775807", want: def},
		{name: "overflows int64 keeps default", set: true, val: "99999999999999999999", want: def},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv(key, tc.val)
			}
			if got := envIntNarrow(key, def); got != tc.want {
				t.Errorf("envIntNarrow(%q=%q) = %d, want %d", key, tc.val, got, tc.want)
			}
		})
	}
}

// envBool gates subsystems that make outbound connections, so "not the string
// false" is not good enough: a typo or a "0" must not read as "on".
func TestEnvBool(t *testing.T) {
	const key = "NINERLOG_TEST_ENV_BOOL"

	cases := []struct {
		name string
		set  bool
		val  string
		def  bool
		want bool
	}{
		{name: "unset keeps default", def: false, want: false},
		{name: "unset keeps a true default", def: true, want: true},
		{name: "empty keeps default", set: true, val: "", def: false, want: false},
		{name: "true enables", set: true, val: "true", def: false, want: true},
		{name: "1 enables", set: true, val: "1", def: false, want: true},
		{name: "false disables", set: true, val: "false", def: true, want: false},
		{name: "0 disables", set: true, val: "0", def: true, want: false},

		// The difference from envBoolWithLegacy, which would read both of these
		// as "on" because they are not the exact string "false".
		{name: "no keeps default", set: true, val: "no", def: false, want: false},
		{name: "typo keeps default", set: true, val: "ture", def: false, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv(key, tc.val)
			}
			if got := envBool(key, tc.def); got != tc.want {
				t.Errorf("envBool(%q=%q, def=%v) = %v, want %v", key, tc.val, tc.def, got, tc.want)
			}
		})
	}
}
