package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service/currency"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// SetCustomCurrencyService wires the user-authored currency rule service.
func (h *APIHandler) SetCustomCurrencyService(s *currency.CustomService) {
	h.customCurrencyService = s
}

// customRuleRequest is the create/update payload.
type customRuleRequest struct {
	Name        string                        `json:"name"`
	Description *string                       `json:"description"`
	Emoji       *string                       `json:"emoji"`
	Definition  models.CustomCurrencyRuleBody `json:"definition"`
}

// previewRequest carries an unsaved definition for evaluation.
type previewRequest struct {
	Definition models.CustomCurrencyRuleBody `json:"definition"`
}

func (r customRuleRequest) toInput() currency.CustomRuleInput {
	return currency.CustomRuleInput{
		Name:        r.Name,
		Description: r.Description,
		Emoji:       r.Emoji,
		Definition:  r.Definition,
	}
}

// customCurrencyUser reads the authenticated user set by AuthMiddleware.
// Returns false and writes a 401 if it is missing.
func (h *APIHandler) customCurrencyUser(c *gin.Context) (uuid.UUID, bool) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return uuid.Nil, false
	}
	return userID, true
}

// respondCustomCurrencyError maps service errors to HTTP responses. Validation
// errors surface their message; not-found is 404; everything else is a generic
// 500.
func (h *APIHandler) respondCustomCurrencyError(c *gin.Context, err error) {
	switch {
	case currency.IsValidationError(err):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, repository.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
	default:
		slog.Error("[custom-currency] internal error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process custom currency rule"})
	}
}

// ListCustomCurrencyRules implements GET /custom-currency.
func (h *APIHandler) ListCustomCurrencyRules(c *gin.Context) {
	userID, ok := h.customCurrencyUser(c)
	if !ok {
		return
	}
	rules, err := h.customCurrencyService.List(c.Request.Context(), userID)
	if err != nil {
		h.respondCustomCurrencyError(c, err)
		return
	}
	c.JSON(http.StatusOK, rules)
}

// GetCustomCurrencyRule implements GET /custom-currency/{ruleId}.
func (h *APIHandler) GetCustomCurrencyRule(c *gin.Context, ruleID generated.CustomCurrencyRuleId) {
	userID, ok := h.customCurrencyUser(c)
	if !ok {
		return
	}
	rule, err := h.customCurrencyService.Get(c.Request.Context(), userID, ruleID)
	if err != nil {
		h.respondCustomCurrencyError(c, err)
		return
	}
	c.JSON(http.StatusOK, rule)
}

// CreateCustomCurrencyRule implements POST /custom-currency.
func (h *APIHandler) CreateCustomCurrencyRule(c *gin.Context) {
	userID, ok := h.customCurrencyUser(c)
	if !ok {
		return
	}
	var req customRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	rule, err := h.customCurrencyService.Create(c.Request.Context(), userID, req.toInput())
	if err != nil {
		h.respondCustomCurrencyError(c, err)
		return
	}
	c.JSON(http.StatusCreated, rule)
}

// UpdateCustomCurrencyRule implements PUT /custom-currency/{ruleId}.
func (h *APIHandler) UpdateCustomCurrencyRule(c *gin.Context, ruleID generated.CustomCurrencyRuleId) {
	userID, ok := h.customCurrencyUser(c)
	if !ok {
		return
	}
	var req customRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	rule, err := h.customCurrencyService.Update(c.Request.Context(), userID, ruleID, req.toInput())
	if err != nil {
		h.respondCustomCurrencyError(c, err)
		return
	}
	c.JSON(http.StatusOK, rule)
}

