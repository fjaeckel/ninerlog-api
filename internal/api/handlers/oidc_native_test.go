package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/gin-gonic/gin"
)

// A native login is marked in the state parameter, so the callback picks its
// redirect without consuming the pending login — including on the paths where
// there is nothing to consume.
func TestOIDCCallbackRedirectsNativeLoginToTheAppScheme(t *testing.T) {
	cases := []struct {
		name  string
		state string
		want  string
	}{
		{"native", "n.Zm9vYmFy", "ninerlog://auth/callback?oidc_error=provider_error"},
		{"web", "Zm9vYmFy", "https://logbook.example/auth/callback?oidc_error=provider_error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := oidcTestHandler(t)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet,
				"/api/v1/auth/oidc/callback?error=access_denied&state="+tc.state, nil)

			providerErr := "access_denied"
			h.OidcCallback(c, generated.OidcCallbackParams{
				Error: &providerErr,
				State: &tc.state,
			})

			if w.Code != http.StatusFound {
				t.Fatalf("status = %d, want 302", w.Code)
			}
			if got := w.Header().Get("Location"); got != tc.want {
				t.Errorf("Location = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAuthProvidersAdvertisesTheNativeFlow(t *testing.T) {
	h, _ := oidcTestHandler(t)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/providers", nil)

	h.GetAuthProviders(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body generated.AuthProviders
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Oidc.NativeAuthorizeUrl == nil || *body.Oidc.NativeAuthorizeUrl != "/api/v1/auth/oidc/authorize?native=1" {
		t.Errorf("nativeAuthorizeUrl = %v", body.Oidc.NativeAuthorizeUrl)
	}
	if body.Oidc.NativeRedirectUri == nil || *body.Oidc.NativeRedirectUri != "ninerlog://auth/callback" {
		t.Errorf("nativeRedirectUri = %v", body.Oidc.NativeRedirectUri)
	}
}

func TestAuthProvidersOmitsTheNativeFlowInLocalMode(t *testing.T) {
	h, _ := setupTestHandler()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/providers", nil)

	h.GetAuthProviders(c)

	var body generated.AuthProviders
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Oidc.NativeAuthorizeUrl != nil || body.Oidc.NativeRedirectUri != nil {
		t.Error("local mode must advertise no native OIDC flow")
	}
}
