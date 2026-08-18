package service_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/google/uuid"
)

// ── Test doubles ──────────────────────────────────────────────────────────

type mockDocumentFileRepo struct {
	images    map[uuid.UUID]*models.DocumentFile
	createErr error
}

func newMockDocumentFileRepo() *mockDocumentFileRepo {
	return &mockDocumentFileRepo{images: make(map[uuid.UUID]*models.DocumentFile)}
}

func (m *mockDocumentFileRepo) matches(img *models.DocumentFile, userID uuid.UUID, subject models.DocumentSubjectType, subjectID uuid.UUID) bool {
	return img.UserID == userID && img.SubjectType() == subject && img.SubjectID() == subjectID
}

func (m *mockDocumentFileRepo) Create(ctx context.Context, img *models.DocumentFile, maxPerSubject int) error {
	if m.createErr != nil {
		return m.createErr
	}
	count, _ := m.CountBySubject(ctx, img.UserID, img.SubjectType(), img.SubjectID())
	if count >= maxPerSubject {
		return repository.ErrDocumentFileLimit
	}
	img.ID = uuid.New()
	img.CreatedAt = time.Now()
	img.UpdatedAt = time.Now()
	stored := *img
	m.images[img.ID] = &stored
	return nil
}

func (m *mockDocumentFileRepo) ListBySubject(ctx context.Context, userID uuid.UUID, subject models.DocumentSubjectType, subjectID uuid.UUID) ([]*models.DocumentFile, error) {
	out := make([]*models.DocumentFile, 0)
	for _, img := range m.images {
		if m.matches(img, userID, subject, subjectID) {
			copied := *img
			copied.Data = nil
			out = append(out, &copied)
		}
	}
	return out, nil
}

func (m *mockDocumentFileRepo) GetWithData(ctx context.Context, userID uuid.UUID, subject models.DocumentSubjectType, subjectID, imageID uuid.UUID) (*models.DocumentFile, error) {
	img, ok := m.images[imageID]
	if !ok || !m.matches(img, userID, subject, subjectID) {
		return nil, repository.ErrNotFound
	}
	return img, nil
}

func (m *mockDocumentFileRepo) Delete(ctx context.Context, userID uuid.UUID, subject models.DocumentSubjectType, subjectID, imageID uuid.UUID) error {
	img, ok := m.images[imageID]
	if !ok || !m.matches(img, userID, subject, subjectID) {
		return repository.ErrNotFound
	}
	delete(m.images, imageID)
	return nil
}

func (m *mockDocumentFileRepo) CountBySubject(ctx context.Context, userID uuid.UUID, subject models.DocumentSubjectType, subjectID uuid.UUID) (int, error) {
	count := 0
	for _, img := range m.images {
		if m.matches(img, userID, subject, subjectID) {
			count++
		}
	}
	return count, nil
}

// The licence mock in license_test.go is not visible from this external test
// package; these tests carry their own.
type docMockLicenseRepo struct {
	licenses map[uuid.UUID]*models.License
}

func (m *docMockLicenseRepo) Create(ctx context.Context, l *models.License) error {
	l.ID = uuid.New()
	l.CreatedAt = time.Now()
	l.UpdatedAt = time.Now()
	m.licenses[l.ID] = l
	return nil
}

