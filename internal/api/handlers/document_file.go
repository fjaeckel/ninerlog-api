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

// SetDocumentFileService wires up the licence/credential image subsystem.
func (h *APIHandler) SetDocumentFileService(s *service.DocumentFileService) {
	h.documentFileService = s
}

// ── Licence files ────────────────────────────────────────────────────────

// ListLicenseFiles implements GET /licenses/{licenseId}/files
func (h *APIHandler) ListLicenseFiles(c *gin.Context, licenseId generated.LicenseId) {
	h.listDocumentFiles(c, models.DocumentSubjectLicense, uuid.UUID(licenseId))
}

// UploadLicenseFile implements POST /licenses/{licenseId}/files
func (h *APIHandler) UploadLicenseFile(c *gin.Context, licenseId generated.LicenseId) {
	h.uploadDocumentFile(c, models.DocumentSubjectLicense, uuid.UUID(licenseId))
}

// GetLicenseFile implements GET /licenses/{licenseId}/files/{fileId}
func (h *APIHandler) GetLicenseFile(c *gin.Context, licenseId generated.LicenseId, fileId generated.DocumentFileId) {
	h.getDocumentFile(c, models.DocumentSubjectLicense, uuid.UUID(licenseId), uuid.UUID(fileId))
}

// DeleteLicenseFile implements DELETE /licenses/{licenseId}/files/{fileId}
func (h *APIHandler) DeleteLicenseFile(c *gin.Context, licenseId generated.LicenseId, fileId generated.DocumentFileId) {
	h.deleteDocumentFile(c, models.DocumentSubjectLicense, uuid.UUID(licenseId), uuid.UUID(fileId))
}

// ── Credential files ─────────────────────────────────────────────────────

// ListCredentialFiles implements GET /credentials/{credentialId}/files
func (h *APIHandler) ListCredentialFiles(c *gin.Context, credentialId generated.CredentialId) {
	h.listDocumentFiles(c, models.DocumentSubjectCredential, uuid.UUID(credentialId))
}

// UploadCredentialFile implements POST /credentials/{credentialId}/files
func (h *APIHandler) UploadCredentialFile(c *gin.Context, credentialId generated.CredentialId) {
	h.uploadDocumentFile(c, models.DocumentSubjectCredential, uuid.UUID(credentialId))
}

// GetCredentialFile implements GET /credentials/{credentialId}/files/{fileId}
func (h *APIHandler) GetCredentialFile(c *gin.Context, credentialId generated.CredentialId, fileId generated.DocumentFileId) {
	h.getDocumentFile(c, models.DocumentSubjectCredential, uuid.UUID(credentialId), uuid.UUID(fileId))
}

// DeleteCredentialFile implements DELETE /credentials/{credentialId}/files/{fileId}
func (h *APIHandler) DeleteCredentialFile(c *gin.Context, credentialId generated.CredentialId, fileId generated.DocumentFileId) {
	h.deleteDocumentFile(c, models.DocumentSubjectCredential, uuid.UUID(credentialId), uuid.UUID(fileId))
}

// ── Shared implementations ────────────────────────────────────────────────
//
// Licences and credentials get separate URLs because they are separate
// resources, but the behaviour behind them is identical, so the eight handlers
// above are thin adapters over these four.

func (h *APIHandler) listDocumentFiles(c *gin.Context, subject models.DocumentSubjectType, subjectID uuid.UUID) {
	userID, ok := h.documentFileCaller(c)
	if !ok {
		return
	}
	images, err := h.documentFileService.List(c.Request.Context(), userID, subject, subjectID)
	if err != nil {
		h.sendDocumentFileError(c, err)
		return
	}
	result := make([]generated.DocumentFile, 0, len(images))
	for _, img := range images {
		result = append(result, convertToGeneratedDocumentFile(img))
	}
	c.JSON(http.StatusOK, result)
}

func (h *APIHandler) uploadDocumentFile(c *gin.Context, subject models.DocumentSubjectType, subjectID uuid.UUID) {
	userID, ok := h.documentFileCaller(c)
	if !ok {
		return
	}

	// Refuse a disabled feature before touching the multipart body: with the
	// switch off we must not spool an attacker's megabytes to disk just to
	// tell them no.
	if !h.documentFileService.Enabled() {
		h.sendDocumentFileError(c, service.ErrDocumentFilesDisabled)
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
	if header.Size > models.MaxDocumentFileBytes {
		h.sendDocumentFileError(c, service.ErrDocumentFileTooLarge)
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, models.MaxDocumentFileBytes+1))
	if err != nil {
		h.sendError(c, http.StatusBadRequest, "Could not read the uploaded file")
		return
	}

	image, err := h.documentFileService.Upload(c.Request.Context(), userID, subject, subjectID, service.UploadInput{
		Data:     data,
		Filename: header.Filename,
		Caption:  c.PostForm("caption"),
	})
	if err != nil {
		h.sendDocumentFileError(c, err)
		return
	}
	c.JSON(http.StatusCreated, convertToGeneratedDocumentFile(image))
}

