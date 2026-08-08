package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// SetDocumentImageService wires up the licence/credential image subsystem.
func (h *APIHandler) SetDocumentImageService(s *service.DocumentImageService) {
	h.documentImageService = s
}

// ── Licence images ────────────────────────────────────────────────────────

// ListLicenseImages implements GET /licenses/{licenseId}/images
func (h *APIHandler) ListLicenseImages(c *gin.Context, licenseId generated.LicenseId) {
	h.listDocumentImages(c, models.DocumentSubjectLicense, uuid.UUID(licenseId))
}

// UploadLicenseImage implements POST /licenses/{licenseId}/images
func (h *APIHandler) UploadLicenseImage(c *gin.Context, licenseId generated.LicenseId) {
	h.uploadDocumentImage(c, models.DocumentSubjectLicense, uuid.UUID(licenseId))
}

// GetLicenseImage implements GET /licenses/{licenseId}/images/{imageId}
func (h *APIHandler) GetLicenseImage(c *gin.Context, licenseId generated.LicenseId, imageId generated.DocumentImageId) {
	h.getDocumentImage(c, models.DocumentSubjectLicense, uuid.UUID(licenseId), uuid.UUID(imageId))
}

// DeleteLicenseImage implements DELETE /licenses/{licenseId}/images/{imageId}
func (h *APIHandler) DeleteLicenseImage(c *gin.Context, licenseId generated.LicenseId, imageId generated.DocumentImageId) {
	h.deleteDocumentImage(c, models.DocumentSubjectLicense, uuid.UUID(licenseId), uuid.UUID(imageId))
}

// ── Credential images ─────────────────────────────────────────────────────

// ListCredentialImages implements GET /credentials/{credentialId}/images
func (h *APIHandler) ListCredentialImages(c *gin.Context, credentialId generated.CredentialId) {
	h.listDocumentImages(c, models.DocumentSubjectCredential, uuid.UUID(credentialId))
}

// UploadCredentialImage implements POST /credentials/{credentialId}/images
func (h *APIHandler) UploadCredentialImage(c *gin.Context, credentialId generated.CredentialId) {
	h.uploadDocumentImage(c, models.DocumentSubjectCredential, uuid.UUID(credentialId))
}

// GetCredentialImage implements GET /credentials/{credentialId}/images/{imageId}
func (h *APIHandler) GetCredentialImage(c *gin.Context, credentialId generated.CredentialId, imageId generated.DocumentImageId) {
	h.getDocumentImage(c, models.DocumentSubjectCredential, uuid.UUID(credentialId), uuid.UUID(imageId))
}

// DeleteCredentialImage implements DELETE /credentials/{credentialId}/images/{imageId}
func (h *APIHandler) DeleteCredentialImage(c *gin.Context, credentialId generated.CredentialId, imageId generated.DocumentImageId) {
	h.deleteDocumentImage(c, models.DocumentSubjectCredential, uuid.UUID(credentialId), uuid.UUID(imageId))
}

// ── Shared implementations ────────────────────────────────────────────────
//
// Licences and credentials get separate URLs because they are separate
// resources, but the behaviour behind them is identical, so the eight handlers
// above are thin adapters over these four.

func (h *APIHandler) listDocumentImages(c *gin.Context, subject models.DocumentSubjectType, subjectID uuid.UUID) {
	userID, ok := h.documentImageCaller(c)
	if !ok {
		return
	}
	images, err := h.documentImageService.List(c.Request.Context(), userID, subject, subjectID)
	if err != nil {
		h.sendDocumentImageError(c, err)
		return
	}
	result := make([]generated.DocumentImage, 0, len(images))
	for _, img := range images {
		result = append(result, convertToGeneratedDocumentImage(img))
	}
	c.JSON(http.StatusOK, result)
}

func (h *APIHandler) uploadDocumentImage(c *gin.Context, subject models.DocumentSubjectType, subjectID uuid.UUID) {
	userID, ok := h.documentImageCaller(c)
	if !ok {
		return
	}

	// Refuse a disabled feature before touching the multipart body: with the
	// switch off we must not spool an attacker's megabytes to disk just to
	// tell them no.
	if !h.documentImageService.Enabled() {
		h.sendDocumentImageError(c, service.ErrDocumentImagesDisabled)
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		h.sendError(c, http.StatusBadRequest, "A file field is required")
		return
	}
	defer func() { _ = file.Close() }()

	// Read one byte past the cap so an oversized upload is detected here
	// rather than by materializing all of it. header.Size is client-declared
	// and is only used for the cheap early rejection below.
	if header.Size > models.MaxDocumentImageBytes {
		h.sendDocumentImageError(c, service.ErrDocumentImageTooLarge)
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, models.MaxDocumentImageBytes+1))
	if err != nil {
		h.sendError(c, http.StatusBadRequest, "Could not read the uploaded file")
		return
	}

	image, err := h.documentImageService.Upload(c.Request.Context(), userID, subject, subjectID, service.UploadInput{
		Data:     data,
		Filename: header.Filename,
		Caption:  c.PostForm("caption"),
	})
	if err != nil {
		h.sendDocumentImageError(c, err)
		return
	}
	c.JSON(http.StatusCreated, convertToGeneratedDocumentImage(image))
}