func (m *docMockLicenseRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.License, error) {
	l, ok := m.licenses[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return l, nil
}

func (m *docMockLicenseRepo) GetByUserID(ctx context.Context, userID uuid.UUID, updatedSince *time.Time) ([]*models.License, error) {
	out := make([]*models.License, 0)
	for _, l := range m.licenses {
		if l.UserID == userID {
			out = append(out, l)
		}
	}
	return out, nil
}

func (m *docMockLicenseRepo) Update(ctx context.Context, l *models.License) error {
	if _, ok := m.licenses[l.ID]; !ok {
		return repository.ErrNotFound
	}
	m.licenses[l.ID] = l
	return nil
}

func (m *docMockLicenseRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if _, ok := m.licenses[id]; !ok {
		return repository.ErrNotFound
	}
	delete(m.licenses, id)
	return nil
}

// ── Fixtures ──────────────────────────────────────────────────────────────

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

// pdfBytes builds a minimal but structurally real PDF: %PDF- signature,
// a couple of objects, and the %%EOF trailer the validator looks for.
func pdfBytes() []byte {
	return []byte("%PDF-1.4\n" +
		"1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n" +
		"2 0 obj<</Type/Pages/Kids[]/Count 0>>endobj\n" +
		"trailer<</Root 1 0 R>>\n" +
		"%%EOF\n")
}

type docImageFixture struct {
	svc        *service.DocumentFileService
	imageRepo  *mockDocumentFileRepo
	userID     uuid.UUID
	otherUser  uuid.UUID
	licenseID  uuid.UUID
	credential uuid.UUID
}

func newDocImageFixture(t *testing.T, enabled bool) *docImageFixture {
	t.Helper()
	licenseRepo := &docMockLicenseRepo{licenses: make(map[uuid.UUID]*models.License)}
	credentialRepo := newMockCredentialRepo()
	imageRepo := newMockDocumentFileRepo()

	userID := uuid.New()
	license := &models.License{
		UserID: userID, RegulatoryAuthority: "EASA", LicenseType: "PPL",
		LicenseNumber: "D-1234", IssueDate: time.Now(), IssuingAuthority: "LBA",
	}
	if err := licenseRepo.Create(context.Background(), license); err != nil {
		t.Fatalf("seed license: %v", err)
	}
	credential := &models.Credential{
		UserID: userID, CredentialType: models.CredentialTypeRadioAZF,
		IssueDate: time.Now(), IssuingAuthority: "BNetzA",
	}
	if err := credentialRepo.Create(context.Background(), credential); err != nil {
		t.Fatalf("seed credential: %v", err)
	}

	return &docImageFixture{
		svc:        service.NewDocumentFileService(imageRepo, licenseRepo, credentialRepo, enabled),
		imageRepo:  imageRepo,
		userID:     userID,
		otherUser:  uuid.New(),
		licenseID:  license.ID,
		credential: credential.ID,
	}
}

func (f *docImageFixture) upload(t *testing.T, data []byte) (*models.DocumentFile, error) {
	t.Helper()
	return f.svc.Upload(context.Background(), f.userID, models.DocumentSubjectLicense, f.licenseID,
		service.UploadInput{Data: data, Filename: "front.png"})
}

// ── Tests ─────────────────────────────────────────────────────────────────

func TestDocumentFileUpload_StoresMetadataFromTheBytes(t *testing.T) {
	f := newDocImageFixture(t, true)

	data := pngBytes(t, 120, 80)
	img, err := f.upload(t, data)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	if img.ContentType != "image/png" {
		t.Errorf("contentType = %q, want image/png", img.ContentType)
	}
	if img.ByteSize != len(data) {
		t.Errorf("byteSize = %d, want %d", img.ByteSize, len(data))
	}
	if img.Width == nil || *img.Width != 120 || img.Height == nil || *img.Height != 80 {
		t.Errorf("dimensions = %v×%v, want 120×80", img.Width, img.Height)
	}
	if img.LicenseID == nil || *img.LicenseID != f.licenseID {
		t.Errorf("licenseId = %v, want %v", img.LicenseID, f.licenseID)
	}
	if img.CredentialID != nil {
		t.Errorf("credentialId should be nil on a licence image, got %v", img.CredentialID)
	}
	// The create response must not carry the payload back out.
	if img.Data != nil {
		t.Errorf("create response leaked %d bytes of payload", len(img.Data))
	}
}

func TestDocumentFileUpload_AcceptsJPEG(t *testing.T) {
	f := newDocImageFixture(t, true)
	img, err := f.upload(t, jpegBytes(t, 32, 32))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if img.ContentType != "image/jpeg" {
		t.Errorf("contentType = %q, want image/jpeg", img.ContentType)
	}
}

// A script/document masquerading as an image must not be storable; the checks
// run on the bytes, never the declared type.
func TestDocumentFileUpload_RejectsNonImageContent(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want error
	}{
		{"empty", nil, service.ErrDocumentFileEmpty},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), service.ErrDocumentFileUnsupported},
		{"html", []byte("<!doctype html><html><body>hi</body></html>"), service.ErrDocumentFileUnsupported},
		{"gif", append([]byte("GIF89a"), bytes.Repeat([]byte{0}, 32)...), service.ErrDocumentFileUnsupported},
		{"png magic with garbage body", append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0x41}, 64)...), service.ErrDocumentFileCorrupt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newDocImageFixture(t, true)
			_, err := f.upload(t, tt.data)
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
			if len(f.imageRepo.images) != 0 {
				t.Errorf("rejected upload was still stored")
			}
		})
	}
}

