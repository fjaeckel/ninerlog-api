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
	// ErrDocumentImagesDisabled is returned from every image endpoint when the
	// feature is switched off (DOCUMENT_IMAGES_ENABLED=false). It covers reads
	// as well as writes: serving multi-megabyte blobs is itself the bandwidth
	// half of the abuse surface an operator turns this off to close. Stored
	// rows are left untouched and become reachable again when it is turned
	// back on.
	ErrDocumentImagesDisabled = errors.New("document image uploads are disabled on this server")

	// ErrDocumentImageNotFound covers both "no such image" and "that image
	// belongs to a different document or user" — the caller cannot tell them
	// apart, so an id cannot be probed through someone else's URL.
	ErrDocumentImageNotFound = errors.New("document image not found")

	// ErrDocumentSubjectNotFound means the licence or credential the image
	// would hang off does not exist or is not the caller's.
	ErrDocumentSubjectNotFound = errors.New("document not found")

	ErrDocumentImageEmpty        = errors.New("image file is empty")
	ErrDocumentImageTooLarge     = errors.New("image exceeds the maximum size")
	ErrDocumentImageUnsupported  = errors.New("unsupported image format")
	ErrDocumentImageCorrupt      = errors.New("image could not be decoded")
	ErrDocumentImageTooManyPixel = errors.New("image resolution is too large")
	ErrDocumentImageLimitReached = errors.New("maximum number of images for this document reached")
)

// DocumentImageService owns reference photos attached to licences and
// credentials: the feature switch, upload validation, ownership checks and the
// per-document cap.
//
// Every method resolves the subject through the licence/credential repository
// first. That both proves ownership and means an image can never be addressed
// except through the document it belongs to.
type DocumentImageService struct {
	imageRepo      repository.DocumentImageRepository
	licenseRepo    repository.LicenseRepository
	credentialRepo repository.CredentialRepository
	enabled        bool
}

func NewDocumentImageService(
	imageRepo repository.DocumentImageRepository,
	licenseRepo repository.LicenseRepository,
	credentialRepo repository.CredentialRepository,
	enabled bool,
) *DocumentImageService {
	return &DocumentImageService{
		imageRepo:      imageRepo,
		licenseRepo:    licenseRepo,
		credentialRepo: credentialRepo,
		enabled:        enabled,
	}
}

// Enabled reports whether the feature is switched on. Handlers use it to
// answer GET /features so a client can hide the UI instead of discovering the
// 403 by uploading.
func (s *DocumentImageService) Enabled() bool { return s.enabled }

// UploadInput is one candidate image as it arrives from the handler. Data is
// the raw file; ContentType is *derived* from it, never taken from the
// client's declared part header.
type UploadInput struct {
	Data     []byte
	Filename string
	Caption  string
}

