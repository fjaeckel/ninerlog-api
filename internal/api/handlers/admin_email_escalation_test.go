package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/gin-gonic/gin"
)

// These tests assert admin status requires a verified ADMIN_EMAIL match.

func TestIsAdminUser_RequiresVerifiedEmail(t *testing.T) {
	h := &APIHandler{adminEmail: "admin@example.com"}

	unverified := &models.User{Email: "admin@example.com", EmailVerified: false}
	if h.isAdminUser(unverified) {
		t.Error("holder of ADMIN_EMAIL with an UNVERIFIED address must not be admin")
	}

	verified := &models.User{Email: "admin@example.com", EmailVerified: true}
	if !h.isAdminUser(verified) {
		t.Error("holder of ADMIN_EMAIL with a verified address should be admin")
	}
}

func TestUpdateCurrentUser_EmailChangeRequiresPassword(t *testing.T) {
	h, userRepo := setupTestHandler() // adminEmail = "admin@test.com"

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/register",
		bytes.NewBufferString(`{"email":"attacker@evil.com","password":"Password1234!","name":"A"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.RegisterUser(c)

	u, err := userRepo.GetByEmail(c.Request.Context(), "attacker@evil.com")
	if err != nil {
		t.Fatalf("registered user missing: %v", err)
	}

	// Attempt the escalation without supplying the current password.
	w = httptest.NewRecorder()
	c = authenticatedContext(w, u.ID)
	c.Request = httptest.NewRequest("PATCH", "/users/me",
		bytes.NewBufferString(`{"email":"admin@test.com"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.UpdateCurrentUser(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("email change without currentPassword: status = %d, want 400", w.Code)
	}
	after, _ := userRepo.GetByID(c.Request.Context(), u.ID)
	if after.Email != "attacker@evil.com" {
		t.Errorf("email changed despite missing password: %q", after.Email)
	}
}

func TestUpdateCurrentUser_EmailChangeClearsVerifiedAndDeniesAdmin(t *testing.T) {
	h, userRepo := setupTestHandler() // adminEmail = "admin@test.com"

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/register",
		bytes.NewBufferString(`{"email":"attacker2@evil.com","password":"Password1234!","name":"A"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.RegisterUser(c)

	u, err := userRepo.GetByEmail(c.Request.Context(), "attacker2@evil.com")
	if err != nil {
		t.Fatalf("registered user missing: %v", err)
	}

	// Supply the correct password — the change is allowed, but must not confer admin.
	w = httptest.NewRecorder()
	c = authenticatedContext(w, u.ID)
	c.Request = httptest.NewRequest("PATCH", "/users/me",
		bytes.NewBufferString(`{"email":"admin@test.com","currentPassword":"Password1234!"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.UpdateCurrentUser(c)

	if w.Code != http.StatusOK {
		t.Fatalf("authorized email change: status = %d, want 200 (body %s)", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["isAdmin"] == true {
		t.Error("ESCALATION: taking ADMIN_EMAIL granted admin rights")
	}
	if resp["emailVerified"] == true {
		t.Error("emailVerified must be cleared when the address changes")
	}

	// And the admin gate itself must reject the caller.
	w = httptest.NewRecorder()
	c = authenticatedContext(w, u.ID)
	c.Request = httptest.NewRequest("GET", "/admin/users", nil)
	if _, ok := h.requireAdmin(c); ok {
		t.Error("ESCALATION: requireAdmin accepted a user with an unverified ADMIN_EMAIL")
	}
}

// A malformed-but-parseable address (quoted local-part that re-emits with a
// raw backslash) must be rejected: it cannot round-trip through
// openapi_types.Email.
func TestUpdateCurrentUser_RejectsNonRoundTrippableEmail(t *testing.T) {
	h, userRepo := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/register",
		bytes.NewBufferString(`{"email":"bs@example.com","password":"Password1234!","name":"B"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.RegisterUser(c)
	u, _ := userRepo.GetByEmail(c.Request.Context(), "bs@example.com")

	w = httptest.NewRecorder()
	c = authenticatedContext(w, u.ID)
	body, _ := json.Marshal(map[string]string{
		"email":           `"back\\slash"@evil.test`,
		"currentPassword": "Password1234!",
	})
	c.Request = httptest.NewRequest("PATCH", "/users/me", bytes.NewBuffer(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.UpdateCurrentUser(c)

	if w.Code == http.StatusOK {
		after, _ := userRepo.GetByID(c.Request.Context(), u.ID)
		t.Errorf("non-round-trippable address accepted, stored as %q", after.Email)
	}
}
