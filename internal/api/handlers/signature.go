package handlers

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/pkg/duration"
	emailpkg "github.com/fjaeckel/ninerlog-api/pkg/email"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// flightSummary renders a short human-readable description of a flight for
// use in signature request/confirmation emails, e.g. "12 Jul 2026 —
// D-EFGH (C172), 1h24m".
func flightSummary(f *models.Flight) string {
	return fmt.Sprintf("%s — %s (%s), %s",
		f.Date.Format("02 Jan 2006"), f.AircraftReg, f.AircraftType, duration.FormatHM(f.TotalTime))
}

// convertToGeneratedFlightSignature maps a models.FlightSignature to its
// generated API representation. The raw signature image is never included —
// it's served separately via GetFlightSignatureImage.
func convertToGeneratedFlightSignature(s *models.FlightSignature) generated.FlightSignature {
	out := generated.FlightSignature{
		Id:             openapi_types.UUID(s.ID),
		FlightId:       openapi_types.UUID(s.FlightID),
		Method:         generated.FlightSignatureMethod(s.Method),
		Status:         generated.FlightSignatureStatus(s.Status),
		EmailSendCount: s.EmailSendCount,
		CreatedAt:      s.CreatedAt,
		UpdatedAt:      s.UpdatedAt,
	}
	out.InstructorName = s.InstructorName
	out.InstructorCredentialNumber = s.InstructorCredentialNo
	if s.InstructorEmail != nil {
		e := openapi_types.Email(*s.InstructorEmail)
		out.InstructorEmail = &e
	}
	out.EmailSentAt = s.EmailSentAt
	out.TokenExpiresAt = s.TokenExpiresAt
	out.SignedAt = s.SignedAt
	out.VoidedAt = s.VoidedAt
	out.VoidedReason = s.VoidedReason
	return out
}

func signatureRequestCreatedResponse(s *models.FlightSignature, signURL string) generated.SignatureRequestCreated {
	sig := convertToGeneratedFlightSignature(s)
	return generated.SignatureRequestCreated{
		Id:                         sig.Id,
		FlightId:                   sig.FlightId,
		Method:                     generated.SignatureRequestCreatedMethod(sig.Method),
		Status:                     generated.SignatureRequestCreatedStatus(sig.Status),
		InstructorName:             sig.InstructorName,
		InstructorCredentialNumber: sig.InstructorCredentialNumber,
		InstructorEmail:            sig.InstructorEmail,
		EmailSentAt:                sig.EmailSentAt,
		EmailSendCount:             sig.EmailSendCount,
		TokenExpiresAt:             sig.TokenExpiresAt,
		SignedAt:                   sig.SignedAt,
		VoidedAt:                   sig.VoidedAt,
		VoidedReason:               sig.VoidedReason,
		CreatedAt:                  sig.CreatedAt,
		UpdatedAt:                  sig.UpdatedAt,
		SignUrl:                    signURL,
	}
}

// sendSignatureServiceError maps the sentinels shared by every signature
// endpoint to an HTTP response.
func (h *APIHandler) sendSignatureServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrFlightNotFound), errors.Is(err, service.ErrUnauthorizedFlight), errors.Is(err, service.ErrSignatureNotFound):
		h.sendError(c, http.StatusNotFound, "Not found")
	case errors.Is(err, service.ErrFlightLocked):
		h.sendError(c, http.StatusConflict, "This flight is locked by a completed signature. Void the signature to make changes.")
	case errors.Is(err, service.ErrSignaturePendingExists):
		h.sendError(c, http.StatusConflict, "A pending signature request already exists for this flight")
	case errors.Is(err, service.ErrSignatureNotPending):
		h.sendError(c, http.StatusConflict, "Signature request is not pending")
	case errors.Is(err, service.ErrSignatureNotCompleted):
		h.sendError(c, http.StatusConflict, "Signature is not completed")
	case errors.Is(err, service.ErrSignerNameRequired), errors.Is(err, service.ErrSignatureImageRequired), errors.Is(err, service.ErrSignatureReasonRequired):
		h.sendError(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrSignatureImageTooLarge):
		h.sendError(c, http.StatusBadRequest, err.Error())
	default:
		h.sendError(c, http.StatusInternalServerError, "Signature operation failed")
	}
}

// ListFlightSignatures implements GET /flights/{flightId}/signatures
func (h *APIHandler) ListFlightSignatures(c *gin.Context, flightId generated.FlightId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	sigs, err := h.flightSignatureService.List(c.Request.Context(), uuid.UUID(flightId), userID)
	if err != nil {
		h.sendSignatureServiceError(c, err)
		return
	}
	out := make([]generated.FlightSignature, 0, len(sigs))
	for _, s := range sigs {
		out = append(out, convertToGeneratedFlightSignature(s))
	}
	c.JSON(http.StatusOK, out)
}

// GetFlightSignature implements GET /flights/{flightId}/signatures/{signatureId}
func (h *APIHandler) GetFlightSignature(c *gin.Context, flightId generated.FlightId, signatureId generated.SignatureId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	sig, err := h.flightSignatureService.Get(c.Request.Context(), uuid.UUID(flightId), userID, uuid.UUID(signatureId))
	if err != nil {
		h.sendSignatureServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, convertToGeneratedFlightSignature(sig))
}

// GetFlightSignatureImage implements GET /flights/{flightId}/signatures/{signatureId}/image
func (h *APIHandler) GetFlightSignatureImage(c *gin.Context, flightId generated.FlightId, signatureId generated.SignatureId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	sig, err := h.flightSignatureService.Get(c.Request.Context(), uuid.UUID(flightId), userID, uuid.UUID(signatureId))
	if err != nil {
		h.sendSignatureServiceError(c, err)
		return
	}
	if len(sig.SignatureImage) == 0 {
		h.sendError(c, http.StatusNotFound, "No signature image recorded")
		return
	}
	c.Data(http.StatusOK, "image/png", sig.SignatureImage)
}

