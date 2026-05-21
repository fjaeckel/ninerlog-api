//go:build e2e

package e2e_test

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// MinIO container in docker-compose.e2e.yaml exposes:
//   endpoint:   http://minio:9000 (inside docker) / http://localhost:9000 (host)
//   bucket:     ninerlog-backups (pre-created by minio-init)
//   creds:      minioadmin / minioadmin
//
// The API talks to MinIO via its docker network alias, so when constructing a
// backup destination we use the docker hostname.
func minioEndpoint() string {
	if v := os.Getenv("E2E_MINIO_ENDPOINT"); v != "" {
		return v
	}
	return "http://minio:9000"
}

type backupProvider struct {
	Name             string `json:"name"`
	DisplayName      string `json:"displayName"`
	Description      string `json:"description"`
	ConfigSchema     struct {
		Fields []struct {
			Name     string `json:"name"`
			Type     string `json:"type"`
			Required bool   `json:"required"`
		} `json:"fields"`
	} `json:"configSchema"`
	CredentialSchema struct {
		Fields []struct {
			Name     string `json:"name"`
			Type     string `json:"type"`
			Required bool   `json:"required"`
		} `json:"fields"`
	} `json:"credentialSchema"`
}

type backupDestination struct {
	ID                  string                 `json:"id"`
	UserID              string                 `json:"userId"`
	Provider            string                 `json:"provider"`
	DisplayName         string                 `json:"displayName"`
	Config              map[string]interface{} `json:"config"`
	CredentialHint      string                 `json:"credentialHint"`
	Schedule            string                 `json:"schedule"`
	ScheduleHourUtc     int                    `json:"scheduleHourUtc"`
	RetentionCount      int                    `json:"retentionCount"`
	Status              string                 `json:"status"`
	Enabled             bool                   `json:"enabled"`
	ConsecutiveFailures int                    `json:"consecutiveFailures"`
	LastError           string                 `json:"lastError"`
}

type backupRun struct {
	ID            string `json:"id"`
	DestinationID string `json:"destinationId"`
	Status        string `json:"status"`
	Trigger       string `json:"trigger"`
	SizeBytes     int64  `json:"sizeBytes"`
	RemotePath    string `json:"remotePath"`
	ErrorMessage  string `json:"errorMessage"`
}

type backupTestResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func newBackupDestPayload(name string) map[string]interface{} {
	return map[string]interface{}{
		"provider":    "s3",
		"displayName": name,
		"config": map[string]interface{}{
			"endpoint": minioEndpoint(),
			"region":   "us-east-1",
			"bucket":   "ninerlog-backups",
			"prefix":   "e2e/" + name + "/",
		},
		"credentials": map[string]interface{}{
			"access_key_id":     "minioadmin",
			"secret_access_key": "minioadmin",
		},
		"schedule":        "manual",
		"scheduleHourUtc": 3,
		"retentionCount":  10,
		"enabled":         true,
	}
}

// TestBackup_ListProviders confirms the S3 provider is registered and its
// schema is exposed for the frontend dynamic form.
func TestBackup_ListProviders(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("backup-providers"), "Password123!", "Backup Tester")

	resp := c.GET("/backups/providers")
	requireStatus(t, resp, http.StatusOK)
	var providers []backupProvider
	if err := resp.JSON(&providers); err != nil {
		t.Fatalf("decode providers: %v", err)
	}
	if len(providers) == 0 {
		t.Fatal("expected at least one provider, got 0")
	}
	var s3p *backupProvider
	for i := range providers {
		if providers[i].Name == "s3" {
			s3p = &providers[i]
			break
		}
	}
	if s3p == nil {
		t.Fatalf("s3 provider not registered; got %+v", providers)
	}
	if len(s3p.ConfigSchema.Fields) == 0 || len(s3p.CredentialSchema.Fields) == 0 {
		t.Fatalf("s3 provider schemas are empty: %+v", s3p)
	}
}