func (h *APIHandler) getDocumentFile(c *gin.Context, subject models.DocumentSubjectType, subjectID, fileID uuid.UUID) {
	userID, ok := h.documentFileCaller(c)
	if !ok {
		return
	}
	file, err := h.documentFileService.Get(c.Request.Context(), userID, subject, subjectID, fileID)
	if err != nil {
		h.sendDocumentFileError(c, err)
		return
	}

	// These are scans of identity documents fetched with a bearer token.
	// Keeping them out of shared caches and off disk costs nothing — the
	// client holds the decoded blob for as long as it needs it.
	c.Header("Cache-Control", "private, no-store")

	// Images were verified by decoding their header and may be rendered
	// inline. A PDF was not — nothing in the stdlib parses one — and it is an
	// active format that can carry scripts and embedded files, so it always
	// goes out as an attachment. That holds regardless of what the client
	// asks for: the decision belongs on the server, not with the caller.
	disposition := "attachment"
	if models.ContentTypeIsInlineSafe(file.ContentType) {
		disposition = "inline"
	}
	if file.Filename != nil {
		c.Header("Content-Disposition",
			fmt.Sprintf("%s; filename=%q", disposition, contentDispositionSafe(*file.Filename)))
	} else {
		c.Header("Content-Disposition", disposition)
	}

	c.Data(http.StatusOK, file.ContentType, file.Data)
}

func (h *APIHandler) deleteDocumentFile(c *gin.Context, subject models.DocumentSubjectType, subjectID, imageID uuid.UUID) {
	userID, ok := h.documentFileCaller(c)
	if !ok {
		return
	}
	if err := h.documentFileService.Delete(c.Request.Context(), userID, subject, subjectID, imageID); err != nil {
		h.sendDocumentFileError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// documentFileCaller resolves the authenticated user and confirms the feature
// is wired up at all, writing the error response itself when it is not.
func (h *APIHandler) documentFileCaller(c *gin.Context) (uuid.UUID, bool) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return uuid.Nil, false
	}
	if h.documentFileService == nil {
		h.sendError(c, http.StatusForbidden, service.ErrDocumentFilesDisabled.Error())
		return uuid.Nil, false
	}
	return userID, true
}

// sendDocumentFileError maps the service's sentinel errors onto status codes.
// "Not yours" and "does not exist" deliberately share the 404 so a caller
// cannot enumerate other users' documents.
func (h *APIHandler) sendDocumentFileError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrDocumentFilesDisabled):
		h.sendError(c, http.StatusForbidden, err.Error())
	case errors.Is(err, service.ErrDocumentSubjectNotFound), errors.Is(err, service.ErrDocumentFileNotFound):
		h.sendError(c, http.StatusNotFound, "Not found")
	case errors.Is(err, service.ErrDocumentFileTooLarge):
		h.sendError(c, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("Image exceeds the maximum size of %d MB", models.MaxDocumentFileBytes/(1024*1024)))
	case errors.Is(err, service.ErrDocumentFileLimitReached):
		h.sendError(c, http.StatusConflict,
			fmt.Sprintf("This document already has the maximum of %d images", models.MaxDocumentFilesPerSubject))
	case errors.Is(err, service.ErrDocumentFileUnreadable):
		// The row exists and belongs to the caller; the server just cannot open
		// it, which means a key problem on this side. A 500 is the honest
		// answer — nothing the client sends will change it — and the message
		// says so rather than implying the upload was somehow at fault.
		h.sendError(c, http.StatusInternalServerError,
			"This file cannot be read with the server's current encryption key")
	case errors.Is(err, service.ErrDocumentFileEmpty),
		errors.Is(err, service.ErrDocumentFileUnsupported),
		errors.Is(err, service.ErrDocumentFileCorrupt),
		errors.Is(err, service.ErrDocumentFileTooManyPixel):
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

func convertToGeneratedDocumentFile(img *models.DocumentFile) generated.DocumentFile {
	out := generated.DocumentFile{
		Id:          openapi_types.UUID(img.ID),
		ContentType: generated.DocumentFileContentType(img.ContentType),
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
	features.DocumentFiles.Enabled = h.documentFileService != nil && h.documentFileService.Enabled()
	features.DocumentFiles.MaxBytes = models.MaxDocumentFileBytes
	features.DocumentFiles.MaxPerDocument = models.MaxDocumentFilesPerSubject
	features.DocumentFiles.AllowedContentTypes = models.AllowedDocumentFileContentTypes

	c.JSON(http.StatusOK, features)
}
