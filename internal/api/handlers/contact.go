package handlers

import (
	"errors"
	"net/http"

	"github.com/fjaeckel/pilotlog-api/internal/api/generated"
	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/fjaeckel/pilotlog-api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// ListContacts handles GET /contacts
func (h *APIHandler) ListContacts(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	contacts, err := h.contactService.ListContacts(c.Request.Context(), userID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve contacts")
		return
	}
	if contacts == nil {
		contacts = []*models.Contact{}
	}
	c.JSON(http.StatusOK, contacts)
}

// CreateContact handles POST /contacts
func (h *APIHandler) CreateContact(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var req struct {
		Name  string  `json:"name" binding:"required"`
		Email *string `json:"email"`
		Phone *string `json:"phone"`
		Notes *string `json:"notes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	contact := &models.Contact{
		UserID: userID, Name: req.Name, Email: req.Email, Phone: req.Phone, Notes: req.Notes,
	}
	if err := h.contactService.CreateContact(c.Request.Context(), contact); err != nil {
		h.sendError(c, http.StatusBadRequest, "Failed to create contact")
		return
	}
	c.JSON(http.StatusCreated, contact)
}

// GetContact handles GET /contacts/{contactId}
func (h *APIHandler) GetContact(c *gin.Context, contactId openapi_types.UUID) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	contact, err := h.contactService.GetContact(c.Request.Context(), uuid.UUID(contactId), userID)
	if err != nil {
		if errors.Is(err, service.ErrContactNotFound) || errors.Is(err, service.ErrUnauthorizedContact) {
			h.sendError(c, http.StatusNotFound, "Contact not found")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve contact")
		return
	}
	c.JSON(http.StatusOK, contact)
}

// UpdateContact handles PUT /contacts/{contactId}
func (h *APIHandler) UpdateContact(c *gin.Context, contactId openapi_types.UUID) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var req struct {
		Name  string  `json:"name" binding:"required"`
		Email *string `json:"email"`
		Phone *string `json:"phone"`
		Notes *string `json:"notes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	contact := &models.Contact{
		ID: uuid.UUID(contactId), Name: req.Name, Email: req.Email, Phone: req.Phone, Notes: req.Notes,
	}
	if err := h.contactService.UpdateContact(c.Request.Context(), contact, userID); err != nil {
		if errors.Is(err, service.ErrContactNotFound) || errors.Is(err, service.ErrUnauthorizedContact) {
			h.sendError(c, http.StatusNotFound, "Contact not found")
			return
		}
		h.sendError(c, http.StatusBadRequest, "Failed to update contact")
		return
	}
	c.JSON(http.StatusOK, contact)
}

// DeleteContact handles DELETE /contacts/{contactId}
func (h *APIHandler) DeleteContact(c *gin.Context, contactId openapi_types.UUID) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if err := h.contactService.DeleteContact(c.Request.Context(), uuid.UUID(contactId), userID); err != nil {
		if errors.Is(err, service.ErrContactNotFound) || errors.Is(err, service.ErrUnauthorizedContact) {
			h.sendError(c, http.StatusNotFound, "Contact not found")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to delete contact")
		return
	}
	c.Status(http.StatusNoContent)
}

// SearchContacts handles GET /contacts/search
func (h *APIHandler) SearchContacts(c *gin.Context, params generated.SearchContactsParams) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if params.Q == "" {
		c.JSON(http.StatusOK, []*models.Contact{})
		return
	}
	contacts, err := h.contactService.SearchContacts(c.Request.Context(), userID, params.Q, 10)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to search contacts")
		return
	}
	if contacts == nil {
		contacts = []*models.Contact{}
	}
	c.JSON(http.StatusOK, contacts)
}