// TestBackup_FullLifecycle exercises create → test → run → list runs →
// get destination → delete on the real MinIO bucket.
func TestBackup_FullLifecycle(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("backup-lifecycle"), "Password123!", "Backup Lifecycle")

	// Create.
	createResp := c.POST("/backups/destinations", newBackupDestPayload("lifecycle"))
	requireStatus(t, createResp, http.StatusCreated)
	var dest backupDestination
	if err := createResp.JSON(&dest); err != nil {
		t.Fatalf("decode destination: %v", err)
	}
	if dest.ID == "" || dest.Provider != "s3" {
		t.Fatalf("unexpected created destination: %+v", dest)
	}
	if dest.CredentialHint == "" {
		t.Errorf("expected credentialHint to be populated, got empty")
	}

	// Test connection (will exercise MinIO).
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
	if !strings.HasPrefix(run.RemotePath, "e2e/lifecycle/") {
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

	// Get after delete -> 404.
	getResp2 := c.GET("/backups/destinations/" + dest.ID)
	if getResp2.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", getResp2.StatusCode)
	}
}

// TestBackup_InvalidCredentialsFailRun confirms creation-time validation
// rejects a destination with bad credentials before any data is persisted.
func TestBackup_InvalidCredentialsFailRun(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("backup-badcreds"), "Password123!", "Backup BadCreds")

	payload := newBackupDestPayload("badcreds")
	payload["credentials"] = map[string]interface{}{
		"access_key_id":     "wronguser",
		"secret_access_key": "wrongpass",
	}
	createResp := c.POST("/backups/destinations", payload)
	if createResp.StatusCode != http.StatusUnauthorized && createResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 401/400 on bad credentials, got %d: %s", createResp.StatusCode, string(createResp.Body))
	}

	// And: creating with a non-existent bucket should also be rejected.
	payload2 := newBackupDestPayload("nobucket")
	cfg := payload2["config"].(map[string]interface{})
	cfg["bucket"] = "this-bucket-does-not-exist-" + time.Now().Format("150405")
	createResp2 := c.POST("/backups/destinations", payload2)
	if createResp2.StatusCode == http.StatusCreated {
		t.Fatalf("expected non-201 for non-existent bucket, got 201")
	}
}

// TestBackup_UserIsolation ensures one user cannot see, update, or run another
// user's backup destination.
func TestBackup_UserIsolation(t *testing.T) {
	// User A creates a destination.
	cA := NewE2EClient(t)
	registerAndLogin(t, cA, uniqueEmail("backup-userA"), "Password123!", "User A")
	createResp := cA.POST("/backups/destinations", newBackupDestPayload("userA"))
	requireStatus(t, createResp, http.StatusCreated)
	var dest backupDestination
	createResp.JSON(&dest)

	// User B should not see A's destination, nor be able to fetch or delete it.
	cB := NewE2EClient(t)
	registerAndLogin(t, cB, uniqueEmail("backup-userB"), "Password123!", "User B")
	listResp := cB.GET("/backups/destinations")
	requireStatus(t, listResp, http.StatusOK)
	if strings.Contains(string(listResp.Body), dest.ID) {
		t.Fatalf("User B sees User A's destination ID in list: %s", string(listResp.Body))
	}
	getResp := cB.GET("/backups/destinations/" + dest.ID)
	if getResp.StatusCode != http.StatusNotFound && getResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 404/403 cross-user read, got %d", getResp.StatusCode)
	}
	delResp := cB.DELETE("/backups/destinations/" + dest.ID)
	if delResp.StatusCode != http.StatusNotFound && delResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 404/403 cross-user delete, got %d", delResp.StatusCode)
	}

	// A can still delete its own.
	delA := cA.DELETE("/backups/destinations/" + dest.ID)
	requireStatus(t, delA, http.StatusNoContent)
}

// TestBackup_ServiceDisabledReturns503 only runs if the test environment
// explicitly disables backups; otherwise it is a no-op.
func TestBackup_ServiceDisabledReturns503(t *testing.T) {
	if os.Getenv("E2E_BACKUPS_DISABLED") != "true" {
		t.Skip("E2E_BACKUPS_DISABLED not set; skipping")
	}
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("backup-disabled"), "Password123!", "Backup Disabled")
	resp := c.GET("/backups/providers")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when backups disabled, got %d", resp.StatusCode)
	}
}

