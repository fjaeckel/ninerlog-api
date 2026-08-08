package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // registers the JPEG decoder used by image.DecodeConfig
	_ "image/png"  // registers the PNG decoder used by image.DecodeConfig
	"net/http"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/pkg/cryptoutil"
	"github.com/google/uuid"
)

var (
	// ErrDocumentFilesDisabled is returned from every image endpoint when the
	// feature is switched off (DOCUMENT_FILES_ENABLED=false). It covers reads
	// as well as writes: serving multi-megabyte blobs is itself the bandwidth
	// half of the abuse surface an operator turns this off to close. Stored
	// rows are left untouched and become reachable again when it is turned
	// back on.
	ErrDocumentFilesDisabled = errors.New("document image uploads are disabled on this server")

	// ErrDocumentFileNotFound covers both "no such image" and "that image
	// belongs to a different document or user" — the caller cannot tell them
	// apart, so an id cannot be probed through someone else's URL.
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

	// ErrDocumentFileUnreadable means the stored bytes did not decrypt: the
	// server is running with a different key than the one that sealed them, or
	// the row was tampered with. Deliberately distinct from "not found" — the
	// file is there and it is the caller's, and telling them it does not exist
	// would send a pilot looking for a scan they never lost.
	ErrDocumentFileUnreadable = errors.New("stored file could not be decrypted")
)

// DocumentFileService owns reference photos attached to licences and
// credentials: the feature switch, upload validation, ownership checks, the
// per-document cap, and encryption of the stored bytes.
//
// Every method resolves the subject through the licence/credential repository
// first. That both proves ownership and means an image can never be addressed
// except through the document it belongs to.
//
// Encryption sits here rather than in the repository so that everything above
// this layer — handlers, tests, background jobs — deals in the actual file.
// The repository stores whatever bytes it is handed and knows nothing about
// what they mean, which is the same division the rest of the codebase keeps.
type DocumentFileService struct {
	imageRepo      repository.DocumentFileRepository
	licenseRepo    repository.LicenseRepository
	credentialRepo repository.CredentialRepository
	enabled        bool
	aead           *cryptoutil.AEAD
}

// NewDocumentFileService wires the subsystem. aead is the purpose-derived key
// for document files; passing nil leaves the feature off no matter what
// enabled says, because storing a pilot's identity documents in the clear is
// not a mode this service offers.
func NewDocumentFileService(
	imageRepo repository.DocumentFileRepository,
	licenseRepo repository.LicenseRepository,
	credentialRepo repository.CredentialRepository,
	enabled bool,
	aead *cryptoutil.AEAD,
) *DocumentFileService {
	return &DocumentFileService{
		imageRepo:      imageRepo,
		licenseRepo:    licenseRepo,
		credentialRepo: credentialRepo,
		enabled:        enabled,
		aead:           aead,
	}
}

// Enabled reports whether the feature is switched on. Handlers use it to
// answer GET /features so a client can hide the UI instead of discovering the
// 403 by uploading.
//
// A missing key counts as switched off. main refuses to start when the feature
// is explicitly enabled without one, so this is the belt to that braces: no
// arrangement of configuration reaches the storage path without a key.
func (s *DocumentFileService) Enabled() bool { return s.enabled && s.aead != nil }

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
	if !s.Enabled() {
		return nil, ErrDocumentFilesDisabled
	}
	if err := s.verifySubject(ctx, userID, subject, subjectID); err != nil {
		return nil, err
	}

	contentType, width, height, err := inspectFile(in.Data)
	if err != nil {
		return nil, err
	}

	// Width and height are nil for formats without intrinsic pixel dimensions
	// (PDF), and are passed through as such rather than stored as zeroes.
	//
	// ByteSize is the size of the file the pilot uploaded, not of what lands in
	// the column: the client shows it, the cap is expressed in it, and the
	// ciphertext's extra authentication tag is an implementation detail of
	// storage that nothing above should have to subtract.
	//
	// The id is minted here rather than by the database, because the bytes are
	// sealed against it and so it has to exist before they are.
	img := &models.DocumentFile{
		ID:          uuid.New(),
		UserID:      userID,
		ContentType: contentType,
		ByteSize:    len(in.Data),
		Width:       width,
		Height:      height,
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

	ciphertext, nonce, err := s.aead.EncryptWithAAD(in.Data, documentFileAAD(img))
	if err != nil {
		return nil, fmt.Errorf("encrypt document file: %w", err)
	}
	img.Data = ciphertext
	img.DataNonce = nonce

	if err := s.imageRepo.Create(ctx, img, models.MaxDocumentFilesPerSubject); err != nil {
		if errors.Is(err, repository.ErrDocumentFileLimit) {
			return nil, ErrDocumentFileLimitReached
		}
		return nil, err
	}

	// The stored payload is not part of the create response; drop it so a
	// caller cannot accidentally serialize megabytes back out.
	img.Data = nil
	img.DataNonce = nil
	return img, nil
}

