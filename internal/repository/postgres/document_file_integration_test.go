//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/repository/postgres"
	"github.com/fjaeckel/ninerlog-api/internal/testutil"
	"github.com/google/uuid"
)

func TestDocumentFileRepositoryIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	ctx := context.Background()
	userRepo := postgres.NewUserRepository(db)
	licenseRepo := postgres.NewLicenseRepository(db)
	credentialRepo := postgres.NewCredentialRepository(db)
	imageRepo := postgres.NewDocumentFileRepository(db)

	user := testutil.CreateTestUser("docimage-test@example.com", "Doc Image User", "hashedpass")
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	license := &models.License{
		UserID: user.ID, RegulatoryAuthority: "EASA", LicenseType: "PPL",
		LicenseNumber: "PPL-IMG-1", IssueDate: time.Now(), IssuingAuthority: "LBA",
	}
	if err := licenseRepo.Create(ctx, license); err != nil {
		t.Fatalf("create license: %v", err)
	}
	credential := &models.Credential{
		UserID: user.ID, CredentialType: models.CredentialTypeRadioAZF,
		IssueDate: time.Now(), IssuingAuthority: "Bundesnetzagentur",
	}
	if err := credentialRepo.Create(ctx, credential); err != nil {
		t.Fatalf("create credential: %v", err)
	}

	// Every row carries a nonce — the column is NOT NULL — so these fixtures
	// supply one. The repository stores whatever bytes it is handed and does
	// not care whether they are really ciphertext; that is the service's job.
	nonce := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}

	newImage := func(data []byte) *models.DocumentFile {
		licenseID := license.ID
		w, h := 4, 4
		name := "front.png"
		return &models.DocumentFile{
			UserID:      user.ID,
			LicenseID:   &licenseID,
			ContentType: "image/png",
			ByteSize:    len(data),
			Width:       &w,
			Height:      &h,
			Filename:    &name,
			Data:        data,
			DataNonce:   nonce,
		}
	}

	payload := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 1, 2, 3, 4}

	t.Run("create and read back the payload", func(t *testing.T) {
		img := newImage(payload)
		if err := imageRepo.Create(ctx, img, models.MaxDocumentFilesPerSubject); err != nil {
			t.Fatalf("create: %v", err)
		}
		if img.ID == uuid.Nil {
			t.Fatal("create did not populate the id")
		}

		fetched, err := imageRepo.GetWithData(ctx, user.ID, models.DocumentSubjectLicense, license.ID, img.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if !bytes.Equal(fetched.Data, payload) {
			t.Error("stored bytes differ from what was written")
		}
		if fetched.LicenseID == nil || *fetched.LicenseID != license.ID {
			t.Errorf("licenseId = %v, want %v", fetched.LicenseID, license.ID)
		}
		if fetched.CredentialID != nil {
			t.Errorf("credentialId = %v, want nil", fetched.CredentialID)
		}
	})

	t.Run("listing omits the payload", func(t *testing.T) {
		images, err := imageRepo.ListBySubject(ctx, user.ID, models.DocumentSubjectLicense, license.ID)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(images) == 0 {
			t.Fatal("expected at least one image")
		}
		for _, img := range images {
			if img.Data != nil {
				t.Error("listing pulled the payload out of storage")
			}
			if img.ByteSize != len(payload) {
				t.Errorf("byteSize = %d, want %d", img.ByteSize, len(payload))
			}
		}
	})

	t.Run("a credential's images are a separate collection", func(t *testing.T) {
		credentialID := credential.ID
		img := newImage(payload)
		img.LicenseID = nil
		img.CredentialID = &credentialID
		if err := imageRepo.Create(ctx, img, models.MaxDocumentFilesPerSubject); err != nil {
			t.Fatalf("create: %v", err)
		}

		// The same id must not resolve through the licence it does not belong to.
		if _, err := imageRepo.GetWithData(ctx, user.ID, models.DocumentSubjectLicense, license.ID, img.ID); !errors.Is(err, repository.ErrNotFound) {
			t.Errorf("cross-subject get: err = %v, want ErrNotFound", err)
		}

		count, err := imageRepo.CountBySubject(ctx, user.ID, models.DocumentSubjectCredential, credential.ID)
		if err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 1 {
			t.Errorf("credential image count = %d, want 1", count)
		}
	})

	t.Run("another user cannot reach the rows", func(t *testing.T) {
		other := testutil.CreateTestUser("docimage-other@example.com", "Other", "hashedpass")
		if err := userRepo.Create(ctx, other); err != nil {
			t.Fatalf("create other user: %v", err)
		}
		images, err := imageRepo.ListBySubject(ctx, other.ID, models.DocumentSubjectLicense, license.ID)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(images) != 0 {
			t.Errorf("other user saw %d images", len(images))
		}
	})

	// The cap is enforced inside the INSERT, so concurrent uploads racing for
	// the last slot must not both succeed. A count-then-insert would.
	t.Run("concurrent uploads cannot exceed the cap", func(t *testing.T) {
		raceLicense := &models.License{
			UserID: user.ID, RegulatoryAuthority: "EASA", LicenseType: "CPL",
			LicenseNumber: "CPL-RACE-1", IssueDate: time.Now(), IssuingAuthority: "LBA",
		}
		if err := licenseRepo.Create(ctx, raceLicense); err != nil {
			t.Fatalf("create license: %v", err)
		}

		// A start barrier, so every goroutine issues its INSERT in the same
		// instant rather than trickling in. Without it this test passes against
		// a count-then-insert that has no lock, purely on timing — which is
		// exactly how the unlocked version survived its first run.
		const attempts = 12
		var wg sync.WaitGroup
		start := make(chan struct{})
		results := make([]error, attempts)
		for i := 0; i < attempts; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				licenseID := raceLicense.ID
				img := newImage(payload)
				img.LicenseID = &licenseID
				<-start
				results[i] = imageRepo.Create(ctx, img, models.MaxDocumentFilesPerSubject)
			}(i)
		}
		close(start)
		wg.Wait()

		succeeded := 0
		for i, err := range results {
			switch {
			case err == nil:
				succeeded++
			case errors.Is(err, repository.ErrDocumentFileLimit):
			default:
				t.Errorf("attempt %d: unexpected error %v", i, err)
			}
		}
		if succeeded > models.MaxDocumentFilesPerSubject {
			t.Errorf("%d uploads succeeded, cap is %d", succeeded, models.MaxDocumentFilesPerSubject)
		}

		count, err := imageRepo.CountBySubject(ctx, user.ID, models.DocumentSubjectLicense, raceLicense.ID)
		if err != nil {
			t.Fatalf("count: %v", err)
		}
		if count > models.MaxDocumentFilesPerSubject {
			t.Errorf("stored %d images, cap is %d", count, models.MaxDocumentFilesPerSubject)
		}
	})

	t.Run("delete", func(t *testing.T) {
		img := newImage(payload)
		delLicense := &models.License{
			UserID: user.ID, RegulatoryAuthority: "EASA", LicenseType: "IR",
			LicenseNumber: "IR-DEL-1", IssueDate: time.Now(), IssuingAuthority: "LBA",
		}
		if err := licenseRepo.Create(ctx, delLicense); err != nil {
			t.Fatalf("create license: %v", err)
		}
		img.LicenseID = &delLicense.ID
		if err := imageRepo.Create(ctx, img, models.MaxDocumentFilesPerSubject); err != nil {
			t.Fatalf("create: %v", err)
		}

		if err := imageRepo.Delete(ctx, user.ID, models.DocumentSubjectLicense, delLicense.ID, img.ID); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if err := imageRepo.Delete(ctx, user.ID, models.DocumentSubjectLicense, delLicense.ID, img.ID); !errors.Is(err, repository.ErrNotFound) {
			t.Errorf("second delete: err = %v, want ErrNotFound", err)
		}
	})

	// ON DELETE CASCADE on the FK is what keeps orphaned identity-document
	// scans from surviving the record they belong to.
	t.Run("deleting the licence cascades to its images", func(t *testing.T) {
		cascadeLicense := &models.License{
			UserID: user.ID, RegulatoryAuthority: "EASA", LicenseType: "ATPL",
			LicenseNumber: "ATPL-CASCADE-1", IssueDate: time.Now(), IssuingAuthority: "LBA",
		}
		if err := licenseRepo.Create(ctx, cascadeLicense); err != nil {
			t.Fatalf("create license: %v", err)
		}
		img := newImage(payload)
		img.LicenseID = &cascadeLicense.ID
		if err := imageRepo.Create(ctx, img, models.MaxDocumentFilesPerSubject); err != nil {
			t.Fatalf("create: %v", err)
		}

		if err := licenseRepo.Delete(ctx, cascadeLicense.ID); err != nil {
			t.Fatalf("delete license: %v", err)
		}
		count, err := imageRepo.CountBySubject(ctx, user.ID, models.DocumentSubjectLicense, cascadeLicense.ID)
		if err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Errorf("%d images survived the licence delete", count)
		}
	})
}

