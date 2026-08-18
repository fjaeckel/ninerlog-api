package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// Asserts logout revokes the refresh token server-side.
func TestLogout_RevokesRefreshToken(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/register",
		bytes.NewBufferString(`{"email":"logout@example.com","password":"Password1234!","name":"L"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.RegisterUser(c)

	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/login",
		bytes.NewBufferString(`{"email":"logout@example.com","password":"Password1234!"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.LoginUser(c)
	if w.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", w.Code, w.Body.String())
	}
	var login map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &login)
	refresh, _ := login["refreshToken"].(string)
	if refresh == "" {
		t.Fatal("no refresh token issued")
	}

	// The token works before logout.
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/refresh",
		bytes.NewBufferString(`{"refreshToken":"`+refresh+`"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.RefreshToken(c)
	if w.Code != http.StatusOK {
		t.Fatalf("refresh before logout: %d, want 200", w.Code)
	}
	var refreshed map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &refreshed)
	rotated, _ := refreshed["refreshToken"].(string)

	// Log out with the current (rotated) token.
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/logout",
		bytes.NewBufferString(`{"refreshToken":"`+rotated+`"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.LogoutUser(c)
	c.Writer.WriteHeaderNow()
	if w.Code != http.StatusNoContent {
		t.Fatalf("logout: %d, want 204", w.Code)
	}

	// It must no longer resurrect the session.
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/refresh",
		bytes.NewBufferString(`{"refreshToken":"`+rotated+`"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.RefreshToken(c)
	if w.Code == http.StatusOK {
		t.Error("SESSION RESURRECTED: refresh token still valid after logout")
	}
}

// Revocation is idempotent and must not reveal whether a token existed.
func TestLogout_UnknownTokenStillReturns204(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/logout",
		bytes.NewBufferString(`{"refreshToken":"not-a-real-token"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.LogoutUser(c)
	c.Writer.WriteHeaderNow()
	if w.Code != http.StatusNoContent {
		t.Errorf("unknown token: %d, want 204", w.Code)
	}
}
