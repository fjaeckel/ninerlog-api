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
