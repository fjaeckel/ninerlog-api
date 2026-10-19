package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup"
	"github.com/fjaeckel/ninerlog-api/internal/testutil/restorefake"
	"github.com/google/uuid"
)

// restoreHandler returns a handler whose restores run against an in-memory
// store, and the id of a user to restore as.
func restoreHandler(t *testing.T) (*APIHandler, *restorefake.Store, uuid.UUID) {
	t.Helper()
	h, _ := setupTestHandler()
	store := restorefake.New()
	h.SetRestorer(cloudbackup.NewRestorer(store))
	return h, store, uuid.New()
}

func postRestore(h *APIHandler, userID uuid.UUID, body []byte) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("POST", "/imports/json", bytes.NewBuffer(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.ImportDataJSON(c, generated.ImportDataJSONParams{})
	return w
}

// An oversized JSON restore is refused up front.
func TestImportDataJSON_RejectsOversizedBackup(t *testing.T) {
	h, store, userID := restoreHandler(t)

	payload := `{"format":"NinerLog JSON Backup","version":"1","flights":[` +
		strings.Repeat(`{},`, cloudbackup.MaxRestoreFlights) + `{}]}`
	w := postRestore(h, userID, []byte(payload))

	if w.Code != http.StatusBadRequest {
		t.Errorf("oversized backup: status = %d, want 400", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, "too many flights") {
		t.Errorf("expected a flight-count error, got %s", body)
	}
	if store.Commits != 0 {
		t.Errorf("an oversized backup committed %d transaction(s)", store.Commits)
	}
}

func TestImportDataJSON_AcceptsBackupWithinCaps(t *testing.T) {
	h, store, userID := restoreHandler(t)

	w := postRestore(h, userID, []byte(`{"format":"NinerLog JSON Backup","version":"1","flights":[],"aircraft":[],"licenses":[],"credentials":[]}`))

	if w.Code != http.StatusOK {
		t.Errorf("an empty in-range backup was rejected: %d %s", w.Code, w.Body.String())
	}
	if store.Commits != 1 {
		t.Errorf("commits = %d, want 1", store.Commits)
	}
}

// Each entity list has its own ceiling, so a backup can't smuggle a huge
// aircraft or credential list past the flight cap.
func TestImportDataJSON_CapsEachEntityList(t *testing.T) {
	h, _, userID := restoreHandler(t)

	for _, field := range []string{"aircraft", "credentials"} {
		items := make([]map[string]any, cloudbackup.MaxRestoreEntities+1)
		for i := range items {
			items[i] = map[string]any{"registration": fmt.Sprintf("D-%04d", i)}
		}
		payload, _ := json.Marshal(map[string]any{
			"format": "NinerLog JSON Backup", "version": "1", field: items,
		})
		w := postRestore(h, userID, payload)
		if w.Code != http.StatusBadRequest {
			t.Errorf("oversized %s list: status = %d, want 400", field, w.Code)
		}
	}
}