// Characterization test: validation stops at the header — a valid header
// followed by arbitrary bytes IS accepted.
//
// If validation is ever tightened to full decode or a structural walk, this
// test SHOULD fail — update it together with the claims in
// api-spec/openapi.yaml, docs/API.md and docs/FEATURES.md.
func TestDocumentFileUpload_ValidatesTheHeaderNotTheWholeFile(t *testing.T) {
	html := []byte("<html><script>alert(1)</script></html>")
	valid := pngBytes(t, 8, 8)

	tests := []struct {
		name string
		data []byte
	}{
		{"trailing payload after a complete PNG", append(append([]byte{}, valid...), html...)},
		{"PNG header with a payload where the pixel data belongs", append(append([]byte{}, valid[:33]...), html...)},
		{"PNG header and nothing else", valid[:33]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newDocImageFixture(t, true)
			img, err := f.upload(t, tt.data)
			if err != nil {
				t.Fatalf("expected the header-only check to accept this, got %v", err)
			}
			// Whatever rode along, the stored type is the sniffed one — which is
			// what the download responds with, under nosniff.
			if img.ContentType != "image/png" {
				t.Errorf("contentType = %q, want image/png", img.ContentType)
			}
			if img.ByteSize != len(tt.data) {
				t.Errorf("byteSize = %d, want %d — the payload is stored verbatim", img.ByteSize, len(tt.data))
			}
		})
	}
}

// PDFs are accepted: structural markers only, no dimensions, and never served
// inline.
func TestDocumentFileUpload_AcceptsPDF(t *testing.T) {
	f := newDocImageFixture(t, true)

	file, err := f.upload(t, pdfBytes())
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if file.ContentType != models.ContentTypePDF {
		t.Errorf("contentType = %q, want %q", file.ContentType, models.ContentTypePDF)
	}
	// No intrinsic pixel size: dimensions stay null.
	if file.Width != nil || file.Height != nil {
		t.Errorf("dimensions = %v×%v, want null×null for a PDF", file.Width, file.Height)
	}
	if models.ContentTypeIsInlineSafe(file.ContentType) {
		t.Error("a PDF must never be classified as safe to serve inline")
	}
}

func TestDocumentFileUpload_RejectsBrokenPDFs(t *testing.T) {
	valid := pdfBytes()

	tests := []struct {
		name string
		data []byte
		want error
	}{
		// A renamed archive or text file: no %PDF- signature, so it does not
		// even sniff as a PDF.
		{"no signature", []byte("PK\x03\x04 this is really a zip"), service.ErrDocumentFileUnsupported},
		// Truncated download: signature present, trailer gone.
		{"no trailer", valid[:len(valid)-8], service.ErrDocumentFileCorrupt},
		{"signature only", []byte("%PDF-1.7\n"), service.ErrDocumentFileCorrupt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newDocImageFixture(t, true)
			if _, err := f.upload(t, tt.data); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
			if len(f.imageRepo.images) != 0 {
				t.Error("rejected upload was still stored")
			}
		})
	}
}

// The trailer is looked for in a window at the end, not at the exact end.
func TestDocumentFileUpload_AcceptsTrailerBeforeTrailingBytes(t *testing.T) {
	f := newDocImageFixture(t, true)
	data := append(pdfBytes(), bytes.Repeat([]byte(" "), 512)...)

	if _, err := f.upload(t, data); err != nil {
		t.Errorf("a PDF with bytes after %%%%EOF should be accepted: %v", err)
	}
}

func TestDocumentFileUpload_PDFStillBoundBySizeAndCount(t *testing.T) {
	f := newDocImageFixture(t, true)

	oversized := make([]byte, models.MaxDocumentFileBytes+1)
	copy(oversized, pdfBytes())
	if _, err := f.upload(t, oversized); !errors.Is(err, service.ErrDocumentFileTooLarge) {
		t.Errorf("err = %v, want ErrDocumentFileTooLarge", err)
	}

	for i := 0; i < models.MaxDocumentFilesPerSubject; i++ {
		if _, err := f.upload(t, pdfBytes()); err != nil {
			t.Fatalf("upload %d: %v", i, err)
		}
	}
	if _, err := f.upload(t, pdfBytes()); !errors.Is(err, service.ErrDocumentFileLimitReached) {
		t.Errorf("err = %v, want ErrDocumentFileLimitReached", err)
	}
}

func TestDocumentFileUpload_RejectsOversizedFile(t *testing.T) {
	f := newDocImageFixture(t, true)
	oversized := make([]byte, models.MaxDocumentFileBytes+1)
	copy(oversized, pngBytes(t, 4, 4))

	if _, err := f.upload(t, oversized); !errors.Is(err, service.ErrDocumentFileTooLarge) {
		t.Errorf("err = %v, want ErrDocumentFileTooLarge", err)
	}
}

