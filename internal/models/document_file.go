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
	// MaxDocumentFileBytes caps a single upload. Phone camera JPEGs of a
	// licence booklet, and the PDFs an authority emails, both land well under
	// this; anything larger is a scan nobody needs at full resolution.
	MaxDocumentFileBytes = 5 * 1024 * 1024

	// MaxDocumentFilesPerSubject caps how many files one licence or
	// credential can carry — front, back, and a couple of ratings pages.
	MaxDocumentFilesPerSubject = 5

	// MaxDocumentFilePixels bounds the decoded pixel count independently of
	// the byte size. A 5 MB PNG can declare a 40000×40000 canvas that costs
	// gigabytes to rasterize; the byte cap alone does not stop a
	// decompression bomb, and DecodeConfig gives us the dimensions without
	// allocating the pixel buffer.
	MaxDocumentFilePixels = 50_000_000

	// MaxDocumentFileFilenameLen / MaxDocumentFileCaptionLen bound the
	// display-only text fields.
	MaxDocumentFileFilenameLen = 255
	MaxDocumentFileCaptionLen  = 200
)

// Content types accepted for upload.
//
// JPEG and PNG are decodable by the standard library, so an upload can be
// re-verified server-side rather than trusted from its declared Content-Type.
// WebP is absent because there is no stdlib decoder, so it could not be
// verified the same way. SVG is deliberately absent and always will be: it is
// a document format that executes script, and serving one back from our own
// origin would be stored XSS.
//
// PDF is accepted because it is what authorities actually issue — but it
// cannot be verified by decoding, and it is an active format in its own right
// (scripts, embedded files). It earns its place by never being rendered in our
// origin: ContentTypeIsInlineSafe is false for it, so it is served as an
// attachment.
const (
	ContentTypeJPEG = "image/jpeg"
	ContentTypePNG  = "image/png"
	ContentTypePDF  = "application/pdf"
)

var AllowedDocumentFileContentTypes = []string{ContentTypeJPEG, ContentTypePNG, ContentTypePDF}

// PDFMagic and PDFTrailer are the structural markers a PDF must carry. They
// are not a validation of the document — nothing in the standard library
// parses PDF — but together with the size cap they reject the obvious cases:
// a truncated download, a renamed ZIP, a text file with a .pdf extension.
var (
	PDFMagic   = []byte("%PDF-")
	PDFTrailer = []byte("%%EOF")
)

// ContentTypeIsImage reports whether a stored file is a raster image, i.e. one
// the clients can render as a thumbnail and the server can verify by decoding.
func ContentTypeIsImage(ct string) bool {
	return ct == ContentTypeJPEG || ct == ContentTypePNG
}

// ContentTypeIsInlineSafe reports whether a stored file may be served with
// Content-Disposition: inline.
//
// Only the verified raster formats are. A PDF is an active document — it can
// carry JavaScript and embedded attachments — and we cannot parse it to find
// out what is inside, so it is always served as an attachment and never
// rendered inside the application's own origin.
func ContentTypeIsInlineSafe(ct string) bool {
	return ContentTypeIsImage(ct)
}

// DocumentFile is a reference photo or scan attached to a licence or a
// credential. Data carries the raw bytes and is only populated on the
// single-image download path — list and create responses leave it nil so a
// listing never drags megabytes through the service layer.
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
