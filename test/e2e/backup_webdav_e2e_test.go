//go:build e2e

package e2e_test

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// The bytemark/webdav container in docker-compose.e2e.yaml exposes:
//   endpoint:   http://webdav/      (inside docker)
//   username:   ninerlog
//   password:   ninerlogtest
//
// The API talks to WebDAV via the docker network alias, so when constructing a
// backup destination we use the docker hostname. The provider rejects
// non-HTTPS URLs by default, so we set allow_insecure=true in the config.
func webdavBaseURL() string {
	if v := os.Getenv("E2E_WEBDAV_URL"); v != "" {
		return v
	}
	return "http://webdav/"
}

func newWebDAVDestPayload(name string) map[string]interface{} {
	return map[string]interface{}{
		"provider":    "webdav",
		"displayName": name,
		"config": map[string]interface{}{
			"base_url":       webdavBaseURL(),
			"path":           "e2e/" + name + "/",
			"allow_insecure": true,
		},
		"credentials": map[string]interface{}{
			"username": "ninerlog",
			"password": "ninerlogtest",
		},
		"schedule":        "manual",
		"scheduleHourUtc": 3,
		"retentionCount":  10,
		"enabled":         true,
	}
}

// TestBackup_WebDAV_ListProviders confirms the WebDAV provider is registered
// and its schema is exposed for the frontend dynamic form.
func TestBackup_WebDAV_ListProviders(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("backup-webdav-providers"), "Password123!", "WebDAV Tester")

	resp := c.GET("/backups/providers")
	requireStatus(t, resp, http.StatusOK)
	var providers []backupProvider
	if err := resp.JSON(&providers); err != nil {
		t.Fatalf("decode providers: %v", err)
	}
	var p *backupProvider
	for i := range providers {
		if providers[i].Name == "webdav" {
			p = &providers[i]
			break
		}
	}
	if p == nil {
		t.Fatalf("webdav provider not registered; got %+v", providers)
	}
	if len(p.ConfigSchema.Fields) == 0 || len(p.CredentialSchema.Fields) == 0 {
		t.Fatalf("webdav provider schemas are empty: %+v", p)
	}

	// Schema sanity: base_url + path on config, username + password on creds.
	wantConfig := map[string]bool{"base_url": false, "path": false}
	for _, f := range p.ConfigSchema.Fields {
		if _, ok := wantConfig[f.Name]; ok {
			wantConfig[f.Name] = true
		}
	}
	for k, seen := range wantConfig {
		if !seen {
			t.Errorf("config schema missing field %q", k)
		}
	}
	wantCreds := map[string]bool{"username": false, "password": false}
	for _, f := range p.CredentialSchema.Fields {
		if _, ok := wantCreds[f.Name]; ok {
			wantCreds[f.Name] = true
		}
	}
	for k, seen := range wantCreds {
		if !seen {
			t.Errorf("credential schema missing field %q", k)
		}
	}
}

