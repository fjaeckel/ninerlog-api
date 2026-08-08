package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// OIDC mode is a mode switch, not an extra login button: when an identity
// provider owns accounts, every local credential path has to be closed. These
// tests walk each of those endpoints and assert it refuses, because a single
// one left reachable is a way around the provider's own policy — its MFA
// requirements, its lockouts, its offboarding.

// oidcTestHandler returns a handler wired for OIDC mode. The service is
// constructed with a real config but is never asked to reach the provider:
// every endpoint under test refuses before any network call.
func oidcTestHandler(t *testing.T) (*APIHandler, *mockUserRepo) {
	t.Helper()
	h, userRepo := setupTestHandler(t)

	svc, err := service.NewOIDCService(service.OIDCConfig{
		Issuer:            "https://idp.example.com",
		ClientID:          "ninerlog",
		ClientSecret:      "secret",
		RedirectURL:       "https://logbook.example/api/v1/auth/oidc/callback",
		PostLoginRedirect: "https://logbook.example/auth/callback",
		ProviderName:      "Test IdP",
		Scopes:            []string{"openid", "email"},
	}, userRepo, stubOIDCIdentityRepo{}, h.authService)
	if err != nil {
		t.Fatalf("NewOIDCService: %v", err)
	}
	h.oidcService = svc
	return h, userRepo
}

// stubOIDCIdentityRepo satisfies the repository interface for tests that never
// reach the provider — every endpoint here refuses before any state is touched.
type stubOIDCIdentityRepo struct{}

func (stubOIDCIdentityRepo) GetBySubject(context.Context, string, string) (*models.OIDCIdentity, error) {
	return nil, repository.ErrNotFound
}

func (stubOIDCIdentityRepo) GetByUserID(context.Context, uuid.UUID) (*models.OIDCIdentity, error) {
	return nil, repository.ErrNotFound
}
func (stubOIDCIdentityRepo) Create(context.Context, *models.OIDCIdentity) error { return nil }
func (stubOIDCIdentityRepo) TouchLogin(context.Context, uuid.UUID, string, time.Time) error {
	return nil
}
func (stubOIDCIdentityRepo) CreateLoginState(context.Context, *models.OIDCLoginState) error {
	return nil
}

func (stubOIDCIdentityRepo) ConsumeLoginState(context.Context, []byte) (*models.OIDCLoginState, error) {
	return nil, repository.ErrNotFound
}
func (stubOIDCIdentityRepo) CreateHandoffCode(context.Context, *models.OIDCHandoffCode) error {
	return nil
}

func (stubOIDCIdentityRepo) ConsumeHandoffCode(context.Context, []byte) (*models.OIDCHandoffCode, error) {
	return nil, repository.ErrNotFound
}
func (stubOIDCIdentityRepo) DeleteExpired(context.Context) (int64, error) { return 0, nil }

func jsonRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestOIDCMode_LocalCredentialEndpointsAreClosed(t *testing.T) {
	userID := uuid.New()
	cases := []struct {
		name   string
		body   string
		invoke func(*APIHandler, *gin.Context)
	}{
		{"register", `{"email":"a@b.com","password":"password1234","name":"A"}`,
			func(h *APIHandler, c *gin.Context) { h.RegisterUser(c) }},
		{"login", `{"email":"a@b.com","password":"password1234"}`,
			func(h *APIHandler, c *gin.Context) { h.LoginUser(c) }},
		{"verify email", `{"token":"x"}`,
			func(h *APIHandler, c *gin.Context) { h.VerifyEmail(c) }},
		{"resend verification", `{"email":"a@b.com"}`,
			func(h *APIHandler, c *gin.Context) { h.ResendVerificationEmail(c) }},
		{"password reset request", `{"email":"a@b.com"}`,
			func(h *APIHandler, c *gin.Context) { h.RequestPasswordReset(c) }},
		{"password reset", `{"token":"x","newPassword":"password1234"}`,
			func(h *APIHandler, c *gin.Context) { h.ResetPassword(c) }},
		{"change password", `{"currentPassword":"a","newPassword":"password1234"}`,
			func(h *APIHandler, c *gin.Context) { h.ChangePassword(c) }},
		{"2fa setup", `{}`,
			func(h *APIHandler, c *gin.Context) { h.Setup2FA(c) }},
		{"2fa verify", `{"code":"123456"}`,
			func(h *APIHandler, c *gin.Context) { h.Verify2FA(c) }},
		{"2fa disable", `{"password":"password1234"}`,
			func(h *APIHandler, c *gin.Context) { h.Disable2FA(c) }},
		{"2fa login", `{"twoFactorToken":"x","code":"123456"}`,
			func(h *APIHandler, c *gin.Context) { h.Login2FA(c) }},
		{"passkey registration", `{}`,
			func(h *APIHandler, c *gin.Context) { h.WebauthnRegisterOptions(c) }},
		{"passkey login", `{}`,
			func(h *APIHandler, c *gin.Context) { h.WebauthnLoginOptions(c) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := oidcTestHandler(t)
			w := httptest.NewRecorder()
			c := authenticatedContext(w, userID)
			c.Request = jsonRequest("POST", "/api/v1/auth/x", tc.body)

			tc.invoke(h, c)

			if w.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503 — this endpoint is still reachable in OIDC mode", w.Code)
			}
		})
	}
}

func TestOIDCMode_LocalCredentialEndpointsStayOpenInLocalMode(t *testing.T) {
	// The mirror image: the gate must not fire on a normal deployment.
	h, _ := setupTestHandler(t)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = jsonRequest("POST", "/api/v1/auth/register",
		`{"email":"local@example.com","password":"password1234","name":"Local"}`)

	h.RegisterUser(c)

	if w.Code == http.StatusServiceUnavailable {
		t.Fatal("registration must work when no identity provider is configured")
	}
}