func (h *APIHandler) getDocumentImage(c *gin.Context, subject models.DocumentSubjectType, subjectID, imageID uuid.UUID) {
	userID, ok := h.documentImageCaller(c)
	if !ok {
		return
	}
	image, err := h.documentImageService.Get(c.Request.Context(), userID, subject, subjectID, imageID)
	if err != nil {
		h.sendDocumentImageError(c, err)
		return
	}

	// These are scans of identity documents fetched with a bearer token.
	// Keeping them out of shared caches and off disk costs nothing — the
	// client holds the decoded blob for as long as it needs it.
	c.Header("Cache-Control", "private, no-store")
	if image.Filename != nil {
		c.Header("Content-Disposition", fmt.Sprintf("inline; filename=%q", contentDispositionSafe(*image.Filename)))
	}
	c.Data(http.StatusOK, image.ContentType, image.Data)
}

func (h *APIHandler) deleteDocumentImage(c *gin.Context, subject models.DocumentSubjectType, subjectID, imageID uuid.UUID) {
	userID, ok := h.documentImageCaller(c)
	if !ok {
		return
	}
	if err := h.documentImageService.Delete(c.Request.Context(), userID, subject, subjectID, imageID); err != nil {
		h.sendDocumentImageError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// documentImageCaller resolves the authenticated user and confirms the feature
// is wired up at all, writing the error response itself when it is not.
func (h *APIHandler) documentImageCaller(c *gin.Context) (uuid.UUID, bool) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return uuid.Nil, false
	}
	if h.documentImageService == nil {
		h.sendError(c, http.StatusForbidden, service.ErrDocumentImagesDisabled.Error())
		return uuid.Nil, false
	}
	return userID, true
}

// sendDocumentImageError maps the service's sentinel errors onto status codes.
// "Not yours" and "does not exist" deliberately share the 404 so a caller
// cannot enumerate other users' documents.
func (h *APIHandler) sendDocumentImageError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrDocumentImagesDisabled):
		h.sendError(c, http.StatusForbidden, err.Error())
	case errors.Is(err, service.ErrDocumentSubjectNotFound), errors.Is(err, service.ErrDocumentImageNotFound):
		h.sendError(c, http.StatusNotFound, "Not found")
	case errors.Is(err, service.ErrDocumentImageTooLarge):
		h.sendError(c, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("Image exceeds the maximum size of %d MB", models.MaxDocumentImageBytes/(1024*1024)))
	case errors.Is(err, service.ErrDocumentImageLimitReached):
		h.sendError(c, http.StatusConflict,
			fmt.Sprintf("This document already has the maximum of %d images", models.MaxDocumentImagesPerSubject))
	case errors.Is(err, service.ErrDocumentImageEmpty),
		errors.Is(err, service.ErrDocumentImageUnsupported),
		errors.Is(err, service.ErrDocumentImageCorrupt),
		errors.Is(err, service.ErrDocumentImageTooManyPixel):
		h.sendError(c, http.StatusBadRequest, err.Error())
	default:
		h.sendError(c, http.StatusInternalServerError, "Failed to process document image")
	}
}

// contentDispositionSafe strips the characters that would let a stored
// filename break out of the quoted header value. The name is already
// sanitized on the way in; this is the second belt on the way out.
func contentDispositionSafe(name string) string {
	return strings.Map(func(r rune) rune {
		if r == '"' || r == '\\' || r == '\r' || r == '\n' || r < 0x20 || r > 0x7e {
			return -1
		}
		return r
	}, name)
}

func convertToGeneratedDocumentImage(img *models.DocumentImage) generated.DocumentImage {
	out := generated.DocumentImage{
		Id:          openapi_types.UUID(img.ID),
		ContentType: generated.DocumentImageContentType(img.ContentType),
		ByteSize:    img.ByteSize,
		Width:       img.Width,
		Height:      img.Height,
		Filename:    img.Filename,
		Caption:     img.Caption,
		CreatedAt:   img.CreatedAt,
		UpdatedAt:   img.UpdatedAt,
	}
	if img.LicenseID != nil {
		id := openapi_types.UUID(*img.LicenseID)
		out.LicenseId = &id
	}
	if img.CredentialID != nil {
		id := openapi_types.UUID(*img.CredentialID)
		out.CredentialId = &id
	}
	return out
}

// GetFeatures implements GET /features — the capability probe a client uses to
// decide whether to render the image UI at all.
func (h *APIHandler) GetFeatures(c *gin.Context) {
	if _, err := h.getUserIDFromContext(c); err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var features generated.Features
	features.DocumentImages.Enabled = h.documentImageService != nil && h.documentImageService.Enabled()
	features.DocumentImages.MaxBytes = models.MaxDocumentImageBytes
	features.DocumentImages.MaxPerDocument = models.MaxDocumentImagesPerSubject
	features.DocumentImages.AllowedContentTypes = models.AllowedDocumentImageContentTypes

	c.JSON(http.StatusOK, features)
}
