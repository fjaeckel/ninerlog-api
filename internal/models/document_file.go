package models

import (
	"time"

	"github.com/google/uuid"
)

// DocumentSubjectType names the kind of document an image is attached to.
// A DocumentFile always has exactly one subject.
type DocumentSubjectType string

const (
	DocumentSubjectLicense    DocumentSubjectType = "license"
	DocumentSubjectCredential DocumentSubjectType = "credential"
)

const (
	// MaxDocumentFileBytes caps a single upload.
	MaxDocumentFileBytes = 5 * 1024 * 1024

	// MaxDocumentFilesPerSubject caps how many files one licence or
	// credential can carry.
	MaxDocumentFilesPerSubject = 5

	// MaxDocumentFilePixels caps the decoded pixel count, independently of
	// the byte size.
	MaxDocumentFilePixels = 50_000_000

	// MaxDocumentFileFilenameLen / MaxDocumentFileCaptionLen bound the
	// display-only text fields.
	MaxDocumentFileFilenameLen = 255
	MaxDocumentFileCaptionLen  = 200
)

// Content types accepted for upload.
const (
	ContentTypeJPEG = "image/jpeg"
	ContentTypePNG  = "image/png"
	ContentTypePDF  = "application/pdf"
)

var AllowedDocumentFileContentTypes = []string{ContentTypeJPEG, ContentTypePNG, ContentTypePDF}

// PDFMagic and PDFTrailer are the structural markers an uploaded PDF must
// carry.
var (
	PDFMagic   = []byte("%PDF-")
	PDFTrailer = []byte("%%EOF")
)

// ContentTypeIsImage reports whether a stored file is a raster image.
func ContentTypeIsImage(ct string) bool {
	return ct == ContentTypeJPEG || ct == ContentTypePNG
}

// ContentTypeIsInlineSafe reports whether a stored file may be served with
// Content-Disposition: inline; anything else is served as an attachment.
func ContentTypeIsInlineSafe(ct string) bool {
	return ContentTypeIsImage(ct)
}

// DocumentFile is a reference photo or scan attached to a licence or a
// credential. Data is populated only on the single-file download path and is
// nil in list and create responses.
type DocumentFile struct {
	ID     uuid.UUID `json:"id"`
	UserID uuid.UUID `json:"userId"`

	// Exactly one of LicenseID / CredentialID is set.
	LicenseID    *uuid.UUID `json:"licenseId,omitempty"`
	CredentialID *uuid.UUID `json:"credentialId,omitempty"`

	ContentType string  `json:"contentType"`
	ByteSize    int     `json:"byteSize"`
	Width       *int    `json:"width,omitempty"`
	Height      *int    `json:"height,omitempty"`
	Filename    *string `json:"filename,omitempty"`
	Caption     *string `json:"caption,omitempty"`

	Data []byte `json:"-"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// SubjectType reports which kind of document this image belongs to.
func (d *DocumentFile) SubjectType() DocumentSubjectType {
	if d.LicenseID != nil {
		return DocumentSubjectLicense
	}
	return DocumentSubjectCredential
}

// SubjectID returns the id of the licence or credential this image belongs
// to, and uuid.Nil for a malformed row with neither set.
func (d *DocumentFile) SubjectID() uuid.UUID {
	switch {
	case d.LicenseID != nil:
		return *d.LicenseID
	case d.CredentialID != nil:
		return *d.CredentialID
	default:
		return uuid.Nil
	}
}

// IsAllowedDocumentFileContentType reports whether ct is an accepted upload
// format.
func IsAllowedDocumentFileContentType(ct string) bool {
	for _, allowed := range AllowedDocumentFileContentTypes {
		if ct == allowed {
			return true
		}
	}
	return false
}
