package handlers

import (
	"errors"
	"net/http"
	"net/mail"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/gin-gonic/gin"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// SetEmailDeliveryService stores the email delivery log service.
func (h *APIHandler) SetEmailDeliveryService(s *service.EmailDeliveryService) {
	h.emailDeliveryService = s
}

// SetUnverifiedAccountService stores the unverified-account reaper, so an
// administrator can run a sweep on demand.
func (h *APIHandler) SetUnverifiedAccountService(s *service.UnverifiedAccountService) {
	h.unverifiedAccountService = s
}

// ListEmailDeliveries implements GET /admin/email/deliveries
func (h *APIHandler) ListEmailDeliveries(c *gin.Context, params generated.ListEmailDeliveriesParams) {
	if _, ok := h.requireAdmin(c); !ok {
		return
	}
	if h.emailDeliveryService == nil {
		h.sendError(c, http.StatusServiceUnavailable, "Email delivery tracking is not available")
		return
	}

	recipient := ""
	if params.Recipient != nil {
		recipient = string(*params.Recipient)
	}
	limit := 0
	if params.Limit != nil {
		limit = *params.Limit
	}

	events, err := h.emailDeliveryService.ListEvents(c.Request.Context(), recipient, limit)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to load delivery events")
		return
	}

	data := make([]generated.EmailDeliveryEvent, 0, len(events))
	for _, e := range events {
		data = append(data, toGeneratedDeliveryEvent(e))
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// ListEmailSuppressions implements GET /admin/email/suppressions
func (h *APIHandler) ListEmailSuppressions(c *gin.Context, params generated.ListEmailSuppressionsParams) {
	if _, ok := h.requireAdmin(c); !ok {
		return
	}
	if h.emailDeliveryService == nil {
		h.sendError(c, http.StatusServiceUnavailable, "Email delivery tracking is not available")
		return
	}

	limit := 0
	if params.Limit != nil {
		limit = *params.Limit
	}

	suppressions, err := h.emailDeliveryService.ListSuppressions(c.Request.Context(), limit)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to load suppressions")
		return
	}

	data := make([]generated.EmailSuppression, 0, len(suppressions))
	for _, s := range suppressions {
		data = append(data, generated.EmailSuppression{
			Email:          openapi_types.Email(s.Email),
			Reason:         s.Reason,
			SmtpCode:       s.SMTPCode,
			Detail:         s.Detail,
			FirstBouncedAt: s.FirstBouncedAt,
			LastBouncedAt:  s.LastBouncedAt,
			BounceCount:    s.BounceCount,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// DeleteEmailSuppression implements DELETE /admin/email/suppressions/{email}
func (h *APIHandler) DeleteEmailSuppression(c *gin.Context, email openapi_types.Email) {
	if _, ok := h.requireAdmin(c); !ok {
		return
	}
	if h.emailDeliveryService == nil {
		h.sendError(c, http.StatusServiceUnavailable, "Email delivery tracking is not available")
		return
	}

	address := string(email)
	if _, err := mail.ParseAddress(address); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid email address")
		return
	}

	if err := h.emailDeliveryService.Unsuppress(c.Request.Context(), address); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.sendError(c, http.StatusNotFound, "Address is not suppressed")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to lift suppression")
		return
	}

	c.Status(http.StatusNoContent)
}

// CleanupUnverifiedAccounts implements POST /admin/maintenance/cleanup-unverified
func (h *APIHandler) CleanupUnverifiedAccounts(c *gin.Context) {
	if _, ok := h.requireAdmin(c); !ok {
		return
	}
	if h.unverifiedAccountService == nil {
		h.sendError(c, http.StatusServiceUnavailable, "Unverified account cleanup is disabled on this deployment")
		return
	}

	reminded, deleted := h.unverifiedAccountService.Sweep(c.Request.Context())

	c.JSON(http.StatusOK, gin.H{
		"remindersSent":   reminded,
		"accountsDeleted": deleted,
		"message":         "Unverified account sweep complete",
	})
}

func toGeneratedDeliveryEvent(e *models.EmailDeliveryEvent) generated.EmailDeliveryEvent {
	out := generated.EmailDeliveryEvent{
		Id:        openapi_types.UUID(e.ID),
		Recipient: openapi_types.Email(e.Recipient),
		EmailType: e.EmailType,
		Status:    generated.EmailDeliveryEventStatus(e.Status),
		SmtpCode:  e.SMTPCode,
		Detail:    e.Detail,
		CreatedAt: e.CreatedAt,
	}
	if e.UserID != nil {
		userID := openapi_types.UUID(*e.UserID)
		out.UserId = &userID
	}
	return out
}
