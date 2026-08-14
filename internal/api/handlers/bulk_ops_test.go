package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// The JSON restore inserts rows in a loop with no cap. The only bound was the
// 50 MB body limit, so one request could drive an unbounded number of inserts —
// and since the loop is not transactional, a failure partway (bad row, or the
// request/statement timeout firing) left a partially-imported account with no
// rollback while the summary reported only what landed first. Refusing an
// oversized backup up front avoids half-applying it.
func TestImportDataJSON_RejectsOversizedBackup(t *testing.T) {
	h, userRepo := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/register",
		bytes.NewBufferString(`{"email":"restore@example.com","password":"Password1234!","name":"R"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.RegisterUser(c)
	u, err := userRepo.GetByEmail(c.Request.Context(), "restore@example.com")
	if err != nil {
		t.Fatalf("registered user missing: %v", err)
	}

	flights := make([]map[string]any, maxRestoreFlights+1)
	for i := range flights {
		flights[i] = map[string]any{"date": "2026-01-01T00:00:00Z", "aircraftReg": "D-ABCD"}
	}
	payload, _ := json.Marshal(map[string]any{
		"format":  "NinerLog JSON Backup",
		"version": "1",
		"flights": flights,
	})

	w = httptest.NewRecorder()
	c = authenticatedContext(w, u.ID)
	c.Request = httptest.NewRequest("POST", "/imports/json", bytes.NewBuffer(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	h.ImportDataJSON(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("oversized backup: status = %d, want 400", w.Code)
	}
	if body := w.Body.String(); !bytes.Contains([]byte(body), []byte("too many flights")) {
		t.Errorf("expected a flight-count error, got %s", body)
	}
}

func TestImportDataJSON_AcceptsBackupWithinCaps(t *testing.T) {
	h, userRepo := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/register",
		bytes.NewBufferString(`{"email":"restore2@example.com","password":"Password1234!","name":"R"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.RegisterUser(c)
	u, _ := userRepo.GetByEmail(c.Request.Context(), "restore2@example.com")

	payload := []byte(`{"format":"NinerLog JSON Backup","version":"1","flights":[],"aircraft":[],"licenses":[],"credentials":[]}`)
	w = httptest.NewRecorder()
	c = authenticatedContext(w, u.ID)
	c.Request = httptest.NewRequest("POST", "/imports/json", bytes.NewBuffer(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	h.ImportDataJSON(c)

	if w.Code == http.StatusBadRequest {
		t.Errorf("an empty in-range backup was rejected: %s", w.Body.String())
	}
}

// Each entity list has its own ceiling, so a backup can't smuggle a huge
// aircraft or credential list past the flight cap.
func TestImportDataJSON_CapsEachEntityList(t *testing.T) {
	h, userRepo := setupTestHandler()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/register",
		bytes.NewBufferString(`{"email":"restore3@example.com","password":"Password1234!","name":"R"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.RegisterUser(c)
	u, _ := userRepo.GetByEmail(c.Request.Context(), "restore3@example.com")

	for _, field := range []string{"aircraft", "credentials"} {
		items := make([]map[string]any, maxRestoreEntities+1)
		for i := range items {
			items[i] = map[string]any{"registration": fmt.Sprintf("D-%04d", i)}
		}
		payload, _ := json.Marshal(map[string]any{
			"format": "NinerLog JSON Backup", "version": "1", field: items,
		})
		w = httptest.NewRecorder()
		c = authenticatedContext(w, u.ID)
		c.Request = httptest.NewRequest("POST", "/imports/json", bytes.NewBuffer(payload))
		c.Request.Header.Set("Content-Type", "application/json")
		h.ImportDataJSON(c)
		if w.Code != http.StatusBadRequest {
			t.Errorf("oversized %s list: status = %d, want 400", field, w.Code)
		}
	}
}
