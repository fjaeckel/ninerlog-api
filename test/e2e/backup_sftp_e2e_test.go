//go:build e2e

package e2e_test

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// The atmoz/sftp container in docker-compose.e2e.yaml exposes:
//   host:     sftp        (inside the docker network)
//   port:     22
//   username: ninerlog
//   password: ninerlogtest
//   writable folder: /upload (relative to the user's home)
//
// The atmoz/sftp image regenerates the host key on container start so the
// fingerprint isn't known up front. The e2e tests set
// accept_any_host_key=true to skip host-key verification — which is the
// documented insecure-but-explicit mode the provider supports for trusted
// networks and tests.
func sftpHost() string {
	if v := os.Getenv("E2E_SFTP_HOST"); v != "" {
		return v
	}
	return "sftp"
}

func newSFTPDestPayload(name string) map[string]interface{} {
	return map[string]interface{}{
		"provider":    "sftp",
		"displayName": name,
		"config": map[string]interface{}{
			"host":                sftpHost(),
			"port":                "22",
			"path":                "upload/" + name + "/",
			"accept_any_host_key": true,
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

// TestBackup_SFTP_ListProviders confirms the SFTP provider is registered and
// its schema is exposed for the frontend dynamic form.
func TestBackup_SFTP_ListProviders(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("backup-sftp-providers"), "Password123!", "SFTP Tester")

	resp := c.GET("/backups/providers")
	requireStatus(t, resp, http.StatusOK)
	var providers []backupProvider
	if err := resp.JSON(&providers); err != nil {
		t.Fatalf("decode providers: %v", err)
	}
	var p *backupProvider
	for i := range providers {
		if providers[i].Name == "sftp" {
			p = &providers[i]
			break
		}
	}
	if p == nil {
		t.Fatalf("sftp provider not registered; got %+v", providers)
	}
	if len(p.ConfigSchema.Fields) == 0 || len(p.CredentialSchema.Fields) == 0 {
		t.Fatalf("sftp provider schemas are empty: %+v", p)
	}

	wantConfig := map[string]bool{"host": false, "port": false, "path": false, "host_key_fingerprint": false}
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

// TestBackup_SFTP_FullLifecycle exercises create → test → run → list runs →
// get destination → delete against the real SFTP server.
func TestBackup_SFTP_FullLifecycle(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("backup-sftp-lifecycle"), "Password123!", "SFTP Lifecycle")

	// Create.
	createResp := c.POST("/backups/destinations", newSFTPDestPayload("lifecycle"))
	requireStatus(t, createResp, http.StatusCreated)
	var dest backupDestination
	if err := createResp.JSON(&dest); err != nil {
		t.Fatalf("decode destination: %v", err)
	}
	if dest.ID == "" || dest.Provider != "sftp" {
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
	if !strings.Contains(run.RemotePath, "upload/lifecycle/") {
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

// TestBackup_SFTP_InvalidCredentials confirms creation-time validation
// rejects a destination with bad credentials.
func TestBackup_SFTP_InvalidCredentials(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("backup-sftp-badcreds"), "Password123!", "SFTP BadCreds")

	payload := newSFTPDestPayload("badcreds")
	payload["credentials"] = map[string]interface{}{
		"username": "wronguser",
		"password": "wrongpass",
	}
	createResp := c.POST("/backups/destinations", payload)
	if createResp.StatusCode != http.StatusUnauthorized && createResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 401/400 on bad credentials, got %d: %s", createResp.StatusCode, string(createResp.Body))
	}
}

// TestBackup_SFTP_RequiresHostKeyOrAcceptAny verifies the provider rejects a
// destination missing both host_key_fingerprint and accept_any_host_key — a
// security regression guard.
func TestBackup_SFTP_RequiresHostKeyOrAcceptAny(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("backup-sftp-nohostkey"), "Password123!", "SFTP NoHostKey")

	payload := newSFTPDestPayload("nohostkey")
	cfg := payload["config"].(map[string]interface{})
	delete(cfg, "accept_any_host_key")
	// host_key_fingerprint was never set -> creation must fail.
	createResp := c.POST("/backups/destinations", payload)
	if createResp.StatusCode == http.StatusCreated {
		t.Fatalf("expected non-201 when host key not provided and accept_any_host_key not set, got 201")
	}
}

// TestBackup_SFTP_RejectsBadHostKeyFingerprint confirms host-key verification
// fails when the supplied fingerprint doesn't match the server.
func TestBackup_SFTP_RejectsBadHostKeyFingerprint(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("backup-sftp-badhostkey"), "Password123!", "SFTP BadHostKey")

	payload := newSFTPDestPayload("badhostkey")
	cfg := payload["config"].(map[string]interface{})
	delete(cfg, "accept_any_host_key")
	cfg["host_key_fingerprint"] = "SHA256:thisisnotthecorrectfingerprintforthetestserver"

	createResp := c.POST("/backups/destinations", payload)
	if createResp.StatusCode == http.StatusCreated {
		t.Fatalf("expected non-201 for mismatched host key, got 201")
	}
}

// TestBackup_SFTP_UserIsolation ensures one user cannot see or delete another
// user's SFTP destination.
func TestBackup_SFTP_UserIsolation(t *testing.T) {
	cA := NewE2EClient(t)
	registerAndLogin(t, cA, uniqueEmail("backup-sftp-userA"), "Password123!", "User A")
	createResp := cA.POST("/backups/destinations", newSFTPDestPayload("userA"+time.Now().Format("150405")))
	requireStatus(t, createResp, http.StatusCreated)
	var dest backupDestination
	createResp.JSON(&dest)

	cB := NewE2EClient(t)
	registerAndLogin(t, cB, uniqueEmail("backup-sftp-userB"), "Password123!", "User B")
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
