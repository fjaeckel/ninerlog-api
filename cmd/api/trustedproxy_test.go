package main

import (
	"reflect"
	"testing"
)

func TestSplitAndTrim(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"172.16.0.0/12", []string{"172.16.0.0/12"}},
		{"10.1.2.3, 10.1.2.4", []string{"10.1.2.3", "10.1.2.4"}},
		{" 127.0.0.1 ,, ::1 ", []string{"127.0.0.1", "::1"}},
		{"", []string{}},
	}
	for _, tt := range tests {
		if got := splitAndTrim(tt.in); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("splitAndTrim(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

// The default must NOT include the RFC-1918 ranges. Trusting those meant any
// client reaching the API from inside them (e.g. via the published port on the
// Docker bridge) could forge X-Real-IP and defeat every IP-keyed rate limit.
func TestDefaultTrustedProxies_ExcludesPrivateRanges(t *testing.T) {
	for _, p := range defaultTrustedProxies {
		switch p {
		case "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16":
			t.Errorf("default trusted proxies must not include the broad private range %q", p)
		}
	}
	if len(defaultTrustedProxies) == 0 {
		t.Error("expected loopback entries in the default trusted proxies")
	}
}
