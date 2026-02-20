package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/google/uuid"
)

type mockClassRatingRepo struct {
	ratings map[uuid.UUID]*models.ClassRating
}

func newMockClassRatingRepo() *mockClassRatingRepo {
	return &mockClassRatingRepo{ratings: make(map[uuid.UUID]*models.ClassRating)}
}

func (m *mockClassRatingRepo) Create(ctx context.Context, cr *models.ClassRating) error {
	cr.ID = uuid.New()
	cr.CreatedAt = time.Now()
	cr.UpdatedAt = time.Now()
	m.ratings[cr.ID] = cr
	return nil
}

func (m *mockClassRatingRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.ClassRating, error) {
	cr, ok := m.ratings[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return cr, nil
}

func (m *mockClassRatingRepo) GetByLicenseID(ctx context.Context, licenseID uuid.UUID) ([]*models.ClassRating, error) {
	var result []*models.ClassRating
	for _, cr := range m.ratings {
		if cr.LicenseID == licenseID {
			result = append(result, cr)
		}
	}
	return result, nil
}

func (m *mockClassRatingRepo) Update(ctx context.Context, cr *models.ClassRating) error {
	if _, ok := m.ratings[cr.ID]; !ok {
		return repository.ErrNotFound
	}
	cr.UpdatedAt = time.Now()
	m.ratings[cr.ID] = cr
	return nil
}

func (m *mockClassRatingRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if _, ok := m.ratings[id]; !ok {
		return repository.ErrNotFound
	}
	delete(m.ratings, id)
	return nil
}

// mockCRLicenseRepo for class rating tests
type mockCRLicenseRepo struct {
	licenses map[uuid.UUID]*models.License
}

func newMockCRLicenseRepo() *mockCRLicenseRepo {
	return &mockCRLicenseRepo{licenses: make(map[uuid.UUID]*models.License)}
}

func (m *mockCRLicenseRepo) Create(ctx context.Context, lic *models.License) error {
	lic.ID = uuid.New()
	m.licenses[lic.ID] = lic
	return nil
}

func (m *mockCRLicenseRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.License, error) {
	l, ok := m.licenses[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return l, nil
}

func (m *mockCRLicenseRepo) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*models.License, error) {
	return nil, nil
}

func (m *mockCRLicenseRepo) Update(ctx context.Context, lic *models.License) error {
	m.licenses[lic.ID] = lic
	return nil
}

func (m *mockCRLicenseRepo) Delete(ctx context.Context, id uuid.UUID) error {
	delete(m.licenses, id)
	return nil
}

func setupClassRatingTest() (*service.ClassRatingService, *mockCRLicenseRepo) {
	crRepo := newMockClassRatingRepo()
	licRepo := newMockCRLicenseRepo()
	svc := service.NewClassRatingService(crRepo, licRepo)
	return svc, licRepo
}

func TestCreateClassRating(t *testing.T) {
	svc, licRepo := setupClassRatingTest()
	ctx := context.Background()
	userID := uuid.New()

	lic := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "EASA", LicenseType: "PPL"}
	licRepo.licenses[lic.ID] = lic

	cr := &models.ClassRating{
		LicenseID: lic.ID,
		ClassType: models.ClassTypeSEPLand,
		IssueDate: time.Now(),
	}

	err := svc.CreateClassRating(ctx, cr, userID)
	if err != nil {
		t.Fatalf("CreateClassRating() error = %v", err)
	}
	if cr.ID == uuid.Nil {
		t.Error("Expected ID to be set")
	}
}

func TestCreateClassRating_InvalidType(t *testing.T) {
	svc, licRepo := setupClassRatingTest()
	ctx := context.Background()
	userID := uuid.New()

	lic := &models.License{ID: uuid.New(), UserID: userID}
	licRepo.licenses[lic.ID] = lic

	cr := &models.ClassRating{LicenseID: lic.ID, ClassType: "INVALID", IssueDate: time.Now()}
	err := svc.CreateClassRating(ctx, cr, userID)
	if err != service.ErrInvalidClassType {
		t.Errorf("Expected ErrInvalidClassType, got %v", err)
	}
}

func TestCreateClassRating_Unauthorized(t *testing.T) {
	svc, licRepo := setupClassRatingTest()
	ctx := context.Background()

	lic := &models.License{ID: uuid.New(), UserID: uuid.New()}
	licRepo.licenses[lic.ID] = lic

	cr := &models.ClassRating{LicenseID: lic.ID, ClassType: models.ClassTypeSEPLand, IssueDate: time.Now()}
	err := svc.CreateClassRating(ctx, cr, uuid.New())
	if err != service.ErrUnauthorizedAccess {
		t.Errorf("Expected ErrUnauthorizedAccess, got %v", err)
	}
}

func TestListClassRatings(t *testing.T) {
	svc, licRepo := setupClassRatingTest()
	ctx := context.Background()
	userID := uuid.New()

	lic := &models.License{ID: uuid.New(), UserID: userID}
	licRepo.licenses[lic.ID] = lic

	_ = svc.CreateClassRating(ctx, &models.ClassRating{LicenseID: lic.ID, ClassType: models.ClassTypeSEPLand, IssueDate: time.Now()}, userID)
	_ = svc.CreateClassRating(ctx, &models.ClassRating{LicenseID: lic.ID, ClassType: models.ClassTypeIR, IssueDate: time.Now()}, userID)

	ratings, err := svc.ListClassRatings(ctx, lic.ID, userID)
	if err != nil {
		t.Fatalf("ListClassRatings() error = %v", err)
	}
	if len(ratings) != 2 {
		t.Errorf("ListClassRatings() count = %d, want 2", len(ratings))
	}
}

func TestDeleteClassRating(t *testing.T) {
	svc, licRepo := setupClassRatingTest()
	ctx := context.Background()
	userID := uuid.New()

	lic := &models.License{ID: uuid.New(), UserID: userID}
	licRepo.licenses[lic.ID] = lic

	cr := &models.ClassRating{LicenseID: lic.ID, ClassType: models.ClassTypeMEPLand, IssueDate: time.Now()}
	_ = svc.CreateClassRating(ctx, cr, userID)

	err := svc.DeleteClassRating(ctx, cr.ID, userID)
	if err != nil {
		t.Fatalf("DeleteClassRating() error = %v", err)
	}

	ratings, _ := svc.ListClassRatings(ctx, lic.ID, userID)
	if len(ratings) != 0 {
		t.Errorf("Expected 0 ratings after delete, got %d", len(ratings))
	}
}

func TestDeleteClassRating_NotFound(t *testing.T) {
	svc, _ := setupClassRatingTest()
	err := svc.DeleteClassRating(context.Background(), uuid.New(), uuid.New())
	if err != service.ErrClassRatingNotFound {
		t.Errorf("Expected ErrClassRatingNotFound, got %v", err)
	}
}
