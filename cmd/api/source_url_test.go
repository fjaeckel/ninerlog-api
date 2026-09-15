// Copyright (C) The NinerLog Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/api/handlers"
)

func TestSourceURLFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    string
		wantErr bool
	}{
		{"unset falls back to upstream", "", handlers.DefaultSourceURL, false},
		{"blank falls back to upstream", "   ", handlers.DefaultSourceURL, false},
		{"https URL", "https://git.example.org/pilot/ninerlog-api", "https://git.example.org/pilot/ninerlog-api", false},
		{"http URL", "http://forge.local/ninerlog", "http://forge.local/ninerlog", false},
		{"no scheme", "github.com/fjaeckel/ninerlog-api", "", true},
		{"wrong scheme", "ftp://example.org/src.tar.gz", "", true},
		{"no host", "https:///ninerlog", "", true},
		{"unparseable", "https://exa mple.org/%zz", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SOURCE_URL", tt.value)
			got, err := sourceURLFromEnv()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
