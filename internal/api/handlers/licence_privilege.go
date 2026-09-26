package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// SetLicencePrivilegeService wires the licence privilege service.
func (h *APIHandler) SetLicencePrivilegeService(s *service.LicencePrivilegeService) {
	h.licencePrivilegeService = s
}

// sendLicencePrivilegeError maps licence privilege service errors to responses.
func (h *APIHandler) sendLicencePrivilegeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrLicenseNotFound), errors.Is(err, service.ErrUnauthorizedAccess):
		h.sendError(c, http.StatusNotFound, "License not found")
	case errors.Is(err, service.ErrLicencePrivilegeNotFound):
		h.sendError(c, http.StatusNotFound, "Licence privilege not found")
	case errors.Is(err, models.ErrInvalidLicencePrivilege), errors.Is(err, models.ErrFieldTooLong):
		h.sendError(c, http.StatusBadRequest, err.Error())
	default:
		h.sendError(c, http.StatusInternalServerError, "Failed to process licence privilege")
	}
}

func dateFromTime(t *time.Time) *openapi_types.Date {
	if t == nil {
		return nil
	}
	return &openapi_types.Date{Time: *t}
}

func timeFromDate(d *openapi_types.Date) *time.Time {
	if d == nil {
		return nil
	}
	t := d.Time
	return &t
}

func convertToGeneratedLicencePrivilege(p *models.LicencePrivilege) generated.LicencePrivilege {
	return generated.LicencePrivilege{
		Id:        openapi_types.UUID(p.ID),
		LicenseId: openapi_types.UUID(p.LicenseID),
		Kind:      generated.LicencePrivilegeKind(p.Kind),
		Detail:    p.Detail,
		IssuedOn:  dateFromTime(p.IssuedOn),
		ExpiresOn: dateFromTime(p.ExpiresOn),
		Notes:     p.Notes,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}

// ListLicencePrivileges implements GET /licenses/{licenseId}/privileges.
func (h *APIHandler) ListLicencePrivileges(c *gin.Context, licenseId generated.LicenseId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	list, err := h.licencePrivilegeService.List(c.Request.Context(), userID, uuid.UUID(licenseId))
	if err != nil {
		h.sendLicencePrivilegeError(c, err)
		return
	}
	out := make([]generated.LicencePrivilege, 0, len(list))
	for _, p := range list {
		out = append(out, convertToGeneratedLicencePrivilege(p))
	}
	c.JSON(http.StatusOK, out)
}

// CreateLicencePrivilege implements POST /licenses/{licenseId}/privileges.
func (h *APIHandler) CreateLicencePrivilege(c *gin.Context, licenseId generated.LicenseId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var req generated.LicencePrivilegeCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	p, err := h.licencePrivilegeService.Create(c.Request.Context(), userID, uuid.UUID(licenseId), service.LicencePrivilegeInput{
		Kind:      models.LicencePrivilegeKind(req.Kind),
		Detail:    req.Detail,
		IssuedOn:  timeFromDate(req.IssuedOn),
		ExpiresOn: timeFromDate(req.ExpiresOn),
		Notes:     req.Notes,
	})
	if err != nil {
		h.sendLicencePrivilegeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, convertToGeneratedLicencePrivilege(p))
}

// UpdateLicencePrivilege implements PATCH /licenses/{licenseId}/privileges/{privilegeId}.
func (h *APIHandler) UpdateLicencePrivilege(c *gin.Context, licenseId generated.LicenseId, privilegeId generated.PrivilegeId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var req generated.LicencePrivilegeUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	var patch service.LicencePrivilegePatch
	if req.Kind != nil {
		k := models.LicencePrivilegeKind(*req.Kind)
		patch.Kind = &k
	}
	patch.Detail, patch.ClearDetail = nullablePatch(req.Detail)
	patch.Notes, patch.ClearNotes = nullablePatch(req.Notes)
	var issued, expires *openapi_types.Date
	issued, patch.ClearIssuedOn = nullablePatch(req.IssuedOn)
	patch.IssuedOn = timeFromDate(issued)
	expires, patch.ClearExpiresOn = nullablePatch(req.ExpiresOn)
	patch.ExpiresOn = timeFromDate(expires)
	p, err := h.licencePrivilegeService.Update(c.Request.Context(), userID, uuid.UUID(licenseId), uuid.UUID(privilegeId), patch)
	if err != nil {
		h.sendLicencePrivilegeError(c, err)
		return
	}
	c.JSON(http.StatusOK, convertToGeneratedLicencePrivilege(p))
}

// DeleteLicencePrivilege implements DELETE /licenses/{licenseId}/privileges/{privilegeId}.
func (h *APIHandler) DeleteLicencePrivilege(c *gin.Context, licenseId generated.LicenseId, privilegeId generated.PrivilegeId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if err := h.licencePrivilegeService.Delete(c.Request.Context(), userID, uuid.UUID(licenseId), uuid.UUID(privilegeId)); err != nil {
		h.sendLicencePrivilegeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