// A small PNG can declare an enormous canvas. The byte cap does not catch it;
// the pixel cap must, and without allocating the pixels to find out.
func TestDocumentFileUpload_RejectsDecompressionBomb(t *testing.T) {
	f := newDocImageFixture(t, true)

	// A valid PNG header claiming 40000×40000 (1.6e9 px) in ~70 bytes.
	bomb := pngBytes(t, 1, 1)
	header := bomb[:33] // signature (8) + IHDR chunk (25)
	bombed := append([]byte{}, header...)
	// width and height live at offsets 16..23 of the file.
	for i, b := range []byte{0, 0, 0x9c, 0x40, 0, 0, 0x9c, 0x40} {
		bombed[16+i] = b
	}
	bombed = append(bombed, bomb[33:]...)

	_, err := f.svc.Upload(context.Background(), f.userID, models.DocumentSubjectLicense, f.licenseID,
		service.UploadInput{Data: bombed})
	// Either a pixel-cap or a corruption rejection is acceptable.
	if !errors.Is(err, service.ErrDocumentFileTooManyPixel) && !errors.Is(err, service.ErrDocumentFileCorrupt) {
		t.Errorf("err = %v, want a pixel-cap or corruption rejection", err)
	}
}

func TestDocumentFileUpload_EnforcesPerDocumentLimit(t *testing.T) {
	f := newDocImageFixture(t, true)

	for i := 0; i < models.MaxDocumentFilesPerSubject; i++ {
		if _, err := f.upload(t, pngBytes(t, 8, 8)); err != nil {
			t.Fatalf("upload %d: %v", i, err)
		}
	}
	if _, err := f.upload(t, pngBytes(t, 8, 8)); !errors.Is(err, service.ErrDocumentFileLimitReached) {
		t.Errorf("err = %v, want ErrDocumentFileLimitReached", err)
	}

	// The cap is per document, not per user: the credential still has room.
	if _, err := f.svc.Upload(context.Background(), f.userID, models.DocumentSubjectCredential, f.credential,
		service.UploadInput{Data: pngBytes(t, 8, 8)}); err != nil {
		t.Errorf("credential upload should be unaffected by the licence's cap: %v", err)
	}
}

func TestDocumentFileUpload_SanitizesFilenameAndCaption(t *testing.T) {
	f := newDocImageFixture(t, true)

	img, err := f.svc.Upload(context.Background(), f.userID, models.DocumentSubjectLicense, f.licenseID,
		service.UploadInput{
			Data:     pngBytes(t, 8, 8),
			Filename: `../../etc/passwd`,
			Caption:  "  " + strings.Repeat("x", models.MaxDocumentFileCaptionLen+50) + "  ",
		})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if img.Filename == nil || *img.Filename != "passwd" {
		t.Errorf("filename = %v, want %q", img.Filename, "passwd")
	}
	if img.Caption == nil || len([]rune(*img.Caption)) != models.MaxDocumentFileCaptionLen {
		t.Errorf("caption was not truncated to %d runes", models.MaxDocumentFileCaptionLen)
	}
}

func TestDocumentFile_OwnershipIsEnforced(t *testing.T) {
	f := newDocImageFixture(t, true)
	img, err := f.upload(t, pngBytes(t, 8, 8))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	ctx := context.Background()

	// Another user must not be able to reach the licence at all — and must not
	// be able to tell "not mine" from "does not exist".
	if _, err := f.svc.List(ctx, f.otherUser, models.DocumentSubjectLicense, f.licenseID); !errors.Is(err, service.ErrDocumentSubjectNotFound) {
		t.Errorf("list as other user: err = %v, want ErrDocumentSubjectNotFound", err)
	}
	if _, err := f.svc.Get(ctx, f.otherUser, models.DocumentSubjectLicense, f.licenseID, img.ID); !errors.Is(err, service.ErrDocumentSubjectNotFound) {
		t.Errorf("get as other user: err = %v, want ErrDocumentSubjectNotFound", err)
	}
	if err := f.svc.Delete(ctx, f.otherUser, models.DocumentSubjectLicense, f.licenseID, img.ID); !errors.Is(err, service.ErrDocumentSubjectNotFound) {
		t.Errorf("delete as other user: err = %v, want ErrDocumentSubjectNotFound", err)
	}
	if _, err := f.svc.Upload(ctx, f.otherUser, models.DocumentSubjectLicense, f.licenseID,
		service.UploadInput{Data: pngBytes(t, 8, 8)}); !errors.Is(err, service.ErrDocumentSubjectNotFound) {
		t.Errorf("upload as other user: err = %v, want ErrDocumentSubjectNotFound", err)
	}
}

