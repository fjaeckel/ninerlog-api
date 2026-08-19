package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	_ "image/jpeg" // registers the JPEG decoder used by image.DecodeConfig
	_ "image/png"  // registers the PNG decoder used by image.DecodeConfig
	"net/http"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

var (
	// ErrDocumentFilesDisabled is returned from every image endpoint — reads
	// included — when the feature is switched off (DOCUMENT_FILES_ENABLED=
	// false). Stored rows are left untouched and become reachable again when
	// it is turned back on.
	ErrDocumentFilesDisabled = errors.New("document image uploads are disabled on this server")

	// ErrDocumentFileNotFound covers both "no such image" and "that image
	// belongs to a different document or user"; the caller cannot tell them
	// apart.
	ErrDocumentFileNotFound = errors.New("document image not found")

	// ErrDocumentSubjectNotFound means the licence or credential the image
	// would hang off does not exist or is not the caller's.
	ErrDocumentSubjectNotFound = errors.New("document not found")

	ErrDocumentFileEmpty        = errors.New("image file is empty")
	ErrDocumentFileTooLarge     = errors.New("image exceeds the maximum size")
	ErrDocumentFileUnsupported  = errors.New("unsupported image format")
	ErrDocumentFileCorrupt      = errors.New("image could not be decoded")
	ErrDocumentFileTooManyPixel = errors.New("image resolution is too large")
	ErrDocumentFileLimitReached = errors.New("maximum number of images for this document reached")
)

// DocumentFileService owns reference photos attached to licences and
// credentials: the feature switch, upload validation, ownership checks and the
// per-document cap. Every method resolves the subject through the
// licence/credential repository first; an image is only addressable through
// the document it belongs to.
type DocumentFileService struct {
	imageRepo      repository.DocumentFileRepository
	licenseRepo    repository.LicenseRepository
	credentialRepo repository.CredentialRepository
	enabled        bool
}

func NewDocumentFileService(
	imageRepo repository.DocumentFileRepository,
	licenseRepo repository.LicenseRepository,
	credentialRepo repository.CredentialRepository,
	enabled bool,
) *DocumentFileService {
	return &DocumentFileService{
		imageRepo:      imageRepo,
		licenseRepo:    licenseRepo,
		credentialRepo: credentialRepo,
		enabled:        enabled,
	}
}

// Enabled reports whether the feature is switched on. Handlers use it to
// answer GET /features.
func (s *DocumentFileService) Enabled() bool { return s.enabled }

// UploadInput is one candidate image as it arrives from the handler. Data is
// the raw file; the content type is derived from it, never taken from the
// client's declared part header.
type UploadInput struct {
	Data     []byte
	Filename string
	Caption  string
}

// verifySubject proves the caller owns the licence/credential the request
// addresses. Both misses collapse to ErrDocumentSubjectNotFound.
func (s *DocumentFileService) verifySubject(ctx context.Context, userID uuid.UUID, subject models.DocumentSubjectType, subjectID uuid.UUID) error {
	switch subject {
	case models.DocumentSubjectLicense:
		license, err := s.licenseRepo.GetByID(ctx, subjectID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return ErrDocumentSubjectNotFound
			}
			return err
		}
		if license.UserID != userID {
			return ErrDocumentSubjectNotFound
		}
		return nil
	case models.DocumentSubjectCredential:
		credential, err := s.credentialRepo.GetByID(ctx, subjectID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return ErrDocumentSubjectNotFound
			}
			return err
		}
		if credential.UserID != userID {
			return ErrDocumentSubjectNotFound
		}
		return nil
	default:
		return ErrDocumentSubjectNotFound
	}
}

// Upload validates and stores one image against a licence or credential.
func (s *DocumentFileService) Upload(ctx context.Context, userID uuid.UUID, subject models.DocumentSubjectType, subjectID uuid.UUID, in UploadInput) (*models.DocumentFile, error) {
	if !s.enabled {
		return nil, ErrDocumentFilesDisabled
	}
	if err := s.verifySubject(ctx, userID, subject, subjectID); err != nil {
		return nil, err
	}

	contentType, width, height, err := inspectFile(in.Data)
	if err != nil {
		return nil, err
	}

	// Width and height are nil for formats without intrinsic pixel
	// dimensions (PDF).
	img := &models.DocumentFile{
		UserID:      userID,
		ContentType: contentType,
		ByteSize:    len(in.Data),
		Width:       width,
		Height:      height,
		Data:        in.Data,
	}
	if name := sanitizeFilename(in.Filename); name != "" {
		img.Filename = &name
	}
	if caption := truncateRunes(strings.TrimSpace(in.Caption), models.MaxDocumentFileCaptionLen); caption != "" {
		img.Caption = &caption
	}
	switch subject {
	case models.DocumentSubjectLicense:
		id := subjectID
		img.LicenseID = &id
	case models.DocumentSubjectCredential:
		id := subjectID
		img.CredentialID = &id
	}

	if err := s.imageRepo.Create(ctx, img, models.MaxDocumentFilesPerSubject); err != nil {
		if errors.Is(err, repository.ErrDocumentFileLimit) {
			return nil, ErrDocumentFileLimitReached
		}
		return nil, err
	}

	// The stored payload is not part of the create response.
	img.Data = nil
	return img, nil
}

