package service

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

// OIDC login mode.
//
// NinerLog runs in exactly one authentication mode. Setting OIDC_ISSUER
// switches the deployment to OIDC and turns off every local credential path —
// passwords, registration, password reset, TOTP and passkeys — because in that
// mode the identity provider owns accounts entirely and a surviving local
// credential would be a way around it.
const (
	// DefaultOIDCLoginStateTTL bounds how long a pending authorization request
	// stays usable. Generous enough to type a password and pass MFA at the
	// provider, short enough that abandoned logins age out quickly.
	DefaultOIDCLoginStateTTL = 10 * time.Minute

	// DefaultOIDCHandoffTTL bounds the window between the provider redirecting
	// the browser back and the SPA exchanging its code for tokens. This is a
	// single automatic request, so it needs seconds, not minutes.
	DefaultOIDCHandoffTTL = 60 * time.Second
)

// OIDCConfig is the operator-supplied configuration for OIDC mode. It is built
// exclusively from environment variables; there is no per-user or database
// configuration for the provider.
type OIDCConfig struct {
	// Issuer is the provider's issuer URL. Its presence enables OIDC mode.
	Issuer string
	// ClientID and ClientSecret identify NinerLog to the provider.
	ClientID     string
	ClientSecret string
	// RedirectURL is this API's own callback, exactly as registered with the
	// provider (…/api/v1/auth/oidc/callback).
	RedirectURL string
	// PostLoginRedirect is the frontend URL the browser is sent to after a
	// successful login. Fixed by configuration and never read from the
	// request, so the callback cannot be turned into an open redirect.
	PostLoginRedirect string
	// Scopes requested at the provider. "openid" is always included.
	Scopes []string
	// ProviderName is the label the sign-in button shows ("Sign in with …").
	ProviderName string
	// NameClaim is the claim used as the pilot's display name.
	NameClaim string
	// LinkByVerifiedEmail lets a first OIDC login adopt a pre-existing local
	// account with the same address, but only when the ID token asserts
	// email_verified. Off by default: with an IdP where users choose their own
	// address this is an account-takeover path.
	LinkByVerifiedEmail bool
	// TrustEmailVerified marks provisioned users as email-verified even when
	// the ID token carries no email_verified claim. Needed for providers that
	// omit it — and required for ADMIN_EMAIL to take effect, since admin
	// status demands a verified address.
	TrustEmailVerified bool
	// LoginStateTTL and HandoffTTL bound the two single-use artefacts of a
	// login round trip.
	LoginStateTTL time.Duration
	HandoffTTL    time.Duration
}

// Enabled reports whether OIDC mode is switched on.
func (c OIDCConfig) Enabled() bool { return c.Issuer != "" }

// CookiePath scopes the login-state cookie to the OIDC endpoints.
//
// It is derived from the configured callback rather than hard-coded, because
// the browser matches the cookie against the path *it* requested. A deployment
// that mounts the API under a prefix (a reverse proxy serving NinerLog at
// /backend, say) would otherwise store the cookie at a path the callback never
// matches, and every login would fail with an unexplained invalid state.
func (c OIDCConfig) CookiePath() string {
	u, err := url.Parse(c.RedirectURL)
	if err != nil || u.Path == "" {
		return "/"
	}
	dir := path.Dir(u.Path)
	if dir == "." || dir == "" {
		return "/"
	}
	return dir
}

// CookieSecure reports whether the login-state cookie must carry the Secure
// attribute. Derived from the configured callback URL, not from the inbound
// request: behind a TLS-terminating proxy the request reaching Go is plain
// http, but the browser is on https and would silently drop a non-Secure
// cookie set from a Secure context in modern browsers. Local http development
// must conversely not set Secure, or the cookie is dropped there instead.
func (c OIDCConfig) CookieSecure() bool {
	return strings.HasPrefix(strings.ToLower(c.RedirectURL), "https://")
}

// ErrOIDCNotConfigured is returned by every OIDC operation on a deployment
// that has not set OIDC_ISSUER.
var ErrOIDCNotConfigured = errors.New("oidc is not configured")