// An image id is only valid under the document it belongs to — presenting it
// under a different (also owned) document must not resolve.
func TestDocumentFile_NotReachableThroughTheWrongParent(t *testing.T) {
	f := newDocImageFixture(t, true)
	img, err := f.upload(t, pngBytes(t, 8, 8))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	_, err = f.svc.Get(context.Background(), f.userID, models.DocumentSubjectCredential, f.credential, img.ID)
	if !errors.Is(err, service.ErrDocumentFileNotFound) {
		t.Errorf("err = %v, want ErrDocumentFileNotFound", err)
	}
}

func TestDocumentFile_GetReturnsPayload(t *testing.T) {
	f := newDocImageFixture(t, true)
	data := pngBytes(t, 16, 16)
	img, err := f.upload(t, data)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	fetched, err := f.svc.Get(context.Background(), f.userID, models.DocumentSubjectLicense, f.licenseID, img.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !bytes.Equal(fetched.Data, data) {
		t.Error("downloaded bytes differ from what was uploaded")
	}
}

func TestDocumentFile_ListOmitsPayload(t *testing.T) {
	f := newDocImageFixture(t, true)
	if _, err := f.upload(t, pngBytes(t, 16, 16)); err != nil {
		t.Fatalf("upload: %v", err)
	}
	images, err := f.svc.List(context.Background(), f.userID, models.DocumentSubjectLicense, f.licenseID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("len(images) = %d, want 1", len(images))
	}
	if images[0].Data != nil {
		t.Error("listing carried the image payload")
	}
}

func TestDocumentFile_Delete(t *testing.T) {
	f := newDocImageFixture(t, true)
	img, err := f.upload(t, pngBytes(t, 8, 8))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	ctx := context.Background()
	if err := f.svc.Delete(ctx, f.userID, models.DocumentSubjectLicense, f.licenseID, img.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := f.svc.Delete(ctx, f.userID, models.DocumentSubjectLicense, f.licenseID, img.ID); !errors.Is(err, service.ErrDocumentFileNotFound) {
		t.Errorf("second delete: err = %v, want ErrDocumentFileNotFound", err)
	}
}

// The switch blocks the whole feature, reads included.
func TestDocumentFile_DisabledBlocksEveryOperation(t *testing.T) {
	f := newDocImageFixture(t, false)
	ctx := context.Background()

	if f.svc.Enabled() {
		t.Error("Enabled() should report false")
	}

	_, err := f.svc.Upload(ctx, f.userID, models.DocumentSubjectLicense, f.licenseID, service.UploadInput{Data: pngBytes(t, 8, 8)})
	if !errors.Is(err, service.ErrDocumentFilesDisabled) {
		t.Errorf("upload: err = %v, want ErrDocumentFilesDisabled", err)
	}
	if _, err := f.svc.List(ctx, f.userID, models.DocumentSubjectLicense, f.licenseID); !errors.Is(err, service.ErrDocumentFilesDisabled) {
		t.Errorf("list: err = %v, want ErrDocumentFilesDisabled", err)
	}
	if _, err := f.svc.Get(ctx, f.userID, models.DocumentSubjectLicense, f.licenseID, uuid.New()); !errors.Is(err, service.ErrDocumentFilesDisabled) {
		t.Errorf("get: err = %v, want ErrDocumentFilesDisabled", err)
	}
	if err := f.svc.Delete(ctx, f.userID, models.DocumentSubjectLicense, f.licenseID, uuid.New()); !errors.Is(err, service.ErrDocumentFilesDisabled) {
		t.Errorf("delete: err = %v, want ErrDocumentFilesDisabled", err)
	}
}

func TestDocumentFile_SubjectMustExist(t *testing.T) {
	f := newDocImageFixture(t, true)
	_, err := f.svc.Upload(context.Background(), f.userID, models.DocumentSubjectLicense, uuid.New(),
		service.UploadInput{Data: pngBytes(t, 8, 8)})
	if !errors.Is(err, service.ErrDocumentSubjectNotFound) {
		t.Errorf("err = %v, want ErrDocumentSubjectNotFound", err)
	}
}