// TestBackup_WebDAV_FullLifecycle exercises create → test → run → list runs →
// get destination → delete against the real WebDAV server.
func TestBackup_WebDAV_FullLifecycle(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("backup-webdav-lifecycle"), "Password123!", "WebDAV Lifecycle")

	// Create.
	createResp := c.POST("/backups/destinations", newWebDAVDestPayload("lifecycle"))
	requireStatus(t, createResp, http.StatusCreated)
	var dest backupDestination
	if err := createResp.JSON(&dest); err != nil {
		t.Fatalf("decode destination: %v", err)
	}
	if dest.ID == "" || dest.Provider != "webdav" {
		t.Fatalf("unexpected created destination: %+v", dest)
	}
	if dest.CredentialHint == "" {
		t.Errorf("expected credentialHint to be populated, got empty")
	}

	// Test connection.
	testResp := c.POST("/backups/destinations/"+dest.ID+"/test", nil)
	requireStatus(t, testResp, http.StatusOK)
	var testRes backupTestResult
	if err := testResp.JSON(&testRes); err != nil {
		t.Fatalf("decode test result: %v", err)
	}
	if !testRes.Success {
		t.Fatalf("Test connection failed: %s", testRes.Message)
	}

	// Run once.
	runResp := c.POST("/backups/destinations/"+dest.ID+"/run", nil)
	if runResp.StatusCode != http.StatusAccepted {
		t.Fatalf("run: expected 202, got %d: %s", runResp.StatusCode, string(runResp.Body))
	}
	var run backupRun
	if err := runResp.JSON(&run); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	if run.Status != "success" {
		t.Fatalf("expected success, got %s: %s", run.Status, run.ErrorMessage)
	}
	if run.SizeBytes <= 0 {
		t.Errorf("expected non-zero SizeBytes, got %d", run.SizeBytes)
	}
	if !strings.Contains(run.RemotePath, "/e2e/lifecycle/") {
		t.Errorf("unexpected remote path: %q", run.RemotePath)
	}

	// List runs.
	listResp := c.GET("/backups/destinations/" + dest.ID + "/runs")
	requireStatus(t, listResp, http.StatusOK)
	var runs struct {
		Data []backupRun `json:"data"`
	}
	if err := listResp.JSON(&runs); err != nil {
		t.Fatalf("decode runs: %v", err)
	}
	if len(runs.Data) == 0 {
		t.Fatal("expected at least one run in audit log")
	}

	// Get destination — should reflect last success.
	getResp := c.GET("/backups/destinations/" + dest.ID)
	requireStatus(t, getResp, http.StatusOK)
	var refreshed backupDestination
	if err := getResp.JSON(&refreshed); err != nil {
		t.Fatalf("decode refreshed destination: %v", err)
	}
	if refreshed.Status != "active" {
		t.Errorf("expected status active, got %s (lastError=%q)", refreshed.Status, refreshed.LastError)
	}

	// Delete.
	delResp := c.DELETE("/backups/destinations/" + dest.ID)
	requireStatus(t, delResp, http.StatusNoContent)
	getResp2 := c.GET("/backups/destinations/" + dest.ID)
	if getResp2.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", getResp2.StatusCode)
	}
}

// TestBackup_WebDAV_InvalidCredentials confirms creation-time validation
// rejects a destination with bad credentials.
func TestBackup_WebDAV_InvalidCredentials(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("backup-webdav-badcreds"), "Password123!", "WebDAV BadCreds")

	payload := newWebDAVDestPayload("badcreds")
	payload["credentials"] = map[string]interface{}{
		"username": "wronguser",
		"password": "wrongpass",
	}
	createResp := c.POST("/backups/destinations", payload)
	if createResp.StatusCode != http.StatusUnauthorized && createResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 401/400 on bad credentials, got %d: %s", createResp.StatusCode, string(createResp.Body))
	}
}

// TestBackup_WebDAV_RejectsHTTPWithoutInsecureFlag ensures the provider
// refuses plaintext URLs by default — a security regression guard.
func TestBackup_WebDAV_RejectsHTTPWithoutInsecureFlag(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("backup-webdav-noinsecure"), "Password123!", "WebDAV NoInsecure")

	payload := newWebDAVDestPayload("noinsecure")
	cfg := payload["config"].(map[string]interface{})
	delete(cfg, "allow_insecure")
	// Force http:// without the override -> should be rejected.
	createResp := c.POST("/backups/destinations", payload)
	if createResp.StatusCode == http.StatusCreated {
		t.Fatalf("expected non-201 when http:// used without allow_insecure, got 201")
	}
}

// TestBackup_WebDAV_UserIsolation ensures one user cannot see or delete
// another user's WebDAV destination.
func TestBackup_WebDAV_UserIsolation(t *testing.T) {
	cA := NewE2EClient(t)
	registerAndLogin(t, cA, uniqueEmail("backup-webdav-userA"), "Password123!", "User A")
	createResp := cA.POST("/backups/destinations", newWebDAVDestPayload("userA"+time.Now().Format("150405")))
	requireStatus(t, createResp, http.StatusCreated)
	var dest backupDestination
	createResp.JSON(&dest)

	cB := NewE2EClient(t)
	registerAndLogin(t, cB, uniqueEmail("backup-webdav-userB"), "Password123!", "User B")
	listResp := cB.GET("/backups/destinations")
	requireStatus(t, listResp, http.StatusOK)
	if strings.Contains(string(listResp.Body), dest.ID) {
		t.Fatalf("User B sees User A's destination ID in list: %s", string(listResp.Body))
	}
	getResp := cB.GET("/backups/destinations/" + dest.ID)
	if getResp.StatusCode != http.StatusNotFound && getResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 404/403 cross-user read, got %d", getResp.StatusCode)
	}

	delA := cA.DELETE("/backups/destinations/" + dest.ID)
	requireStatus(t, delA, http.StatusNoContent)
}