// DeleteCustomCurrencyRule implements DELETE /custom-currency/{ruleId}.
func (h *APIHandler) DeleteCustomCurrencyRule(c *gin.Context, ruleID generated.CustomCurrencyRuleId) {
	userID, ok := h.customCurrencyUser(c)
	if !ok {
		return
	}
	if err := h.customCurrencyService.Delete(c.Request.Context(), userID, ruleID); err != nil {
		h.respondCustomCurrencyError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// PreviewCustomCurrencyRule implements POST /custom-currency/preview.
func (h *APIHandler) PreviewCustomCurrencyRule(c *gin.Context) {
	userID, ok := h.customCurrencyUser(c)
	if !ok {
		return
	}
	var req previewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	result, err := h.customCurrencyService.Preview(c.Request.Context(), userID, req.Definition)
	if err != nil {
		h.respondCustomCurrencyError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// EnableCustomCurrencyRuleShare implements POST /custom-currency/{ruleId}/share.
func (h *APIHandler) EnableCustomCurrencyRuleShare(c *gin.Context, ruleID generated.CustomCurrencyRuleId) {
	h.setCustomCurrencyShare(c, ruleID, true)
}

// DisableCustomCurrencyRuleShare implements DELETE /custom-currency/{ruleId}/share.
func (h *APIHandler) DisableCustomCurrencyRuleShare(c *gin.Context, ruleID generated.CustomCurrencyRuleId) {
	h.setCustomCurrencyShare(c, ruleID, false)
}

func (h *APIHandler) setCustomCurrencyShare(c *gin.Context, ruleID uuid.UUID, shared bool) {
	userID, ok := h.customCurrencyUser(c)
	if !ok {
		return
	}
	rule, err := h.customCurrencyService.SetShared(c.Request.Context(), userID, ruleID, shared)
	if err != nil {
		h.respondCustomCurrencyError(c, err)
		return
	}
	c.JSON(http.StatusOK, rule)
}

// SetCustomCurrencyRuleEnabled implements PUT /custom-currency/{ruleId}/enabled
// — pause or resume a rule.
func (h *APIHandler) SetCustomCurrencyRuleEnabled(c *gin.Context, ruleID generated.CustomCurrencyRuleId) {
	userID, ok := h.customCurrencyUser(c)
	if !ok {
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	rule, err := h.customCurrencyService.SetEnabled(c.Request.Context(), userID, ruleID, req.Enabled)
	if err != nil {
		h.respondCustomCurrencyError(c, err)
		return
	}
	c.JSON(http.StatusOK, rule)
}

// SetCustomCurrencyRuleNotify implements PUT /custom-currency/{ruleId}/notify —
// opt a rule in/out of expiry notifications.
func (h *APIHandler) SetCustomCurrencyRuleNotify(c *gin.Context, ruleID generated.CustomCurrencyRuleId) {
	userID, ok := h.customCurrencyUser(c)
	if !ok {
		return
	}
	var req struct {
		Notify bool `json:"notify"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	rule, err := h.customCurrencyService.SetNotify(c.Request.Context(), userID, ruleID, req.Notify)
	if err != nil {
		h.respondCustomCurrencyError(c, err)
		return
	}
	c.JSON(http.StatusOK, rule)
}

// GetSharedCustomCurrencyRule implements GET /custom-currency/shared/{shareToken}
// — a read-only preview.
func (h *APIHandler) GetSharedCustomCurrencyRule(c *gin.Context, shareToken generated.CustomCurrencyShareToken) {
	if _, ok := h.customCurrencyUser(c); !ok {
		return
	}
	view, err := h.customCurrencyService.GetShared(c.Request.Context(), shareToken)
	if err != nil {
		h.respondCustomCurrencyError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

// ImportSharedCustomCurrencyRule implements
// POST /custom-currency/shared/{shareToken}/import.
func (h *APIHandler) ImportSharedCustomCurrencyRule(c *gin.Context, shareToken generated.CustomCurrencyShareToken) {
	userID, ok := h.customCurrencyUser(c)
	if !ok {
		return
	}
	rule, err := h.customCurrencyService.Import(c.Request.Context(), userID, shareToken)
	if err != nil {
		h.respondCustomCurrencyError(c, err)
		return
	}
	c.JSON(http.StatusCreated, rule)
}
