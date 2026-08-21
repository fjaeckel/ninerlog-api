package service_test

import (
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/service"
)

// setOIDCEnv applies a full, valid environment and then the caller's overrides.
// t.Setenv restores everything when the test ends.
func setOIDCEnv(t *testing.T, overrides map[string]string) {
	t.Helper()
	base := map[string]string{
		"OIDC_ISSUER":                     "https://idp.example.com",
		"OIDC_CLIENT_ID":                  "ninerlog",
		"OIDC_CLIENT_SECRET":              "s3cret",
		"OIDC_REDIRECT_URL":               "https://logbook.example/api/v1/auth/oidc/callback",
		"OIDC_POST_LOGIN_REDIRECT":        "https://logbook.example/auth/callback",
		"OIDC_NATIVE_POST_LOGIN_REDIRECT": "",
		"OIDC_SCOPES":                     "",
		"OIDC_PROVIDER_NAME":              "",
		"OIDC_NAME_CLAIM":                 "",
		"OIDC_LINK_BY_VERIFIED_EMAIL":     "",
		"OIDC_TRUST_EMAIL_VERIFIED":       "",
		"OIDC_LOGIN_STATE_TTL":            "",
		"OIDC_HANDOFF_TTL":                "",
	}
	for k, v := range overrides {
		base[k] = v
	}
	for k, v := range base {
		t.Setenv(k, v)
	}
}

func TestLoadOIDCConfigDisabledByDefault(t *testing.T) {
	setOIDCEnv(t, map[string]string{"OIDC_ISSUER": ""})

	cfg, err := service.LoadOIDCConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Enabled() {
		t.Error("OIDC must stay off unless OIDC_ISSUER is set")
	}
}

func TestLoadOIDCConfigDefaults(t *testing.T) {
	setOIDCEnv(t, nil)

	cfg, err := service.LoadOIDCConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Enabled() {
		t.Fatal("expected OIDC to be enabled")
	}
	if got := strings.Join(cfg.Scopes, " "); got != "openid profile email" {
		t.Errorf("scopes = %q, want the openid/profile/email default", got)
	}
	if cfg.NameClaim != "name" {
		t.Errorf("NameClaim = %q, want name", cfg.NameClaim)
	}
	if cfg.ProviderName == "" {
		t.Error("ProviderName should fall back to a usable button label")
	}
	if cfg.LinkByVerifiedEmail {
		t.Error("account linking must be opt-in")
	}
	if cfg.TrustEmailVerified {
		t.Error("trusting unasserted email verification must be opt-in")
	}
	if cfg.LoginStateTTL != service.DefaultOIDCLoginStateTTL {
		t.Errorf("LoginStateTTL = %v, want the default", cfg.LoginStateTTL)
	}
	if cfg.HandoffTTL != service.DefaultOIDCHandoffTTL {
		t.Errorf("HandoffTTL = %v, want the default", cfg.HandoffTTL)
	}
}

func TestLoadOIDCConfigRejectsIncompleteSetup(t *testing.T) {
	cases := []struct {
		name      string
		overrides map[string]string
		wantIn    string
	}{
		{"no client id", map[string]string{"OIDC_CLIENT_ID": ""}, "OIDC_CLIENT_ID"},
		{"no client secret", map[string]string{"OIDC_CLIENT_SECRET": ""}, "OIDC_CLIENT_SECRET"},
		{"no redirect url", map[string]string{"OIDC_REDIRECT_URL": ""}, "OIDC_REDIRECT_URL"},
		{"no post-login redirect", map[string]string{"OIDC_POST_LOGIN_REDIRECT": ""}, "OIDC_POST_LOGIN_REDIRECT"},
		{"relative redirect url", map[string]string{"OIDC_REDIRECT_URL": "/auth/callback"}, "OIDC_REDIRECT_URL"},
		{"scheme-less issuer", map[string]string{"OIDC_ISSUER": "idp.example.com"}, "OIDC_ISSUER"},
		{"non-http issuer", map[string]string{"OIDC_ISSUER": "ldap://idp.example.com"}, "OIDC_ISSUER"},
		{"issuer with query", map[string]string{"OIDC_ISSUER": "https://idp.example.com/?realm=x"}, "OIDC_ISSUER"},
		{"post-login redirect not absolute", map[string]string{"OIDC_POST_LOGIN_REDIRECT": "/callback"}, "OIDC_POST_LOGIN_REDIRECT"},
		{"bad state ttl", map[string]string{"OIDC_LOGIN_STATE_TTL": "ten minutes"}, "OIDC_LOGIN_STATE_TTL"},
		{"negative handoff ttl", map[string]string{"OIDC_HANDOFF_TTL": "-30s"}, "OIDC_HANDOFF_TTL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setOIDCEnv(t, tc.overrides)

			_, err := service.LoadOIDCConfig()
			if err == nil {
				t.Fatal("expected a startup error, got none")
			}
			if !strings.Contains(err.Error(), tc.wantIn) {
				t.Errorf("error %q does not mention %q", err, tc.wantIn)
			}
		})
	}
}

