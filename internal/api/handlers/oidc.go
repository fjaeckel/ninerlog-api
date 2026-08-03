package handlers

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/gin-gonic/gin"
)

// oidcStateCookie holds the browser half of the login-state binding. It is
// HttpOnly (no script needs it), SameSite=Lax so it is still sent on the
// provider's top-level redirect back to the callback, and scoped to the OIDC
// paths so it is never attached to ordinary API calls.
const oidcStateCookie = "ninerlog_oidc_state"

// oidcCookiePath scopes the cookie to the only two endpoints that read it.
const oidcCookiePath = "/api/v1/auth/oidc"

// OIDCEnabled reports whether the server runs in OIDC mode.
func (h *APIHandler) OIDCEnabled() bool { return h.oidcService != nil }

// requireOIDC writes a 503 and returns false when OIDC is not configured.
func (h *APIHandler) requireOIDC(c *gin.Context) bool {
	if h.oidcService == nil {
		h.sendError(c, http.StatusServiceUnavailable, "OIDC is not configured on this server")
		return false
	}
	return true
}

// requireLocalAuth writes a 503 and returns false when the server runs in OIDC
// mode, where the identity provider owns accounts and every local credential
// path is switched off.
//
// This is the single choke point for that rule: registration, password login,
// email verification, password reset, password change, TOTP and passkeys all
// go through it, so there is one place to audit rather than a dozen.
func (h *APIHandler) requireLocalAuth(c *gin.Context) bool {
	if h.oidcService != nil {
		h.sendError(c, http.StatusServiceUnavailable,
			"Local authentication is disabled on this server (OIDC mode)")
		return false
	}
	return true
}

// GetAuthProviders implements GET /auth/providers
//
// Unauthenticated on purpose: the client has to know which sign-in UI to draw
// before anyone can log in. It exposes only which methods exist, never the
// issuer's client secret or any account data.
func (h *APIHandler) GetAuthProviders(c *gin.Context) {
	resp := generated.AuthProviders{
		Mode:                 generated.AuthProvidersModeLocal,
		PasswordLoginEnabled: true,
		RegistrationEnabled:  true,
		TwoFactorEnabled:     true,
		WebauthnEnabled:      h.webauthnService != nil,
	}
	resp.Oidc.Enabled = false

	if h.oidcService != nil {
		cfg := h.oidcService.Config()
		name := cfg.ProviderName
		authorizeURL := "/api/v1/auth/oidc/authorize"
		resp = generated.AuthProviders{
			Mode:                 generated.AuthProvidersModeOidc,
			PasswordLoginEnabled: false,
			RegistrationEnabled:  false,
			TwoFactorEnabled:     false,
			WebauthnEnabled:      false,
		}
		resp.Oidc.Enabled = true
		resp.Oidc.Name = &name
		resp.Oidc.AuthorizeUrl = &authorizeURL
	}

	c.JSON(http.StatusOK, resp)
}

// RegisterOIDCRoutes registers the two browser-facing OIDC endpoints.
//
// These are redirects driven by top-level navigation, not JSON operations, so
// they are wired manually rather than through the generated ServerInterface —
// the same treatment the reports and flight-utility routes get. The two JSON
// endpoints of the flow (/auth/providers and /auth/oidc/exchange) are in the
// OpenAPI spec and generated normally.
func RegisterOIDCRoutes(rg *gin.RouterGroup, h *APIHandler) {
	rg.GET("/auth/oidc/authorize", h.OIDCAuthorize)
	rg.GET("/auth/oidc/callback", h.OIDCCallback)
}

