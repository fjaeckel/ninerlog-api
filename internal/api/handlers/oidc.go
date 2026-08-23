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

// oidcStateCookie holds the browser half of the login-state binding:
// HttpOnly, SameSite=Lax, scoped to the OIDC paths.
const oidcStateCookie = "ninerlog_oidc_state"

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

// requireLocalAuth writes a 503 and returns false when the server runs in
// OIDC mode. Registration, password login, email verification, password
// reset, password change, TOTP and passkeys all go through it.
func (h *APIHandler) requireLocalAuth(c *gin.Context) bool {
	if h.oidcService != nil {
		h.sendError(c, http.StatusServiceUnavailable,
			"Local authentication is disabled on this server (OIDC mode)")
		return false
	}
	return true
}

// GetAuthProviders implements GET /auth/providers. Unauthenticated; exposes
// only which sign-in methods exist.
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
		nativeAuthorizeURL := authorizeURL + "?native=1"
		nativeRedirectURI := cfg.NativePostLoginRedirect
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
		resp.Oidc.NativeAuthorizeUrl = &nativeAuthorizeURL
		resp.Oidc.NativeRedirectUri = &nativeRedirectURI
	}

	c.JSON(http.StatusOK, resp)
}

// AuthorizeOidc implements GET /auth/oidc/authorize.
//
// Starts a login: mints the state, nonce and PKCE verifier, plants the
// browser-binding cookie, and redirects to the provider. `native=1` finishes
// the login at the native redirect URI instead of the web frontend.
func (h *APIHandler) AuthorizeOidc(c *gin.Context, params generated.AuthorizeOidcParams) {
	if !h.requireOIDC(c) {
		return
	}

	native := params.Native != nil && isAffirmative(*params.Native)
	auth, err := h.oidcService.BeginLogin(c.Request.Context(), native)
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
	if native {
		OIDCLoginAttemptsTotal.WithLabelValues("authorize_native").Inc()
	} else {
		OIDCLoginAttemptsTotal.WithLabelValues("authorize").Inc()
	}
	c.Redirect(http.StatusFound, auth.AuthorizationURL)
}

// OidcCallback implements GET /auth/oidc/callback.
//
// The provider sends the browser here. Every failure ends in a redirect back
// to the frontend carrying a short error code, not a JSON error page.
func (h *APIHandler) OidcCallback(c *gin.Context, params generated.OidcCallbackParams) {
	if !h.requireOIDC(c) {
		return
	}

	// The cookie is single-use regardless of outcome.
	browserToken, _ := c.Cookie(oidcStateCookie)
	h.clearOIDCStateCookie(c)

	native := service.IsNativeOIDCState(safeStr(params.State))

	// The provider reports user-facing failures (consent denied, and so on) as
	// query parameters rather than a non-2xx status.
	if providerErr := safeStr(params.Error); providerErr != "" {
		OIDCLoginAttemptsTotal.WithLabelValues("provider_error").Inc()
		h.redirectOIDCError(c, native, "provider_error")
		return
	}

	handoff, err := h.oidcService.CompleteCallback(c.Request.Context(),
		safeStr(params.Code), safeStr(params.State), browserToken)
	if err != nil {
		h.redirectOIDCError(c, native, oidcErrorCode(err))
		return
	}

	OIDCLoginAttemptsTotal.WithLabelValues("callback_success").Inc()
	h.redirectOIDC(c, native, url.Values{"oidc_code": {handoff}})
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

// oidcErrorCode maps an internal failure to the short, coarse code handed to
// the frontend.
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

func (h *APIHandler) redirectOIDCError(c *gin.Context, native bool, code string) {
	h.redirectOIDC(c, native, url.Values{"oidc_error": {code}})
}

// redirectOIDC sends the browser back to the configured post-login URL with
// the supplied query parameters merged in. Both targets come from
// configuration — OIDC_POST_LOGIN_REDIRECT and OIDC_NATIVE_POST_LOGIN_REDIRECT
// — never from the request.
func (h *APIHandler) redirectOIDC(c *gin.Context, native bool, params url.Values) {
	target := h.oidcService.PostLoginRedirect()
	if native {
		target = h.oidcService.NativePostLoginRedirect()
	}
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

// setOIDCStateCookie plants the browser-binding value. Secure is derived from
// the configured callback URL, not from the inbound request.
func (h *APIHandler) setOIDCStateCookie(c *gin.Context, value string, maxAge int) {
	if maxAge <= 0 {
		maxAge = int(service.DefaultOIDCLoginStateTTL.Seconds())
	}
	cfg := h.oidcService.Config()
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(oidcStateCookie, value, maxAge, cfg.CookiePath(), "",
		cfg.CookieSecure(), true)
}

// isAffirmative reads the query-parameter spellings of true.
func isAffirmative(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (h *APIHandler) clearOIDCStateCookie(c *gin.Context) {
	cfg := h.oidcService.Config()
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(oidcStateCookie, "", -1, cfg.CookiePath(), "", cfg.CookieSecure(), true)
}