func TestLoadOIDCConfigScopesAlwaysIncludeOpenID(t *testing.T) {
	setOIDCEnv(t, map[string]string{"OIDC_SCOPES": "email, profile,groups"})

	cfg, err := service.LoadOIDCConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Scopes[0] != "openid" {
		t.Errorf("scopes = %v, want openid first", cfg.Scopes)
	}
	if len(cfg.Scopes) != 4 {
		t.Errorf("scopes = %v, want four entries", cfg.Scopes)
	}
}

func TestLoadOIDCConfigBooleanSpellings(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want bool
	}{
		{"true", true}, {"TRUE", true}, {"1", true}, {"yes", true}, {"on", true},
		{"false", false}, {"0", false}, {"", false}, {"maybe", false},
	} {
		t.Run("value="+tc.raw, func(t *testing.T) {
			setOIDCEnv(t, map[string]string{"OIDC_LINK_BY_VERIFIED_EMAIL": tc.raw})

			cfg, err := service.LoadOIDCConfig()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.LinkByVerifiedEmail != tc.want {
				t.Errorf("LinkByVerifiedEmail = %v, want %v", cfg.LinkByVerifiedEmail, tc.want)
			}
		})
	}
}

func TestLoadOIDCConfigCustomTTLs(t *testing.T) {
	setOIDCEnv(t, map[string]string{
		"OIDC_LOGIN_STATE_TTL": "3m",
		"OIDC_HANDOFF_TTL":     "20s",
	})

	cfg, err := service.LoadOIDCConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LoginStateTTL != 3*time.Minute {
		t.Errorf("LoginStateTTL = %v, want 3m", cfg.LoginStateTTL)
	}
	if cfg.HandoffTTL != 20*time.Second {
		t.Errorf("HandoffTTL = %v, want 20s", cfg.HandoffTTL)
	}
}

func TestOIDCCookieScoping(t *testing.T) {
	// The cookie is scoped to the callback's own directory (any reverse-proxy
	// prefix included), and Secure follows the callback URL's scheme.
	for _, tc := range []struct {
		redirect   string
		wantPath   string
		wantSecure bool
	}{
		{"https://api.example.com/api/v1/auth/oidc/callback", "/api/v1/auth/oidc", true},
		{"https://example.com/backend/api/v1/auth/oidc/callback", "/backend/api/v1/auth/oidc", true},
		{"http://localhost:3000/api/v1/auth/oidc/callback", "/api/v1/auth/oidc", false},
		{"https://example.com/callback", "/", true},
	} {
		t.Run(tc.redirect, func(t *testing.T) {
			cfg := service.OIDCConfig{RedirectURL: tc.redirect}
			if got := cfg.CookiePath(); got != tc.wantPath {
				t.Errorf("CookiePath() = %q, want %q", got, tc.wantPath)
			}
			if got := cfg.CookieSecure(); got != tc.wantSecure {
				t.Errorf("CookieSecure() = %v, want %v", got, tc.wantSecure)
			}
		})
	}
}

func TestLoadOIDCConfigNativeRedirectDefaults(t *testing.T) {
	setOIDCEnv(t, nil)

	cfg, err := service.LoadOIDCConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.NativePostLoginRedirect != service.DefaultOIDCNativePostLoginRedirect {
		t.Errorf("native redirect = %q, want %q",
			cfg.NativePostLoginRedirect, service.DefaultOIDCNativePostLoginRedirect)
	}
}

func TestLoadOIDCConfigNativeRedirectAcceptsCustomScheme(t *testing.T) {
	setOIDCEnv(t, map[string]string{"OIDC_NATIVE_POST_LOGIN_REDIRECT": "fleetbook://sso/done"})

	cfg, err := service.LoadOIDCConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.NativePostLoginRedirect != "fleetbook://sso/done" {
		t.Errorf("native redirect = %q, want the configured value", cfg.NativePostLoginRedirect)
	}
}

func TestLoadOIDCConfigRejectsUnusableNativeRedirect(t *testing.T) {
	cases := map[string]string{
		"no scheme":     "auth/callback",
		"script scheme": "javascript:alert(1)",
		"no host":       "https://",
		"scheme only":   "ninerlog:",
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			setOIDCEnv(t, map[string]string{"OIDC_NATIVE_POST_LOGIN_REDIRECT": value})

			_, err := service.LoadOIDCConfig()
			if err == nil {
				t.Fatalf("expected %q to be refused at startup", value)
			}
			if !strings.Contains(err.Error(), "OIDC_NATIVE_POST_LOGIN_REDIRECT") {
				t.Errorf("error must name the offending variable, got %v", err)
			}
		})
	}
}
