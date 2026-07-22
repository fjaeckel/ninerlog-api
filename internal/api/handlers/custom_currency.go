package handlers

import (
	"errors"
	"net/http"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service/currency"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CustomCurrencyHandler serves the user-authored currency rule endpoints. It is
// self-contained (its own service + auth) and registered on the authenticated
// /api group, so every route already has a validated userID in context.
type CustomCurrencyHandler struct {
	service *currency.CustomService
}

// NewCustomCurrencyHandler creates the handler.
func NewCustomCurrencyHandler(service *currency.CustomService) *CustomCurrencyHandler {
	return &CustomCurrencyHandler{service: service}
}

// RegisterCustomCurrencyRoutes wires the custom currency routes onto the group.
func RegisterCustomCurrencyRoutes(api *gin.RouterGroup, h *CustomCurrencyHandler) {
	g := api.Group("/custom-currency")
	g.GET("", h.List)
	g.POST("", h.Create)
	g.POST("/preview", h.Preview)
	g.GET("/shared/:token", h.GetShared)
	g.POST("/shared/:token/import", h.Import)
	g.GET("/:id", h.Get)
	g.PUT("/:id", h.Update)
	g.DELETE("/:id", h.Delete)
	g.POST("/:id/share", h.EnableShare)
	g.DELETE("/:id/share", h.DisableShare)
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

// userID reads the authenticated user set by AuthMiddleware. Returns false and
// writes a 401 if it is missing.
func (h *CustomCurrencyHandler) userID(c *gin.Context) (uuid.UUID, bool) {
	v, ok := c.Get("userID")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return uuid.Nil, false
	}
	id, ok := v.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return uuid.Nil, false
	}
	return id, true
}

// respondError maps service errors to HTTP responses. Validation errors surface
// their message; not-found is 404; everything else is a generic 500.
func (h *CustomCurrencyHandler) respondError(c *gin.Context, err error) {
	switch {
	case currency.IsValidationError(err):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, repository.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process custom currency rule"})
	}
}

func (h *CustomCurrencyHandler) parseID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid rule id"})
		return uuid.Nil, false
	}
	return id, true
}

// List handles GET /custom-currency.
func (h *CustomCurrencyHandler) List(c *gin.Context) {
	userID, ok := h.userID(c)
	if !ok {
		return
	}
	rules, err := h.service.List(c.Request.Context(), userID)
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, rules)
}

// Get handles GET /custom-currency/:id.
func (h *CustomCurrencyHandler) Get(c *gin.Context) {
	userID, ok := h.userID(c)
	if !ok {
		return
	}
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	rule, err := h.service.Get(c.Request.Context(), userID, id)
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, rule)
}

// Create handles POST /custom-currency.
func (h *CustomCurrencyHandler) Create(c *gin.Context) {
	userID, ok := h.userID(c)
	if !ok {
		return
	}
	var req customRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	rule, err := h.service.Create(c.Request.Context(), userID, req.toInput())
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, rule)
}

// Update handles PUT /custom-currency/:id.
func (h *CustomCurrencyHandler) Update(c *gin.Context) {
	userID, ok := h.userID(c)
	if !ok {
		return
	}
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	var req customRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	rule, err := h.service.Update(c.Request.Context(), userID, id, req.toInput())
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, rule)
}

// Delete handles DELETE /custom-currency/:id.
func (h *CustomCurrencyHandler) Delete(c *gin.Context) {
	userID, ok := h.userID(c)
	if !ok {
		return
	}
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), userID, id); err != nil {
		h.respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Preview handles POST /custom-currency/preview.
func (h *CustomCurrencyHandler) Preview(c *gin.Context) {
	userID, ok := h.userID(c)
	if !ok {
		return
	}
	var req previewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	result, err := h.service.Preview(c.Request.Context(), userID, req.Definition)
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// EnableShare handles POST /custom-currency/:id/share.
func (h *CustomCurrencyHandler) EnableShare(c *gin.Context) {
	h.setShare(c, true)
}

// DisableShare handles DELETE /custom-currency/:id/share.
func (h *CustomCurrencyHandler) DisableShare(c *gin.Context) {
	h.setShare(c, false)
}

func (h *CustomCurrencyHandler) setShare(c *gin.Context, shared bool) {
	userID, ok := h.userID(c)
	if !ok {
		return
	}
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	rule, err := h.service.SetShared(c.Request.Context(), userID, id, shared)
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, rule)
}

// GetShared handles GET /custom-currency/shared/:token — a read-only preview.
func (h *CustomCurrencyHandler) GetShared(c *gin.Context) {
	if _, ok := h.userID(c); !ok {
		return
	}
	view, err := h.service.GetShared(c.Request.Context(), c.Param("token"))
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

// Import handles POST /custom-currency/shared/:token/import.
func (h *CustomCurrencyHandler) Import(c *gin.Context) {
	userID, ok := h.userID(c)
	if !ok {
		return
	}
	rule, err := h.service.Import(c.Request.Context(), userID, c.Param("token"))
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, rule)
}
