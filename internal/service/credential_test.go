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

type mockCredentialRepo struct {
	credentials map[uuid.UUID]*models.Credential
}

func newMockCredentialRepo() *mockCredentialRepo {
	return &mockCredentialRepo{credentials: make(map[uuid.UUID]*models.Credential)}
}

func (m *mockCredentialRepo) Create(ctx context.Context, cred *models.Credential) error {
	cred.ID = uuid.New()
	cred.CreatedAt = time.Now()
	cred.UpdatedAt = time.Now()
	m.credentials[cred.ID] = cred
	return nil
}

func (m *mockCredentialRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Credential, error) {
	c, ok := m.credentials[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return c, nil
}

func (m *mockCredentialRepo) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*models.Credential, error) {
	var result []*models.Credential
	for _, c := range m.credentials {
		if c.UserID == userID {
			result = append(result, c)
		}
	}
	return result, nil
}

func (m *mockCredentialRepo) Update(ctx context.Context, cred *models.Credential) error {
	if _, ok := m.credentials[cred.ID]; !ok {
		return repository.ErrNotFound
	}
	cred.UpdatedAt = time.Now()
	m.credentials[cred.ID] = cred
	return nil
}

func (m *mockCredentialRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if _, ok := m.credentials[id]; !ok {
		return repository.ErrNotFound
	}
	delete(m.credentials, id)
	return nil
}

func setupCredentialService() *service.CredentialService {
	return service.NewCredentialService(newMockCredentialRepo())
}

func TestCreateCredential(t *testing.T) {
	svc := setupCredentialService()
	ctx := context.Background()
	userID := uuid.New()

	cred := &models.Credential{
		UserID:           userID,
		CredentialType:   models.CredentialTypeEASAClass1Medical,
		IssueDate:        time.Now(),
		IssuingAuthority: "EASA",
	}

	err := svc.CreateCredential(ctx, cred)
	if err != nil {
		t.Fatalf("CreateCredential() error = %v", err)
	}
	if cred.ID == uuid.Nil {
		t.Error("Expected credential ID to be set")
	}
}

func TestGetCredential(t *testing.T) {
	svc := setupCredentialService()
	ctx := context.Background()
	userID := uuid.New()

	cred := &models.Credential{
		UserID:           userID,
		CredentialType:   models.CredentialTypeFAAClass2Medical,
		IssueDate:        time.Now(),
		IssuingAuthority: "FAA",
	}
	_ = svc.CreateCredential(ctx, cred)

	retrieved, err := svc.GetCredential(ctx, cred.ID, userID)
	if err != nil {
		t.Fatalf("GetCredential() error = %v", err)
	}
	if retrieved.CredentialType != models.CredentialTypeFAAClass2Medical {
		t.Errorf("CredentialType = %s, want FAA_CLASS2_MEDICAL", retrieved.CredentialType)
	}
}

func TestGetCredentialNotFound(t *testing.T) {
	svc := setupCredentialService()
	ctx := context.Background()

	_, err := svc.GetCredential(ctx, uuid.New(), uuid.New())
	if err != service.ErrCredentialNotFound {
		t.Errorf("Expected ErrCredentialNotFound, got %v", err)
	}
}

func TestGetCredentialUnauthorized(t *testing.T) {
	svc := setupCredentialService()
	ctx := context.Background()
	userID := uuid.New()
	otherUserID := uuid.New()

	cred := &models.Credential{
		UserID:           userID,
		CredentialType:   models.CredentialTypeEASAClass1Medical,
		IssueDate:        time.Now(),
		IssuingAuthority: "EASA",
	}
	_ = svc.CreateCredential(ctx, cred)

	_, err := svc.GetCredential(ctx, cred.ID, otherUserID)
	if err != service.ErrUnauthorizedCredential {
		t.Errorf("Expected ErrUnauthorizedCredential, got %v", err)
	}
}

func TestListCredentials(t *testing.T) {
	svc := setupCredentialService()
	ctx := context.Background()
	userID := uuid.New()

	cred1 := &models.Credential{UserID: userID, CredentialType: models.CredentialTypeEASAClass1Medical, IssueDate: time.Now(), IssuingAuthority: "EASA"}
	cred2 := &models.Credential{UserID: userID, CredentialType: models.CredentialTypeLangICAOLevel4, IssueDate: time.Now(), IssuingAuthority: "EASA"}
	_ = svc.CreateCredential(ctx, cred1)
	_ = svc.CreateCredential(ctx, cred2)

	creds, err := svc.ListCredentials(ctx, userID)
	if err != nil {
		t.Fatalf("ListCredentials() error = %v", err)
	}
	if len(creds) != 2 {
		t.Errorf("ListCredentials() count = %d, want 2", len(creds))
	}
}

func TestListCredentialsEmpty(t *testing.T) {
	svc := setupCredentialService()
	ctx := context.Background()

	creds, err := svc.ListCredentials(ctx, uuid.New())
	if err != nil {
		t.Fatalf("ListCredentials() error = %v", err)
	}
	if len(creds) != 0 {
		t.Errorf("ListCredentials() count = %d, want 0", len(creds))
	}
}

