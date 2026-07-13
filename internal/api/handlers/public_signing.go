package handlers

import (
	"errors"
	"net/http"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	emailpkg "github.com/fjaeckel/ninerlog-api/pkg/email"
	"github.com/gin-gonic/gin"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// This file holds ONLY the two unauthenticated /sign/{token} endpoints. No
// function here should ever call getUserIDFromContext, and neither endpoint
// may respond with 401/403 — an anonymous instructor with no NinerLog
// account must never be bounced through an auth flow (see the frontend's
// global 401-response interceptor, which would otherwise redirect them to
// /login). Invalid/used/revoked/voided tokens and unknown tokens all report
// an identical 404 so a caller cannot distinguish "never existed" from
// "already used" (anti-enumeration); expired tokens report 410.

// GetPublicSignatureInfo implements GET /sign/{token}
func (h *APIHandler) GetPublicSignatureInfo(c *gin.Context, token generated.SignatureToken) {
	sig, flight, err := h.flightSignatureService.ResolveToken(c.Request.Context(), token)
	if err != nil {
		sendPublicSigningError(h, c, err)
		return
	}

	info := generated.PublicSignatureInfo{
		Status:       generated.PublicSignatureInfoStatus(sig.Status),
		FlightDate:   openapi_types.Date{Time: flight.Date},
		AircraftReg:  flight.AircraftReg,
		AircraftType: flight.AircraftType,
		TotalTime:    flight.TotalTime,
		Route:        flight.Route,
	}
	dualTime := flight.DualTime
	info.DualTime = &dualTime
	info.InstructorName = sig.InstructorName
	if sig.TokenExpiresAt != nil {
		info.ExpiresAt = *sig.TokenExpiresAt
	}

	c.JSON(http.StatusOK, info)
}

// CompletePublicSignature implements POST /sign/{token}
func (h *APIHandler) CompletePublicSignature(c *gin.Context, token generated.SignatureToken) {
	var req generated.CompletePublicSignatureJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	sig, ownerEmail, ownerName, err := h.flightSignatureService.CompleteFromToken(
		c.Request.Context(), token, req.SignerName, req.CredentialNumber, req.SignatureImage,
		c.ClientIP(), c.Request.UserAgent(),
	)
	if err != nil {
		sendPublicSigningError(h, c, err)
		return
	}

	h.sendSignatureCompletedEmail(c, sig, ownerEmail, ownerName)

	c.JSON(http.StatusOK, gin.H{"message": "Signature recorded. Thank you!"})
}

// sendPublicSigningError maps the public token-flow sentinels to 404/410/400
// ONLY — this endpoint must never emit 401/403 (see file doc comment above).
func sendPublicSigningError(h *APIHandler, c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrSignatureTokenExpired):
		h.sendError(c, http.StatusGone, "This signing link has expired")
	case errors.Is(err, service.ErrSignatureTokenInvalid):
		h.sendError(c, http.StatusNotFound, "This signing link is invalid or has already been used")
	case errors.Is(err, service.ErrSignerNameRequired), errors.Is(err, service.ErrSignatureImageRequired), errors.Is(err, service.ErrSignatureImageTooLarge):
		h.sendError(c, http.StatusBadRequest, err.Error())
	default:
		h.sendError(c, http.StatusInternalServerError, "Signature operation failed")
	}
}

// sendSignatureCompletedEmail notifies the flight owner that their entry was
// signed, using the DB-sourced address/name returned by the service — never
// anything from the (unauthenticated) request itself (CWE-640). Errors are
// swallowed; this is a courtesy notification, not part of the core flow.
func (h *APIHandler) sendSignatureCompletedEmail(c *gin.Context, sig *models.FlightSignature, ownerEmail, ownerName string) {
	if h.emailSender == nil || ownerEmail == "" {
		return
	}
	// The owner trivially "owns" their own flight; this is not an
	// unauthenticated-caller-supplied ID, it comes from the already-verified
	// signature record.
	flight, err := h.flightService.GetFlight(c.Request.Context(), sig.FlightID, sig.UserID)
	if err != nil {
		return
	}
	signerName := ""
	if sig.InstructorName != nil {
		signerName = *sig.InstructorName
	}
	locale := ""
	if owner, err := h.authService.GetUserByID(c.Request.Context(), sig.UserID); err == nil && owner != nil {
		locale = owner.PreferredLocale
	}
	tmpl := emailpkg.Templates(locale)
	subject, body := tmpl.SignatureCompleted(emailpkg.SignatureCompletedParams{
		OwnerName:     ownerName,
		FlightSummary: flightSummary(flight),
		SignerName:    signerName,
	})
	_ = h.emailSender.Send(ownerEmail, subject, body)
}
