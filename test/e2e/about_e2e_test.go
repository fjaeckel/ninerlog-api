// Copyright (C) The NinerLog Authors
// SPDX-License-Identifier: AGPL-3.0-only

//go:build e2e

package e2e_test

import (
	"net/http"
	"strings"
	"testing"
)

// GET /about is the AGPL section 13 source offer: it answers without a token
// and names the licence and where the corresponding source lives.
func TestAboutSourceOffer(t *testing.T) {
	c := NewE2EClient(t)
	c.ClearToken()

	resp := c.GET("/about")
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		Name       string `json:"name"`
		Version    string `json:"version"`
		License    string `json:"license"`
		LicenseURL string `json:"licenseUrl"`
		SourceURL  string `json:"sourceUrl"`
	}
	if err := resp.JSON(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body.Name != "NinerLog API" {
		t.Errorf("name = %q, want NinerLog API", body.Name)
	}
	if body.Version == "" {
		t.Error("version must always be reported")
	}
	if body.License != "AGPL-3.0-only" {
		t.Errorf("license = %q, want AGPL-3.0-only", body.License)
	}
	if !strings.HasPrefix(body.LicenseURL, "https://www.gnu.org/licenses/") {
		t.Errorf("licenseUrl = %q, want the GNU licence text", body.LicenseURL)
	}
	if !strings.HasPrefix(body.SourceURL, "http://") && !strings.HasPrefix(body.SourceURL, "https://") {
		t.Errorf("sourceUrl = %q, want an http(s) URL", body.SourceURL)
	}
}