func TestUpdateCredential(t *testing.T) {
	svc := setupCredentialService()
	ctx := context.Background()
	userID := uuid.New()

	cred := &models.Credential{
		UserID:           userID,
		CredentialType:   models.CredentialTypeEASAClass1Medical,
		IssueDate:        time.Now(),
		IssuingAuthority: "EASA",
	}
	_ = svc.CreateCredential(ctx, cred)

	expiry := time.Now().Add(365 * 24 * time.Hour)
	cred.ExpiryDate = &expiry
	err := svc.UpdateCredential(ctx, cred, userID)
	if err != nil {
		t.Fatalf("UpdateCredential() error = %v", err)
	}
}

func TestUpdateCredentialUnauthorized(t *testing.T) {
	svc := setupCredentialService()
	ctx := context.Background()
	userID := uuid.New()

	cred := &models.Credential{
		UserID:           userID,
		CredentialType:   models.CredentialTypeEASAClass1Medical,
		IssueDate:        time.Now(),
		IssuingAuthority: "EASA",
	}
	_ = svc.CreateCredential(ctx, cred)

	err := svc.UpdateCredential(ctx, cred, uuid.New())
	if err != service.ErrUnauthorizedCredential {
		t.Errorf("Expected ErrUnauthorizedCredential, got %v", err)
	}
}

func TestUpdateCredentialNotFound(t *testing.T) {
	svc := setupCredentialService()
	ctx := context.Background()

	cred := &models.Credential{ID: uuid.New()}
	err := svc.UpdateCredential(ctx, cred, uuid.New())
	if err != service.ErrCredentialNotFound {
		t.Errorf("Expected ErrCredentialNotFound, got %v", err)
	}
}

func TestDeleteCredential(t *testing.T) {
	svc := setupCredentialService()
	ctx := context.Background()
	userID := uuid.New()

	cred := &models.Credential{
		UserID:           userID,
		CredentialType:   models.CredentialTypeEASAClass1Medical,
		IssueDate:        time.Now(),
		IssuingAuthority: "EASA",
	}
	_ = svc.CreateCredential(ctx, cred)

	err := svc.DeleteCredential(ctx, cred.ID, userID)
	if err != nil {
		t.Fatalf("DeleteCredential() error = %v", err)
	}

	_, err = svc.GetCredential(ctx, cred.ID, userID)
	if err != service.ErrCredentialNotFound {
		t.Errorf("Expected ErrCredentialNotFound after delete, got %v", err)
	}
}

func TestDeleteCredentialUnauthorized(t *testing.T) {
	svc := setupCredentialService()
	ctx := context.Background()
	userID := uuid.New()

	cred := &models.Credential{
		UserID:           userID,
		CredentialType:   models.CredentialTypeEASAClass1Medical,
		IssueDate:        time.Now(),
		IssuingAuthority: "EASA",
	}
	_ = svc.CreateCredential(ctx, cred)

	err := svc.DeleteCredential(ctx, cred.ID, uuid.New())
	if err != service.ErrUnauthorizedCredential {
		t.Errorf("Expected ErrUnauthorizedCredential, got %v", err)
	}
}

func TestDeleteCredentialNotFound(t *testing.T) {
	svc := setupCredentialService()
	ctx := context.Background()

	err := svc.DeleteCredential(ctx, uuid.New(), uuid.New())
	if err != service.ErrCredentialNotFound {
		t.Errorf("Expected ErrCredentialNotFound, got %v", err)
	}
}

func TestCreateCredential_ExpiryBeforeIssue(t *testing.T) {
	svc := setupCredentialService()
	ctx := context.Background()
	userID := uuid.New()

	issueDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	expiryDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) // before issue date

	cred := &models.Credential{
		UserID:           userID,
		CredentialType:   models.CredentialTypeEASAClass1Medical,
		IssueDate:        issueDate,
		ExpiryDate:       &expiryDate,
		IssuingAuthority: "EASA",
	}

	err := svc.CreateCredential(ctx, cred)
	if err != service.ErrExpiryBeforeIssue {
		t.Errorf("Expected ErrExpiryBeforeIssue, got %v", err)
	}
}

func TestUpdateCredential_ExpiryBeforeIssue(t *testing.T) {
	svc := setupCredentialService()
	ctx := context.Background()
	userID := uuid.New()

	cred := &models.Credential{
		UserID:           userID,
		CredentialType:   models.CredentialTypeEASAClass1Medical,
		IssueDate:        time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		IssuingAuthority: "EASA",
	}
	_ = svc.CreateCredential(ctx, cred)

	expiryDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cred.ExpiryDate = &expiryDate
	err := svc.UpdateCredential(ctx, cred, userID)
	if err != service.ErrExpiryBeforeIssue {
		t.Errorf("Expected ErrExpiryBeforeIssue, got %v", err)
	}
}

func TestCreateCredential_NilExpiry(t *testing.T) {
	svc := setupCredentialService()
	ctx := context.Background()
	userID := uuid.New()

	cred := &models.Credential{
		UserID:           userID,
		CredentialType:   models.CredentialTypeFAAClass3Medical,
		IssueDate:        time.Now(),
		ExpiryDate:       nil,
		IssuingAuthority: "FAA",
	}

	err := svc.CreateCredential(ctx, cred)
	if err != nil {
		t.Fatalf("CreateCredential with nil expiry should succeed, got %v", err)
	}
}