// OIDCAuthorize implements GET /auth/oidc/authorize.
//
// Starts a login: mints the state, nonce and PKCE verifier, plants the
// browser-binding cookie, and redirects to the provider.
func (h *APIHandler) OIDCAuthorize(c *gin.Context) {
	if !h.requireOIDC(c) {
		return
	}

	auth, err := h.oidcService.BeginLogin(c.Request.Context())
	if err != nil {
		OIDCLoginAttemptsTotal.WithLabelValues("authorize_failed").Inc()
		if errors.Is(err, service.ErrOIDCProviderUnavailable) {
			h.sendError(c, http.StatusServiceUnavailable,
				"The identity provider is currently unreachable. Please try again shortly.")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to start single sign-on")
		return
	}

	h.setOIDCStateCookie(c, auth.BrowserToken, int(time.Until(auth.Expiry).Seconds()))
	OIDCLoginAttemptsTotal.WithLabelValues("authorize").Inc()
	c.Redirect(http.StatusFound, auth.AuthorizationURL)
}

// OIDCCallback implements GET /auth/oidc/callback.
//
// The provider sends the browser here. Everything that can go wrong ends in a
// redirect back to the frontend carrying a short error code rather than a JSON
// error page, because the user is looking at a browser window mid-login, not
// at an API client.
func (h *APIHandler) OIDCCallback(c *gin.Context) {
	if !h.requireOIDC(c) {
		return
	}

	// The cookie is single-use regardless of outcome: a failed attempt must
	// not leave a binding another attempt could ride on.
	browserToken, _ := c.Cookie(oidcStateCookie)
	h.clearOIDCStateCookie(c)

	// The provider reports user-facing failures (consent denied, and so on) as
	// query parameters rather than a non-2xx status.
	if providerErr := c.Query("error"); providerErr != "" {
		OIDCLoginAttemptsTotal.WithLabelValues("provider_error").Inc()
		h.redirectOIDCError(c, "provider_error")
		return
	}

	handoff, err := h.oidcService.CompleteCallback(c.Request.Context(),
		c.Query("code"), c.Query("state"), browserToken)
	if err != nil {
		h.redirectOIDCError(c, oidcErrorCode(err))
		return
	}

	OIDCLoginAttemptsTotal.WithLabelValues("callback_success").Inc()
	h.redirectOIDC(c, url.Values{"oidc_code": {handoff}})
}

// ExchangeOidcCode implements POST /auth/oidc/exchange
func (h *APIHandler) ExchangeOidcCode(c *gin.Context) {
	if !h.requireOIDC(c) {
		return
	}
	var req generated.ExchangeOidcCodeJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	user, tokens, err := h.oidcService.ExchangeHandoff(c.Request.Context(), req.Code)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOIDCHandoffInvalid):
			OIDCLoginAttemptsTotal.WithLabelValues("handoff_invalid").Inc()
			h.sendError(c, http.StatusUnauthorized, "Sign-in code is invalid or has expired")
		case errors.Is(err, service.ErrAccountDisabled):
			OIDCLoginAttemptsTotal.WithLabelValues("account_disabled").Inc()
			h.sendError(c, http.StatusForbidden, "Account disabled. Contact the administrator.")
		default:
			OIDCLoginAttemptsTotal.WithLabelValues("error").Inc()
			h.sendError(c, http.StatusInternalServerError, "Sign-in failed")
		}
		return
	}

	OIDCLoginAttemptsTotal.WithLabelValues("success").Inc()
	c.JSON(http.StatusOK, h.convertAuthResponse(user, tokens))
}

// oidcErrorCode maps an internal failure to the short, non-descriptive code
// handed to the frontend. Deliberately coarse: the browser (and anyone
// watching the URL) learns that the login failed, not why.
func oidcErrorCode(err error) string {
	switch {
	case errors.Is(err, service.ErrOIDCProviderUnavailable):
		OIDCLoginAttemptsTotal.WithLabelValues("provider_unavailable").Inc()
		return "provider_unavailable"
	case errors.Is(err, service.ErrOIDCInvalidState):
		OIDCLoginAttemptsTotal.WithLabelValues("invalid_state").Inc()
		return "invalid_state"
	case errors.Is(err, service.ErrOIDCEmailMissing):
		OIDCLoginAttemptsTotal.WithLabelValues("email_missing").Inc()
		return "email_missing"
	case errors.Is(err, service.ErrOIDCEmailConflict):
		OIDCLoginAttemptsTotal.WithLabelValues("email_conflict").Inc()
		return "email_conflict"
	case errors.Is(err, service.ErrAccountDisabled):
		OIDCLoginAttemptsTotal.WithLabelValues("account_disabled").Inc()
		return "account_disabled"
	default:
		OIDCLoginAttemptsTotal.WithLabelValues("error").Inc()
		return "login_failed"
	}
}

func (h *APIHandler) redirectOIDCError(c *gin.Context, code string) {
	h.redirectOIDC(c, url.Values{"oidc_error": {code}})
}

// redirectOIDC sends the browser back to the configured frontend URL with the
// supplied query parameters merged in.
//
// The target always comes from OIDC_POST_LOGIN_REDIRECT and never from the
// request, so no combination of query parameters can turn the callback into an
// open redirect.
func (h *APIHandler) redirectOIDC(c *gin.Context, params url.Values) {
	target := h.oidcService.PostLoginRedirect()
	u, err := url.Parse(target)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Sign-in failed")
		return
	}
	q := u.Query()
	for k, vs := range params {
		for _, v := range vs {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	c.Redirect(http.StatusFound, u.String())
}

// setOIDCStateCookie plants the browser-binding value.
//
// Secure is derived from the configured callback URL rather than from the
// inbound request, which may have been terminated at a reverse proxy: an
// https deployment must always get a Secure cookie, and a plain-http local
// dev setup must not (browsers drop Secure cookies on insecure origins).
func (h *APIHandler) setOIDCStateCookie(c *gin.Context, value string, maxAge int) {
	if maxAge <= 0 {
		maxAge = int(service.DefaultOIDCLoginStateTTL.Seconds())
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(oidcStateCookie, value, maxAge, oidcCookiePath, "",
		h.oidcCookieSecure(), true)
}

func (h *APIHandler) clearOIDCStateCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(oidcStateCookie, "", -1, oidcCookiePath, "", h.oidcCookieSecure(), true)
}

func (h *APIHandler) oidcCookieSecure() bool {
	if h.oidcService == nil {
		return true
	}
	return strings.HasPrefix(strings.ToLower(h.oidcService.Config().RedirectURL), "https://")
}
