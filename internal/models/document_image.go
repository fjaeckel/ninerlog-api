package models

import (
	"time"

	"github.com/google/uuid"
)

// DocumentSubjectType names the kind of document an image is attached to.
// A DocumentImage always has exactly one subject.
type DocumentSubjectType string

const (
	DocumentSubjectLicense    DocumentSubjectType = "license"
	DocumentSubjectCredential DocumentSubjectType = "credential"
)

const (
	// MaxDocumentImageBytes caps a single uploaded image. Phone camera JPEGs
	// of a licence booklet land well under this; anything larger is a scan
	// nobody needs at full resolution.
	MaxDocumentImageBytes = 5 * 1024 * 1024

	// MaxDocumentImagesPerSubject caps how many images one licence or
	// credential can carry — front, back, and a couple of ratings pages.
	MaxDocumentImagesPerSubject = 5

	// MaxDocumentImagePixels bounds the decoded pixel count independently of
	// the byte size. A 5 MB PNG can declare a 40000×40000 canvas that costs
	// gigabytes to rasterize; the byte cap alone does not stop a
	// decompression bomb, and DecodeConfig gives us the dimensions without
	// allocating the pixel buffer.
	MaxDocumentImagePixels = 50_000_000

	// MaxDocumentImageFilenameLen / MaxDocumentImageCaptionLen bound the
	// display-only text fields.
	MaxDocumentImageFilenameLen = 255
	MaxDocumentImageCaptionLen  = 200
)

// AllowedDocumentImageContentTypes are the only formats accepted for upload.
//
// Both are decodable by the standard library, so an upload can be re-verified
// server-side rather than trusted from its declared Content-Type. SVG is
// deliberately absent: it is a document format that executes script, and
// serving one back from our own origin would be stored XSS. WebP is absent
// because there is no stdlib decoder, so it could not be verified the same
// way.
var AllowedDocumentImageContentTypes = []string{"image/jpeg", "image/png"}

// DocumentImage is a reference photo or scan attached to a licence or a
// credential. Data carries the raw bytes and is only populated on the
// single-image download path — list and create responses leave it nil so a
// listing never drags megabytes through the service layer.
type DocumentImage struct {
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
func (d *DocumentImage) SubjectType() DocumentSubjectType {
	if d.LicenseID != nil {
		return DocumentSubjectLicense
	}
	return DocumentSubjectCredential
}

// SubjectID returns the id of the licence or credential this image belongs
// to, and uuid.Nil for a malformed row with neither set.
func (d *DocumentImage) SubjectID() uuid.UUID {
	switch {
	case d.LicenseID != nil:
		return *d.LicenseID
	case d.CredentialID != nil:
		return *d.CredentialID
	default:
		return uuid.Nil
	}
}

// IsAllowedDocumentImageContentType reports whether ct is an accepted upload
// format.
func IsAllowedDocumentImageContentType(ct string) bool {
	for _, allowed := range AllowedDocumentImageContentTypes {
		if ct == allowed {
			return true
		}
	}
	return false
}