// The storage half of at-rest encryption: the nonce column round-trips with the
// payload, and the schema refuses the shapes the read path would have to
// second-guess — a wrong-length nonce, or none at all.
func TestDocumentFileRepositoryEncryptionIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	ctx := context.Background()
	userRepo := postgres.NewUserRepository(db)
	licenseRepo := postgres.NewLicenseRepository(db)
	fileRepo := postgres.NewDocumentFileRepository(db)

	user := testutil.CreateTestUser("docfile-crypto@example.com", "Crypto User", "hashedpass")
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	license := &models.License{
		UserID: user.ID, RegulatoryAuthority: "EASA", LicenseType: "PPL",
		LicenseNumber: "PPL-CRYPTO-1", IssueDate: time.Now(), IssuingAuthority: "LBA",
	}
	if err := licenseRepo.Create(ctx, license); err != nil {
		t.Fatalf("create license: %v", err)
	}

	newFile := func(data, nonce []byte) *models.DocumentFile {
		licenseID := license.ID
		return &models.DocumentFile{
			UserID:      user.ID,
			LicenseID:   &licenseID,
			ContentType: "image/png",
			ByteSize:    len(data),
			Data:        data,
			DataNonce:   nonce,
		}
	}

	ciphertext := []byte{0x9a, 0x11, 0x00, 0xff, 0x42}
	nonce := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}

	t.Run("the nonce round-trips with the payload", func(t *testing.T) {
		// A caller-supplied id must survive: the service seals the bytes
		// against it, so a database-generated substitute would make the file
		// unreadable.
		file := newFile(ciphertext, nonce)
		file.ID = uuid.New()
		wanted := file.ID
		if err := fileRepo.Create(ctx, file, models.MaxDocumentFilesPerSubject); err != nil {
			t.Fatalf("create: %v", err)
		}
		if file.ID != wanted {
			t.Fatalf("id = %v, want the one supplied (%v)", file.ID, wanted)
		}

		fetched, err := fileRepo.GetWithData(ctx, user.ID, models.DocumentSubjectLicense, license.ID, file.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if !bytes.Equal(fetched.Data, ciphertext) {
			t.Error("stored bytes differ from what was written")
		}
		if !bytes.Equal(fetched.DataNonce, nonce) {
			t.Errorf("nonce = %v, want %v", fetched.DataNonce, nonce)
		}
	})

	t.Run("a wrong-length nonce is refused by the database", func(t *testing.T) {
		file := newFile(ciphertext, []byte{1, 2, 3})
		if err := fileRepo.Create(ctx, file, models.MaxDocumentFilesPerSubject); err == nil {
			t.Fatal("a 3-byte nonce was accepted")
		}
	})

	t.Run("a row with no nonce is refused by the database", func(t *testing.T) {
		// The column is NOT NULL precisely so that "stored in the clear" is not
		// a state the schema can represent.
		file := newFile(ciphertext, nil)
		if err := fileRepo.Create(ctx, file, models.MaxDocumentFilesPerSubject); err == nil {
			t.Fatal("a row without a nonce was accepted")
		}
	})
}
