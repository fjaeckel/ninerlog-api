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

// ListCredentials implements GET /credentials
func (h *APIHandler) ListCredentials(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	credentials, err := h.credentialService.ListCredentials(c.Request.Context(), userID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve credentials")
		return
	}

	result := make([]generated.Credential, 0, len(credentials))
	for _, cred := range credentials {
		result = append(result, convertToGeneratedCredential(cred))
	}

	c.JSON(http.StatusOK, result)
}

// CreateCredential implements POST /credentials
func (h *APIHandler) CreateCredential(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req generated.CreateCredentialJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	issueDate, err := time.Parse("2006-01-02", req.IssueDate.String())
	if err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid issue date")
		return
	}

	credential := &models.Credential{
		UserID:           userID,
		CredentialType:   models.CredentialType(req.CredentialType),
		IssueDate:        issueDate,
		IssuingAuthority: req.IssuingAuthority,
	}

	if req.CredentialNumber != nil {
		credential.CredentialNumber = req.CredentialNumber
	}
	if req.ExpiryDate != nil {
		expiry, err := time.Parse("2006-01-02", req.ExpiryDate.String())
		if err != nil {
			h.sendError(c, http.StatusBadRequest, "Invalid expiry date")
			return
		}
		credential.ExpiryDate = &expiry
	}
	if req.Notes != nil {
		credential.Notes = req.Notes
	}

	if err := h.credentialService.CreateCredential(c.Request.Context(), credential); err != nil {
		if errors.Is(err, service.ErrExpiryBeforeIssue) {
			h.sendError(c, http.StatusBadRequest, err.Error())
			return
		}
		h.sendError(c, http.StatusBadRequest, "Failed to create credential")
		return
	}

	c.JSON(http.StatusCreated, convertToGeneratedCredential(credential))
}

// GetCredential implements GET /credentials/{credentialId}
func (h *APIHandler) GetCredential(c *gin.Context, credentialId generated.CredentialId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	credential, err := h.credentialService.GetCredential(c.Request.Context(), uuid.UUID(credentialId), userID)
	if err != nil {
		if errors.Is(err, service.ErrCredentialNotFound) || errors.Is(err, service.ErrUnauthorizedCredential) {
			h.sendError(c, http.StatusNotFound, "Credential not found")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve credential")
		return
	}

	c.JSON(http.StatusOK, convertToGeneratedCredential(credential))
}

// UpdateCredential implements PATCH /credentials/{credentialId}
func (h *APIHandler) UpdateCredential(c *gin.Context, credentialId generated.CredentialId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req generated.UpdateCredentialJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	credential, err := h.credentialService.GetCredential(c.Request.Context(), uuid.UUID(credentialId), userID)
	if err != nil {
		if errors.Is(err, service.ErrCredentialNotFound) || errors.Is(err, service.ErrUnauthorizedCredential) {
			h.sendError(c, http.StatusNotFound, "Credential not found")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve credential")
		return
	}

	if req.CredentialType != nil {
		credential.CredentialType = models.CredentialType(*req.CredentialType)
	}
	if req.CredentialNumber != nil {
		credential.CredentialNumber = req.CredentialNumber
	}
	if req.IssueDate != nil {
		issueDate, err := time.Parse("2006-01-02", req.IssueDate.String())
		if err != nil {
			h.sendError(c, http.StatusBadRequest, "Invalid issue date")
			return
		}
		credential.IssueDate = issueDate
	}
	if req.ExpiryDate != nil {
		expiry, err := time.Parse("2006-01-02", req.ExpiryDate.String())
		if err != nil {
			h.sendError(c, http.StatusBadRequest, "Invalid expiry date")
			return
		}
		credential.ExpiryDate = &expiry
	}
	if req.IssuingAuthority != nil {
		credential.IssuingAuthority = *req.IssuingAuthority
	}
	if req.Notes != nil {
		credential.Notes = req.Notes
	}

	if err := h.credentialService.UpdateCredential(c.Request.Context(), credential, userID); err != nil {
		if errors.Is(err, service.ErrExpiryBeforeIssue) {
			h.sendError(c, http.StatusBadRequest, err.Error())
			return
		}
		h.sendError(c, http.StatusBadRequest, "Failed to update credential")
		return
	}

	c.JSON(http.StatusOK, convertToGeneratedCredential(credential))
}

// DeleteCredential implements DELETE /credentials/{credentialId}
func (h *APIHandler) DeleteCredential(c *gin.Context, credentialId generated.CredentialId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	if err := h.credentialService.DeleteCredential(c.Request.Context(), uuid.UUID(credentialId), userID); err != nil {
		if errors.Is(err, service.ErrCredentialNotFound) || errors.Is(err, service.ErrUnauthorizedCredential) {
			h.sendError(c, http.StatusNotFound, "Credential not found")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to delete credential")
		return
	}

	c.Status(http.StatusNoContent)
}

func convertToGeneratedCredential(c *models.Credential) generated.Credential {
	cred := generated.Credential{
		Id:               openapi_types.UUID(c.ID),
		UserId:           openapi_types.UUID(c.UserID),
		CredentialType:   generated.CredentialType(c.CredentialType),
		IssueDate:        openapi_types.Date{Time: c.IssueDate},
		IssuingAuthority: c.IssuingAuthority,
		CreatedAt:        c.CreatedAt,
		UpdatedAt:        c.UpdatedAt,
	}

	if c.CredentialNumber != nil {
		cred.CredentialNumber = c.CredentialNumber
	}
	if c.ExpiryDate != nil {
		expiryDate := openapi_types.Date{Time: *c.ExpiryDate}
		cred.ExpiryDate = &expiryDate
	}
	if c.Notes != nil {
		cred.Notes = c.Notes
	}

	return cred
}
