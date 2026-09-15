// Copyright (C) The NinerLog Authors
// SPDX-License-Identifier: AGPL-3.0-only

package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type aboutBody struct {
	Name       string  `json:"name"`
	Version    string  `json:"version"`
	Commit     *string `json:"commit"`
	License    string  `json:"license"`
	LicenseURL string  `json:"licenseUrl"`
	SourceURL  string  `json:"sourceUrl"`
}

func getAbout(t *testing.T, h *APIHandler) aboutBody {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/about", nil)

	h.GetAbout(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body aboutBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body
}

func TestGetAbout_ReportsLicenceAndDefaultSource(t *testing.T) {
	t.Setenv("APP_VERSION", "")
	t.Setenv("APP_COMMIT", "")
	h, _ := setupTestHandler()

	body := getAbout(t, h)

	if body.Name != "NinerLog API" {
		t.Errorf("name = %q, want NinerLog API", body.Name)
	}
	if body.License != "AGPL-3.0-only" {
		t.Errorf("license = %q, want AGPL-3.0-only", body.License)
	}
	if body.LicenseURL != "https://www.gnu.org/licenses/agpl-3.0.html" {
		t.Errorf("licenseUrl = %q", body.LicenseURL)
	}
	if body.SourceURL != DefaultSourceURL {
		t.Errorf("sourceUrl = %q, want %q when SOURCE_URL is unset", body.SourceURL, DefaultSourceURL)
	}
	if body.Version != "dev" {
		t.Errorf("version = %q, want dev for an unstamped build", body.Version)
	}
	if body.Commit != nil {
		t.Errorf("commit = %q, want absent when unknown", *body.Commit)
	}
}

func TestGetAbout_ReportsConfiguredSourceAndCommit(t *testing.T) {
	t.Setenv("APP_VERSION", "v9.9.9")
	t.Setenv("APP_COMMIT", "4f2c1ab9d3e5c6178b0a2d4e6f8091a2b3c4d5e6")
	h, _ := setupTestHandler()
	h.SetSourceURL("https://git.example.org/pilot/ninerlog-api")

	body := getAbout(t, h)

	if body.SourceURL != "https://git.example.org/pilot/ninerlog-api" {
		t.Errorf("sourceUrl = %q, want the configured URL", body.SourceURL)
	}
	if body.Version != "v9.9.9" {
		t.Errorf("version = %q, want v9.9.9", body.Version)
	}
	if body.Commit == nil || *body.Commit != "4f2c1ab9d3e5c6178b0a2d4e6f8091a2b3c4d5e6" {
		t.Errorf("commit = %v, want the stamped commit", body.Commit)
	}
}