// List returns the metadata for a document's images, oldest first.
func (s *DocumentFileService) List(ctx context.Context, userID uuid.UUID, subject models.DocumentSubjectType, subjectID uuid.UUID) ([]*models.DocumentFile, error) {
	if !s.Enabled() {
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
	if !s.Enabled() {
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
	if err := s.open(img); err != nil {
		return nil, err
	}
	return img, nil
}

// open replaces a row's stored bytes with the file itself, in place.
//
// Every stored file is encrypted — the column holding the nonce is NOT NULL, so
// a row without one cannot exist — which means there is no "maybe it is
// plaintext" branch to get wrong. A missing nonce is a corrupt row, not an old
// one, and is treated as unreadable rather than served as if it were the file.
func (s *DocumentFileService) open(img *models.DocumentFile) error {
	if s.aead == nil || len(img.DataNonce) == 0 {
		return ErrDocumentFileUnreadable
	}
	plaintext, err := s.aead.DecryptWithAAD(img.Data, img.DataNonce, documentFileAAD(img))
	if err != nil {
		return ErrDocumentFileUnreadable
	}
	img.Data = plaintext
	img.DataNonce = nil
	return nil
}

// documentFileAAD is the context a file's ciphertext is bound to: its own id,
// its owner, and the content type the server will hand back to a browser.
//
// None of this is stored in the ciphertext — the decrypting side rebuilds it
// from the row — so it costs nothing and makes the stored blob non-portable.
// Moving a blob to another row, another user, or relabelling a PDF as a JPEG
// all break authentication and surface as ErrDocumentFileUnreadable instead of
// as a file served under someone else's name.
//
// The two UUIDs are fixed-width in their string form, so concatenating with a
// separator cannot be made ambiguous by a crafted content type.
func documentFileAAD(img *models.DocumentFile) []byte {
	return []byte(strings.Join([]string{
		"ninerlog/document-file/v1",
		img.ID.String(),
		img.UserID.String(),
		img.ContentType,
	}, "|"))
}

// Delete removes one image from a document.
func (s *DocumentFileService) Delete(ctx context.Context, userID uuid.UUID, subject models.DocumentSubjectType, subjectID, imageID uuid.UUID) error {
	if !s.Enabled() {
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

// inspectFile decides whether a byte slice is something we are willing to
// store, and returns its true content type and (for images) its dimensions.
//
// The declared Content-Type of the multipart part is ignored entirely: it is
// attacker-controlled, and the only thing that matters is what the bytes
// actually are — because those same bytes get served back to a browser later.
// Sniffing pins the format; what happens next depends on which format it is,
// because the two families offer very different guarantees.
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
		// A PDF has no single intrinsic pixel size — pages carry their own
		// dimensions in points — so width/height stay null rather than being
		// invented.
		return sniffed, nil, nil, nil
	default:
		return "", nil, nil, ErrDocumentFileUnsupported
	}
}

// inspectImage verifies a raster image and returns its declared dimensions.
//
// DecodeConfig requires the header to parse as the sniffed format, and the
// header's declared dimensions are capped, so a decompression bomb is refused
// without ever allocating its pixels.
//
// This validates the HEADER, not the whole file. DecodeConfig stops at the
// PNG IHDR / JPEG SOF, so a valid header followed by arbitrary bytes — or by
// nothing at all — is accepted and stored verbatim. That is a deliberate
// trade: a full image.Decode is the only thing that proves every byte, and it
// must allocate the entire pixel buffer, which is exactly the cost the
// dimension cap exists to avoid.
//
// Serving is what makes that acceptable. The response Content-Type is the
// sniffed value, never the uploader's, X-Content-Type-Options: nosniff is set
// on every response, and the download requires a bearer token — so a browser
// can neither navigate to these bytes nor reinterpret them as anything but an
// image. The residual is storage: a caller can park arbitrary bytes behind a
// valid header, bounded by the 5 MB × 5-per-document caps and attributable to
// their account. See TestDocumentFileUpload_ValidatesTheHeaderNotTheWholeFile.
func inspectImage(data []byte, sniffed string) (width, height int, err error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, ErrDocumentFileCorrupt
	}
	// The decoder that claimed the file must be the one the sniffed type
	// implies — otherwise the bytes are a polyglot and the two consumers
	// (browser and server) would disagree about what they hold.
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

// inspectPDF applies the only structural check a PDF affords us without a
// parser: the %PDF- signature at the front and a %%EOF trailer at the end.
//
// This is deliberately weaker than the image path and it is worth being blunt
// about why. Nothing in the standard library parses PDF, and pulling in a
// third-party parser to inspect untrusted input would add attack surface
// rather than remove it. So this rejects the honest mistakes — a truncated
// download, a renamed archive, a text file with the wrong extension — and
// nothing more. A structurally valid PDF carrying JavaScript or an embedded
// payload is stored.
//
// What contains that is the serving path, not this function: PDFs go out with
// Content-Disposition: attachment (see models.ContentTypeIsInlineSafe), behind
// nosniff and a bearer token, so nothing renders them inside our origin.
//
// The trailer is looked for in the last few KB rather than at the very end,
// because a linearized or incrementally-updated PDF legitimately carries bytes
// after its final %%EOF.
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
// %%EOF. Generous enough for the trailing bytes a linearized or incrementally
// updated PDF appends, small enough that the scan is trivial.
const pdfTrailerSearchWindow = 4096

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
	return truncateRunes(name, models.MaxDocumentFileFilenameLen)
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
