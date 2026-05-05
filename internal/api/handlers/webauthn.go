package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// requireWebAuthn returns true and writes an error response when the WebAuthn
// service has not been configured (RP ID/origins missing).
func (h *APIHandler) requireWebAuthn(c *gin.Context) bool {
	if h.webauthnService == nil {
		h.sendError(c, http.StatusServiceUnavailable, "WebAuthn is not configured on this server")
		return false
	}
	return true
}

func toMap(v interface{}) map[string]interface{} {
	raw, err := json.Marshal(v)
	if err != nil {
		return map[string]interface{}{}
	}
	out := map[string]interface{}{}
	_ = json.Unmarshal(raw, &out)
	return out
}

// WebauthnRegisterOptions implements POST /auth/webauthn/register/options
func (h *APIHandler) WebauthnRegisterOptions(c *gin.Context) {
	if !h.requireWebAuthn(c) {
		return
	}
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	sessionID, options, err := h.webauthnService.BeginRegistration(c.Request.Context(), userID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to start passkey registration")
		return
	}

	c.JSON(http.StatusOK, generated.WebAuthnRegistrationOptions{
		SessionId: sessionID,
		PublicKey: toMap(options.Response),
	})
}

// WebauthnRegisterVerify implements POST /auth/webauthn/register/verify
func (h *APIHandler) WebauthnRegisterVerify(c *gin.Context) {
	if !h.requireWebAuthn(c) {
		return
	}
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req generated.WebauthnRegisterVerifyJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	responseJSON, err := json.Marshal(req.Response)
	if err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid registration response")
		return
	}

	cred, err := h.webauthnService.FinishRegistration(c.Request.Context(), userID, req.SessionId, req.Label, responseJSON)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrWebAuthnSessionNotFound), errors.Is(err, service.ErrWebAuthnSessionExpired):
			h.sendError(c, http.StatusBadRequest, "Registration session expired — please try again")
		case errors.Is(err, service.ErrWebAuthnInvalidResponse):
			h.sendError(c, http.StatusBadRequest, "Invalid registration response")
		case errors.Is(err, service.ErrWebAuthnVerification):
			h.sendError(c, http.StatusBadRequest, "Passkey could not be verified")
		default:
			h.sendError(c, http.StatusInternalServerError, "Failed to register passkey")
		}
		return
	}

	c.JSON(http.StatusOK, webauthnCredentialToResponse(cred))
}

// WebauthnLoginOptions implements POST /auth/webauthn/login/options
func (h *APIHandler) WebauthnLoginOptions(c *gin.Context) {
	if !h.requireWebAuthn(c) {
		return
	}

	var req generated.WebauthnLoginOptionsJSONRequestBody
	// Body is optional — ignore decode errors for empty bodies.
	_ = c.ShouldBindJSON(&req)

	email := ""
	if req.Email != nil {
		email = string(*req.Email)
	}

	sessionID, options, err := h.webauthnService.BeginLogin(c.Request.Context(), email)
	if err != nil {
		if errors.Is(err, service.ErrInvalidEmail) {
			h.sendError(c, http.StatusBadRequest, "Invalid email")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to start passkey login")
		return
	}

	c.JSON(http.StatusOK, generated.WebAuthnLoginOptions{
		SessionId: sessionID,
		PublicKey: toMap(options.Response),
	})
}

// WebauthnLoginVerify implements POST /auth/webauthn/login/verify
func (h *APIHandler) WebauthnLoginVerify(c *gin.Context) {
	if !h.requireWebAuthn(c) {
		return
	}

	var req generated.WebauthnLoginVerifyJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	responseJSON, err := json.Marshal(req.Response)
	if err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid login response")
		return
	}

	user, tokens, err := h.webauthnService.FinishLogin(c.Request.Context(), req.SessionId, responseJSON)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrWebAuthnSessionNotFound), errors.Is(err, service.ErrWebAuthnSessionExpired):
			h.sendError(c, http.StatusBadRequest, "Login session expired — please try again")
		case errors.Is(err, service.ErrWebAuthnInvalidResponse):
			h.sendError(c, http.StatusBadRequest, "Invalid login response")
		case errors.Is(err, service.ErrWebAuthnUnknownCredential):
			h.sendError(c, http.StatusUnauthorized, "Unknown passkey")
		case errors.Is(err, service.ErrAccountDisabled):
			h.sendError(c, http.StatusForbidden, "Account disabled. Contact the administrator.")
		case errors.Is(err, service.ErrWebAuthnVerification):
			h.sendError(c, http.StatusUnauthorized, "Passkey verification failed")
		default:
			h.sendError(c, http.StatusInternalServerError, "Failed to complete passkey login")
		}
		return
	}

	c.JSON(http.StatusOK, h.convertAuthResponse(user, tokens))
}

// ListWebauthnCredentials implements GET /auth/webauthn/credentials
func (h *APIHandler) ListWebauthnCredentials(c *gin.Context) {
	if !h.requireWebAuthn(c) {
		return
	}
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	creds, err := h.webauthnService.ListCredentials(c.Request.Context(), userID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to list passkeys")
		return
	}
	out := make([]generated.WebAuthnCredential, 0, len(creds))
	for _, cred := range creds {
		out = append(out, webauthnCredentialToResponse(cred))
	}
	c.JSON(http.StatusOK, out)
}

// DeleteWebauthnCredential implements DELETE /auth/webauthn/credentials/{credentialId}
func (h *APIHandler) DeleteWebauthnCredential(c *gin.Context, credentialId openapi_types.UUID) {
	if !h.requireWebAuthn(c) {
		return
	}
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if err := h.webauthnService.DeleteCredential(c.Request.Context(), userID, uuid.UUID(credentialId)); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.sendError(c, http.StatusNotFound, "Passkey not found")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to revoke passkey")
		return
	}
	c.Status(http.StatusNoContent)
}

func webauthnCredentialToResponse(c *models.WebAuthnCredential) generated.WebAuthnCredential {
	out := generated.WebAuthnCredential{
		Id:        openapi_types.UUID(c.ID),
		Label:     c.Label,
		CreatedAt: c.CreatedAt,
	}
	if c.LastUsedAt != nil {
		out.LastUsedAt = c.LastUsedAt
	}
	if len(c.Transports) > 0 {
		t := []string(c.Transports)
		out.Transports = &t
	}
	if len(c.AAGUID) > 0 {
		hex := uuidFromAAGUID(c.AAGUID)
		out.Aaguid = &hex
	}
	return out
}

// uuidFromAAGUID renders an AAGUID byte slice as a canonical UUID-like hex string.
func uuidFromAAGUID(b []byte) string {
	if len(b) == 16 {
		var u uuid.UUID
		copy(u[:], b)
		return u.String()
	}
	const hexChars = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hexChars[v>>4]
		out[i*2+1] = hexChars[v&0x0f]
	}
	return string(out)
}