func TestOIDCMode_ProfileIdentityFieldsAreProviderOwned(t *testing.T) {
	h, userRepo := oidcTestHandler(t)
	user := &models.User{Email: "pilot@example.com", Name: "Amelia"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	for _, body := range []string{`{"name":"New Name"}`, `{"email":"other@example.com"}`} {
		w := httptest.NewRecorder()
		c := authenticatedContext(w, user.ID)
		c.Request = jsonRequest("PATCH", "/api/v1/users/me", body)

		h.UpdateCurrentUser(c)

		if w.Code != http.StatusForbidden {
			t.Errorf("body %s: status = %d, want 403", body, w.Code)
		}
	}

	// Display preferences remain the user's own.
	w := httptest.NewRecorder()
	c := authenticatedContext(w, user.ID)
	c.Request = jsonRequest("PATCH", "/api/v1/users/me", `{"timeDisplayFormat":"decimal"}`)

	h.UpdateCurrentUser(c)

	if w.Code != http.StatusOK {
		t.Errorf("preference update: status = %d, want 200", w.Code)
	}
}

func TestOIDCMode_AccountDeletionConfirmsWithEmail(t *testing.T) {
	h, userRepo := oidcTestHandler(t)
	user := &models.User{Email: "pilot@example.com", Name: "Amelia"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	// A password is meaningless here and must not delete the account.
	w := httptest.NewRecorder()
	c := authenticatedContext(w, user.ID)
	c.Request = jsonRequest("DELETE", "/api/v1/users/me", `{"password":"anything"}`)
	h.DeleteCurrentUser(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("password-only deletion: status = %d, want 400", w.Code)
	}

	// So must the wrong address.
	w = httptest.NewRecorder()
	c = authenticatedContext(w, user.ID)
	c.Request = jsonRequest("DELETE", "/api/v1/users/me", `{"confirmEmail":"someone@else.com"}`)
	h.DeleteCurrentUser(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("wrong confirmEmail: status = %d, want 400", w.Code)
	}
	if _, err := userRepo.GetByID(context.Background(), user.ID); err != nil {
		t.Fatal("account was deleted without a valid confirmation")
	}

	// The account's own address, case-insensitively, is the confirmation.
	w = httptest.NewRecorder()
	c = authenticatedContext(w, user.ID)
	c.Request = jsonRequest("DELETE", "/api/v1/users/me", `{"confirmEmail":"PILOT@example.com"}`)
	h.DeleteCurrentUser(c)
	c.Writer.WriteHeaderNow()
	if w.Code != http.StatusNoContent {
		t.Fatalf("valid confirmEmail: status = %d, want 204", w.Code)
	}
	if _, err := userRepo.GetByID(context.Background(), user.ID); err == nil {
		t.Error("account should have been deleted")
	}
}

func TestAuthProviders_ReportsLocalModeByDefault(t *testing.T) {
	h, _ := setupTestHandler(t)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/auth/providers", nil)

	h.GetAuthProviders(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body struct {
		Mode                 string `json:"mode"`
		PasswordLoginEnabled bool   `json:"passwordLoginEnabled"`
		RegistrationEnabled  bool   `json:"registrationEnabled"`
		TwoFactorEnabled     bool   `json:"twoFactorEnabled"`
		WebauthnEnabled      bool   `json:"webauthnEnabled"`
		OIDC                 struct {
			Enabled bool `json:"enabled"`
		} `json:"oidc"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Mode != "local" || !body.PasswordLoginEnabled || !body.RegistrationEnabled || body.OIDC.Enabled {
		t.Errorf("unexpected local-mode capabilities: %+v", body)
	}
	if body.WebauthnEnabled {
		t.Error("passkeys should be reported off when WEBAUTHN_RP_ID is unset")
	}
}

func TestAuthProviders_ReportsOIDCMode(t *testing.T) {
	h, _ := oidcTestHandler(t)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/auth/providers", nil)

	h.GetAuthProviders(c)

	var body struct {
		Mode                 string `json:"mode"`
		PasswordLoginEnabled bool   `json:"passwordLoginEnabled"`
		RegistrationEnabled  bool   `json:"registrationEnabled"`
		TwoFactorEnabled     bool   `json:"twoFactorEnabled"`
		WebauthnEnabled      bool   `json:"webauthnEnabled"`
		OIDC                 struct {
			Enabled      bool   `json:"enabled"`
			Name         string `json:"name"`
			AuthorizeURL string `json:"authorizeUrl"`
		} `json:"oidc"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Mode != "oidc" {
		t.Errorf("mode = %q, want oidc", body.Mode)
	}
	if body.PasswordLoginEnabled || body.RegistrationEnabled || body.TwoFactorEnabled || body.WebauthnEnabled {
		t.Errorf("no local method may be advertised in OIDC mode: %+v", body)
	}
	if !body.OIDC.Enabled || body.OIDC.Name != "Test IdP" || body.OIDC.AuthorizeURL == "" {
		t.Errorf("incomplete OIDC advertisement: %+v", body.OIDC)
	}
	// The client secret must never surface on an unauthenticated endpoint.
	if bytes.Contains(w.Body.Bytes(), []byte("secret")) {
		t.Error("response leaked the client secret")
	}
}

func TestOIDCEndpointsReport503WhenNotConfigured(t *testing.T) {
	h, _ := setupTestHandler(t)
	for _, tc := range []struct {
		name   string
		invoke func(*gin.Context)
	}{
		{"authorize", h.OIDCAuthorize},
		{"callback", h.OIDCCallback},
		{"exchange", h.ExchangeOidcCode},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = jsonRequest("GET", "/api/v1/auth/oidc/"+tc.name, `{"code":"x"}`)

			tc.invoke(c)

			if w.Code != http.StatusServiceUnavailable {
				t.Errorf("status = %d, want 503", w.Code)
			}
		})
	}
}
