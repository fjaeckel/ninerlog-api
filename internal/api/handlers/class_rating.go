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

func (h *APIHandler) ListClassRatings(c *gin.Context, licenseId generated.LicenseId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	licenseID := uuid.UUID(licenseId)

	ratings, err := h.classRatingService.ListClassRatings(c.Request.Context(), licenseID, userID)
	if err != nil {
		if errors.Is(err, service.ErrLicenseNotFound) || errors.Is(err, service.ErrUnauthorizedAccess) {
			h.sendError(c, http.StatusNotFound, "License not found")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to list class ratings")
		return
	}

	if ratings == nil {
		ratings = []*models.ClassRating{}
	}
	c.JSON(http.StatusOK, ratings)
}

func (h *APIHandler) CreateClassRating(c *gin.Context, licenseId generated.LicenseId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	licenseID := uuid.UUID(licenseId)

	var req struct {
		ClassType  string  `json:"classType" binding:"required"`
		IssueDate  string  `json:"issueDate" binding:"required"`
		ExpiryDate *string `json:"expiryDate"`
		Notes      *string `json:"notes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	issueDate, err := time.Parse("2006-01-02", req.IssueDate)
	if err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid issue date format")
		return
	}

	cr := &models.ClassRating{
		LicenseID: licenseID,
		ClassType: models.ClassType(req.ClassType),
		IssueDate: issueDate,
		Notes:     req.Notes,
	}

	if req.ExpiryDate != nil && *req.ExpiryDate != "" {
		expiry, err := time.Parse("2006-01-02", *req.ExpiryDate)
		if err != nil {
			h.sendError(c, http.StatusBadRequest, "Invalid expiry date format")
			return
		}
		cr.ExpiryDate = &expiry
	}

	if err := h.classRatingService.CreateClassRating(c.Request.Context(), cr, userID); err != nil {
		if errors.Is(err, service.ErrInvalidClassType) {
			h.sendError(c, http.StatusBadRequest, "Invalid class type")
			return
		}
		if errors.Is(err, service.ErrLicenseNotFound) || errors.Is(err, service.ErrUnauthorizedAccess) {
			h.sendError(c, http.StatusNotFound, "License not found")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to create class rating")
		return
	}

	c.JSON(http.StatusCreated, cr)
}

func (h *APIHandler) UpdateClassRating(c *gin.Context, licenseId generated.LicenseId, ratingId openapi_types.UUID) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	ratingID := uuid.UUID(ratingId)

	var req struct {
		IssueDate  *string `json:"issueDate"`
		ExpiryDate *string `json:"expiryDate"`
		Notes      *string `json:"notes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	cr := &models.ClassRating{ID: ratingID}
	if req.IssueDate != nil {
		d, err := time.Parse("2006-01-02", *req.IssueDate)
		if err != nil {
			h.sendError(c, http.StatusBadRequest, "Invalid issue date")
			return
		}
		cr.IssueDate = d
	}
	if req.ExpiryDate != nil {
		if *req.ExpiryDate == "" {
			cr.ExpiryDate = nil
		} else {
			d, err := time.Parse("2006-01-02", *req.ExpiryDate)
			if err != nil {
				h.sendError(c, http.StatusBadRequest, "Invalid expiry date")
				return
			}
			cr.ExpiryDate = &d
		}
	}
	cr.Notes = req.Notes

	if err := h.classRatingService.UpdateClassRating(c.Request.Context(), cr, userID); err != nil {
		if errors.Is(err, service.ErrClassRatingNotFound) || errors.Is(err, service.ErrUnauthorizedClassRating) {
			h.sendError(c, http.StatusNotFound, "Class rating not found")
			return
		}
		h.sendError(c, http.StatusBadRequest, "Failed to update class rating")
		return
	}

	c.JSON(http.StatusOK, cr)
}

func (h *APIHandler) DeleteClassRating(c *gin.Context, licenseId generated.LicenseId, ratingId openapi_types.UUID) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	ratingID := uuid.UUID(ratingId)

	if err := h.classRatingService.DeleteClassRating(c.Request.Context(), ratingID, userID); err != nil {
		if errors.Is(err, service.ErrClassRatingNotFound) || errors.Is(err, service.ErrUnauthorizedClassRating) {
			h.sendError(c, http.StatusNotFound, "Class rating not found")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to delete class rating")
		return
	}

	c.Status(http.StatusNoContent)
}