// List returns the metadata for a document's images, oldest first.
func (s *DocumentFileService) List(ctx context.Context, userID uuid.UUID, subject models.DocumentSubjectType, subjectID uuid.UUID) ([]*models.DocumentFile, error) {
	if !s.enabled {
		return nil, ErrDocumentFilesDisabled
	}
	if err := s.verifySubject(ctx, userID, subject, subjectID); err != nil {
		return nil, err
	}
	return s.imageRepo.ListBySubject(ctx, userID, subject, subjectID)
}

// Get returns a single image including its bytes, for the authenticated
// download endpoint.
func (s *DocumentFileService) Get(ctx context.Context, userID uuid.UUID, subject models.DocumentSubjectType, subjectID, imageID uuid.UUID) (*models.DocumentFile, error) {
	if !s.enabled {
		return nil, ErrDocumentFilesDisabled
	}
	if err := s.verifySubject(ctx, userID, subject, subjectID); err != nil {
		return nil, err
	}
	img, err := s.imageRepo.GetWithData(ctx, userID, subject, subjectID, imageID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrDocumentFileNotFound
		}
		return nil, err
	}
	return img, nil
}

// Delete removes one image from a document.
func (s *DocumentFileService) Delete(ctx context.Context, userID uuid.UUID, subject models.DocumentSubjectType, subjectID, imageID uuid.UUID) error {
	if !s.enabled {
		return ErrDocumentFilesDisabled
	}
	if err := s.verifySubject(ctx, userID, subject, subjectID); err != nil {
		return err
	}
	if err := s.imageRepo.Delete(ctx, userID, subject, subjectID, imageID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrDocumentFileNotFound
		}
		return err
	}
	return nil
}

// inspectFile decides whether a byte slice is storable and returns its
// sniffed content type and (for images) its dimensions. The declared
// Content-Type of the multipart part is ignored entirely.
func inspectFile(data []byte) (contentType string, width, height *int, err error) {
	if len(data) == 0 {
		return "", nil, nil, ErrDocumentFileEmpty
	}
	if len(data) > models.MaxDocumentFileBytes {
		return "", nil, nil, ErrDocumentFileTooLarge
	}

	sniffed := http.DetectContentType(data)
	switch {
	case models.ContentTypeIsImage(sniffed):
		w, h, err := inspectImage(data, sniffed)
		if err != nil {
			return "", nil, nil, err
		}
		return sniffed, &w, &h, nil
	case sniffed == models.ContentTypePDF:
		if err := inspectPDF(data); err != nil {
			return "", nil, nil, err
		}
		// A PDF has no single intrinsic pixel size; width/height stay null.
		return sniffed, nil, nil, nil
	default:
		return "", nil, nil, ErrDocumentFileUnsupported
	}
}

// inspectImage verifies a raster image header and returns its declared
// dimensions. It validates the HEADER, not the whole file: the header must
// parse as the sniffed format and its declared dimensions must be within the
// pixel cap; a valid header followed by arbitrary bytes is accepted and
// stored verbatim. See
// TestDocumentFileUpload_ValidatesTheHeaderNotTheWholeFile.
func inspectImage(data []byte, sniffed string) (width, height int, err error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, ErrDocumentFileCorrupt
	}
	// The decoder that claimed the file must match the sniffed type.
	if "image/"+format != sniffed {
		return 0, 0, ErrDocumentFileUnsupported
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0, ErrDocumentFileCorrupt
	}
	if int64(cfg.Width)*int64(cfg.Height) > models.MaxDocumentFilePixels {
		return 0, 0, ErrDocumentFileTooManyPixel
	}
	return cfg.Width, cfg.Height, nil
}

// inspectPDF checks structural markers only: the %PDF- signature at the front
// and a %%EOF trailer within the last few KB. A structurally valid PDF
// carrying JavaScript or an embedded payload is stored.
func inspectPDF(data []byte) error {
	if !bytes.HasPrefix(data, models.PDFMagic) {
		return ErrDocumentFileUnsupported
	}
	tail := data
	if len(tail) > pdfTrailerSearchWindow {
		tail = tail[len(tail)-pdfTrailerSearchWindow:]
	}
	if !bytes.Contains(tail, models.PDFTrailer) {
		return ErrDocumentFileCorrupt
	}
	return nil
}

// pdfTrailerSearchWindow is how far back from the end of the file to look for
// %%EOF.
const pdfTrailerSearchWindow = 4096

// sanitizeFilename reduces a client-supplied filename to a display-safe
// basename, stripping directory components and control characters. It is
// never used to build a path.
func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	// Cut Windows-style separators too; filepath.Base only handles "/" on
	// Linux.
	if idx := strings.LastIndexAny(name, `/\`); idx >= 0 {
		name = name[idx+1:]
	}
	name = filepath.Base(name)
	name = strings.Map(func(r rune) rune {
		if r == unicode.ReplacementChar || unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "." || name == ".." {
		return ""
	}
	return truncateRunes(name, models.MaxDocumentFileFilenameLen)
}

// truncateRunes caps a string at max runes (not bytes).
func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