// LoadOIDCConfig reads OIDC configuration from the environment. It returns a
// disabled config (and no error) when OIDC_ISSUER is unset. When the issuer is
// set, every other mandatory value must be present and well-formed: a
// half-configured identity provider must stop startup rather than leave a
// deployment that can neither log in locally nor via OIDC.
func LoadOIDCConfig() (OIDCConfig, error) {
	cfg := OIDCConfig{
		Issuer:              strings.TrimSpace(os.Getenv("OIDC_ISSUER")),
		ClientID:            strings.TrimSpace(os.Getenv("OIDC_CLIENT_ID")),
		ClientSecret:        os.Getenv("OIDC_CLIENT_SECRET"),
		RedirectURL:         strings.TrimSpace(os.Getenv("OIDC_REDIRECT_URL")),
		PostLoginRedirect:   strings.TrimSpace(os.Getenv("OIDC_POST_LOGIN_REDIRECT")),
		ProviderName:        strings.TrimSpace(os.Getenv("OIDC_PROVIDER_NAME")),
		NameClaim:           strings.TrimSpace(os.Getenv("OIDC_NAME_CLAIM")),
		LinkByVerifiedEmail: envBool("OIDC_LINK_BY_VERIFIED_EMAIL"),
		TrustEmailVerified:  envBool("OIDC_TRUST_EMAIL_VERIFIED"),
		LoginStateTTL:       DefaultOIDCLoginStateTTL,
		HandoffTTL:          DefaultOIDCHandoffTTL,
	}
	if !cfg.Enabled() {
		return cfg, nil
	}

	if cfg.ClientID == "" {
		return cfg, errors.New("OIDC_CLIENT_ID is required when OIDC_ISSUER is set")
	}
	if cfg.ClientSecret == "" {
		return cfg, errors.New("OIDC_CLIENT_SECRET is required when OIDC_ISSUER is set")
	}
	if err := requireAbsoluteURL("OIDC_ISSUER", cfg.Issuer); err != nil {
		return cfg, err
	}
	if cfg.RedirectURL == "" {
		return cfg, errors.New("OIDC_REDIRECT_URL is required when OIDC_ISSUER is set")
	}
	if err := requireAbsoluteURL("OIDC_REDIRECT_URL", cfg.RedirectURL); err != nil {
		return cfg, err
	}
	if cfg.PostLoginRedirect == "" {
		return cfg, errors.New("OIDC_POST_LOGIN_REDIRECT is required when OIDC_ISSUER is set")
	}
	if err := requireAbsoluteURL("OIDC_POST_LOGIN_REDIRECT", cfg.PostLoginRedirect); err != nil {
		return cfg, err
	}

	// The issuer must not carry a query or fragment: it is compared verbatim
	// against the `iss` claim, and a provider never issues one with either.
	if u, _ := url.Parse(cfg.Issuer); u != nil && (u.RawQuery != "" || u.Fragment != "") {
		return cfg, errors.New("OIDC_ISSUER must not contain a query string or fragment")
	}

	cfg.Scopes = parseOIDCScopes(os.Getenv("OIDC_SCOPES"))
	if cfg.ProviderName == "" {
		cfg.ProviderName = "Single sign-on"
	}
	if cfg.NameClaim == "" {
		cfg.NameClaim = "name"
	}
	if ttl, err := envDuration("OIDC_LOGIN_STATE_TTL"); err != nil {
		return cfg, err
	} else if ttl > 0 {
		cfg.LoginStateTTL = ttl
	}
	if ttl, err := envDuration("OIDC_HANDOFF_TTL"); err != nil {
		return cfg, err
	} else if ttl > 0 {
		cfg.HandoffTTL = ttl
	}

	return cfg, nil
}

// parseOIDCScopes normalises the configured scope list. "openid" is mandatory
// per the spec, so it is added when the operator leaves it out, and duplicates
// are dropped so the authorization URL stays clean.
func parseOIDCScopes(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' '
	})
	if len(fields) == 0 {
		return []string{"openid", "profile", "email"}
	}
	seen := map[string]bool{}
	scopes := make([]string, 0, len(fields)+1)
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" || seen[f] {
			continue
		}
		seen[f] = true
		scopes = append(scopes, f)
	}
	if !seen["openid"] {
		scopes = append([]string{"openid"}, scopes...)
	}
	return scopes
}

// requireAbsoluteURL rejects values that are not absolute http(s) URLs. A
// relative or scheme-less value here would produce a broken authorization URL
// or, worse, a redirect the browser resolves against the API's own origin.
func requireAbsoluteURL(key, raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s is not a valid URL: %w", key, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s must be an absolute http(s) URL", key)
	}
	if u.Host == "" {
		return fmt.Errorf("%s must include a host", key)
	}
	return nil
}

// envBool treats only the explicit affirmative spellings as true, so a typo
// never silently switches on a security-relevant behaviour.
func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// envDuration parses an optional Go duration. Unlike the tuning knobs in
// cmd/api, an unparseable value here is an error rather than a warning: these
// bound how long a security artefact stays valid, and silently keeping the
// default would hide a misconfiguration the operator believes is in effect.
func envDuration(key string) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s is not a valid duration (e.g. 10m, 90s): %w", key, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return d, nil
}