// verifySubject proves the caller owns the licence/credential the request
// addresses. Both misses collapse to ErrDocumentSubjectNotFound so a
// non-owner learns nothing beyond "not yours".
func (s *DocumentImageService) verifySubject(ctx context.Context, userID uuid.UUID, subject models.DocumentSubjectType, subjectID uuid.UUID) error {
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
func (s *DocumentImageService) Upload(ctx context.Context, userID uuid.UUID, subject models.DocumentSubjectType, subjectID uuid.UUID, in UploadInput) (*models.DocumentImage, error) {
	if !s.enabled {
		return nil, ErrDocumentImagesDisabled
	}
	if err := s.verifySubject(ctx, userID, subject, subjectID); err != nil {
		return nil, err
	}

	contentType, width, height, err := inspectImage(in.Data)
	if err != nil {
		return nil, err
	}

	img := &models.DocumentImage{
		UserID:      userID,
		ContentType: contentType,
		ByteSize:    len(in.Data),
		Width:       &width,
		Height:      &height,
		Data:        in.Data,
	}
	if name := sanitizeFilename(in.Filename); name != "" {
		img.Filename = &name
	}
	if caption := truncateRunes(strings.TrimSpace(in.Caption), models.MaxDocumentImageCaptionLen); caption != "" {
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

	if err := s.imageRepo.Create(ctx, img, models.MaxDocumentImagesPerSubject); err != nil {
		if errors.Is(err, repository.ErrDocumentImageLimit) {
			return nil, ErrDocumentImageLimitReached
		}
		return nil, err
	}

	// The stored payload is not part of the create response; drop it so a
	// caller cannot accidentally serialize megabytes back out.
	img.Data = nil
	return img, nil
}

// List returns the metadata for a document's images, oldest first.
func (s *DocumentImageService) List(ctx context.Context, userID uuid.UUID, subject models.DocumentSubjectType, subjectID uuid.UUID) ([]*models.DocumentImage, error) {
	if !s.enabled {
		return nil, ErrDocumentImagesDisabled
	}
	if err := s.verifySubject(ctx, userID, subject, subjectID); err != nil {
		return nil, err
	}
	return s.imageRepo.ListBySubject(ctx, userID, subject, subjectID)
}

// Get returns a single image including its bytes, for the authenticated
// download endpoint.
func (s *DocumentImageService) Get(ctx context.Context, userID uuid.UUID, subject models.DocumentSubjectType, subjectID, imageID uuid.UUID) (*models.DocumentImage, error) {
	if !s.enabled {
		return nil, ErrDocumentImagesDisabled
	}
	if err := s.verifySubject(ctx, userID, subject, subjectID); err != nil {
		return nil, err
	}
	img, err := s.imageRepo.GetWithData(ctx, userID, subject, subjectID, imageID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrDocumentImageNotFound
		}
		return nil, err
	}
	return img, nil
}

// Delete removes one image from a document.
func (s *DocumentImageService) Delete(ctx context.Context, userID uuid.UUID, subject models.DocumentSubjectType, subjectID, imageID uuid.UUID) error {
	if !s.enabled {
		return ErrDocumentImagesDisabled
	}
	if err := s.verifySubject(ctx, userID, subject, subjectID); err != nil {
		return err
	}
	if err := s.imageRepo.Delete(ctx, userID, subject, subjectID, imageID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrDocumentImageNotFound
		}
		return err
	}
	return nil
}

// inspectImage decides whether a byte slice is an image we are willing to
// store, and returns its true content type and dimensions.
//
// The declared Content-Type of the multipart part is ignored entirely: it is
// attacker-controlled, and the only thing that matters is what the bytes
// actually are — because those same bytes get served back from our own origin
// later. Sniffing pins the format, and DecodeConfig proves the file really
// parses as that format (a "PNG" that is actually an HTML document with a PNG
// magic prefix fails here) while reading only the header, so a decompression
// bomb is rejected on its declared dimensions rather than by allocating it.
func inspectImage(data []byte) (contentType string, width, height int, err error) {
	if len(data) == 0 {
		return "", 0, 0, ErrDocumentImageEmpty
	}
	if len(data) > models.MaxDocumentImageBytes {
		return "", 0, 0, ErrDocumentImageTooLarge
	}

	sniffed := http.DetectContentType(data)
	if !models.IsAllowedDocumentImageContentType(sniffed) {
		return "", 0, 0, ErrDocumentImageUnsupported
	}

	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", 0, 0, ErrDocumentImageCorrupt
	}
	// The decoder that claimed the file must be the one the sniffed type
	// implies — otherwise the bytes are a polyglot and the two consumers
	// (browser and server) would disagree about what they hold.
	if "image/"+format != sniffed {
		return "", 0, 0, ErrDocumentImageUnsupported
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return "", 0, 0, ErrDocumentImageCorrupt
	}
	if int64(cfg.Width)*int64(cfg.Height) > models.MaxDocumentImagePixels {
		return "", 0, 0, ErrDocumentImageTooManyPixel
	}

	return sniffed, cfg.Width, cfg.Height, nil
}

// sanitizeFilename reduces a client-supplied filename to a display-safe
// basename. It is never used to build a path — it exists so the UI can show
// "licence-front.jpg" — but it still gets stripped of directory components
// and control characters so a stored name cannot smuggle a traversal sequence
// or a terminal escape into whatever renders it later.
func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	// Cut Windows-style separators too: filepath.Base is a no-op on "\" when
	// the server runs on Linux.
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
	return truncateRunes(name, models.MaxDocumentImageFilenameLen)
}

// truncateRunes caps a string at max runes (not bytes), so a cut never lands
// mid-codepoint.
func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