// SignFlightLive implements POST /flights/{flightId}/signatures/live
func (h *APIHandler) SignFlightLive(c *gin.Context, flightId generated.FlightId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var req generated.SignFlightLiveJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	sig, err := h.flightSignatureService.SignLive(c.Request.Context(), uuid.UUID(flightId), userID,
		req.SignerName, req.CredentialNumber, req.SignatureImage, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		h.sendSignatureServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, convertToGeneratedFlightSignature(sig))
}

// CreateSignatureRequest implements POST /flights/{flightId}/signatures
func (h *APIHandler) CreateSignatureRequest(c *gin.Context, flightId generated.FlightId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var req generated.CreateSignatureRequestJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	var instructorEmail *string
	if req.InstructorEmail != nil {
		e := string(*req.InstructorEmail)
		instructorEmail = &e
	}

	sig, rawToken, err := h.flightSignatureService.CreateRequest(c.Request.Context(), uuid.UUID(flightId), userID, instructorEmail, req.ExpiresInHours)
	if err != nil {
		h.sendSignatureServiceError(c, err)
		return
	}

	signURL := fmt.Sprintf("%s/sign?token=%s", frontendBaseURL(), rawToken)
	if instructorEmail != nil {
		h.sendSignatureRequestEmailAndMark(c, userID, sig, signURL)
	}

	c.JSON(http.StatusCreated, signatureRequestCreatedResponse(sig, signURL))
}

// ResendSignatureRequest implements POST /flights/{flightId}/signatures/{signatureId}/resend
func (h *APIHandler) ResendSignatureRequest(c *gin.Context, flightId generated.FlightId, signatureId generated.SignatureId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var req generated.ResendSignatureRequestJSONRequestBody
	_ = c.ShouldBindJSON(&req) // body is optional (rotate-only when empty)

	var instructorEmail *string
	if req.InstructorEmail != nil {
		e := string(*req.InstructorEmail)
		instructorEmail = &e
	}

	sig, rawToken, err := h.flightSignatureService.Resend(c.Request.Context(), uuid.UUID(flightId), userID, uuid.UUID(signatureId), instructorEmail)
	if err != nil {
		h.sendSignatureServiceError(c, err)
		return
	}

	signURL := fmt.Sprintf("%s/sign?token=%s", frontendBaseURL(), rawToken)
	if instructorEmail != nil {
		h.sendSignatureRequestEmailAndMark(c, userID, sig, signURL)
	}

	c.JSON(http.StatusOK, signatureRequestCreatedResponse(sig, signURL))
}

// RevokeSignatureRequest implements POST /flights/{flightId}/signatures/{signatureId}/revoke
func (h *APIHandler) RevokeSignatureRequest(c *gin.Context, flightId generated.FlightId, signatureId generated.SignatureId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if err := h.flightSignatureService.Revoke(c.Request.Context(), uuid.UUID(flightId), userID, uuid.UUID(signatureId)); err != nil {
		h.sendSignatureServiceError(c, err)
		return
	}
	sig, err := h.flightSignatureService.Get(c.Request.Context(), uuid.UUID(flightId), userID, uuid.UUID(signatureId))
	if err != nil {
		h.sendSignatureServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, convertToGeneratedFlightSignature(sig))
}

// VoidFlightSignature implements POST /flights/{flightId}/signatures/{signatureId}/void
func (h *APIHandler) VoidFlightSignature(c *gin.Context, flightId generated.FlightId, signatureId generated.SignatureId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var req generated.VoidFlightSignatureJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	if err := h.flightSignatureService.Void(c.Request.Context(), uuid.UUID(flightId), userID, uuid.UUID(signatureId), req.Reason); err != nil {
		h.sendSignatureServiceError(c, err)
		return
	}
	sig, err := h.flightSignatureService.Get(c.Request.Context(), uuid.UUID(flightId), userID, uuid.UUID(signatureId))
	if err != nil {
		h.sendSignatureServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, convertToGeneratedFlightSignature(sig))
}

// sendSignatureRequestEmailAndMark sends the "please sign" email to the
// request's instructor address and records that it was sent. Errors are
// swallowed (mirrors sendVerificationEmail): the operator can inspect SMTP
// logs and the owner can resend from the UI.
func (h *APIHandler) sendSignatureRequestEmailAndMark(c *gin.Context, userID uuid.UUID, sig *models.FlightSignature, signURL string) {
	if h.emailSender == nil || sig.InstructorEmail == nil {
		return
	}
	owner, err := h.authService.GetUserByID(c.Request.Context(), userID)
	if err != nil || owner == nil {
		return
	}
	flight, err := h.flightService.GetFlight(c.Request.Context(), sig.FlightID, userID)
	if err != nil {
		return
	}

	expiresAt := ""
	if sig.TokenExpiresAt != nil {
		expiresAt = sig.TokenExpiresAt.Format("02 Jan 2006 15:04 MST")
	}
	tmpl := emailpkg.Templates(owner.PreferredLocale)
	subject, body := tmpl.SignatureRequest(emailpkg.SignatureRequestParams{
		OwnerName:     owner.Name,
		FlightSummary: flightSummary(flight),
		Link:          signURL,
		ExpiresAt:     expiresAt,
	})
	if err := h.emailSender.Send(*sig.InstructorEmail, subject, body); err == nil {
		_ = h.flightSignatureService.MarkEmailSent(c.Request.Context(), sig)
	}
}
