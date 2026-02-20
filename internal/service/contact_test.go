package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/google/uuid"
)

type mockContactRepo struct {
	contacts map[uuid.UUID]*models.Contact
}

func newMockContactRepo() *mockContactRepo {
	return &mockContactRepo{contacts: make(map[uuid.UUID]*models.Contact)}
}

func (m *mockContactRepo) Create(ctx context.Context, c *models.Contact) error {
	c.ID = uuid.New()
	c.CreatedAt = time.Now()
	c.UpdatedAt = time.Now()
	m.contacts[c.ID] = c
	return nil
}

func (m *mockContactRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Contact, error) {
	c, ok := m.contacts[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return c, nil
}

func (m *mockContactRepo) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*models.Contact, error) {
	var result []*models.Contact
	for _, c := range m.contacts {
		if c.UserID == userID {
			result = append(result, c)
		}
	}
	return result, nil
}

func (m *mockContactRepo) Search(ctx context.Context, userID uuid.UUID, query string, limit int) ([]*models.Contact, error) {
	var result []*models.Contact
	for _, c := range m.contacts {
		if c.UserID == userID {
			result = append(result, c)
		}
	}
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (m *mockContactRepo) Update(ctx context.Context, c *models.Contact) error {
	if _, ok := m.contacts[c.ID]; !ok {
		return repository.ErrNotFound
	}
	c.UpdatedAt = time.Now()
	m.contacts[c.ID] = c
	return nil
}

func (m *mockContactRepo) GetByExactName(ctx context.Context, userID uuid.UUID, name string) (*models.Contact, error) {
	for _, c := range m.contacts {
		if c.UserID == userID && strings.EqualFold(c.Name, name) {
			return c, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (m *mockContactRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if _, ok := m.contacts[id]; !ok {
		return repository.ErrNotFound
	}
	delete(m.contacts, id)
	return nil
}

func setupContactService() *service.ContactService {
	return service.NewContactService(newMockContactRepo())
}

func TestCreateContact(t *testing.T) {
	svc := setupContactService()
	ctx := context.Background()
	userID := uuid.New()

	contact := &models.Contact{UserID: userID, Name: "John Doe"}
	err := svc.CreateContact(ctx, contact)
	if err != nil {
		t.Fatalf("CreateContact() error = %v", err)
	}
	if contact.ID == uuid.Nil {
		t.Error("Expected contact ID to be set")
	}
}

func TestCreateContactEmptyName(t *testing.T) {
	svc := setupContactService()
	ctx := context.Background()

	contact := &models.Contact{UserID: uuid.New(), Name: ""}
	err := svc.CreateContact(ctx, contact)
	if err == nil {
		t.Error("Expected error for empty name")
	}
}

func TestGetContact(t *testing.T) {
	svc := setupContactService()
	ctx := context.Background()
	userID := uuid.New()

	contact := &models.Contact{UserID: userID, Name: "Jane Doe"}
	_ = svc.CreateContact(ctx, contact)

	retrieved, err := svc.GetContact(ctx, contact.ID, userID)
	if err != nil {
		t.Fatalf("GetContact() error = %v", err)
	}
	if retrieved.Name != "Jane Doe" {
		t.Errorf("Name = %s, want Jane Doe", retrieved.Name)
	}
}

func TestGetContactUnauthorized(t *testing.T) {
	svc := setupContactService()
	ctx := context.Background()
	userID := uuid.New()

	contact := &models.Contact{UserID: userID, Name: "Test"}
	_ = svc.CreateContact(ctx, contact)

	_, err := svc.GetContact(ctx, contact.ID, uuid.New())
	if err != service.ErrUnauthorizedContact {
		t.Errorf("Expected ErrUnauthorizedContact, got %v", err)
	}
}

func TestGetContactNotFound(t *testing.T) {
	svc := setupContactService()
	ctx := context.Background()

	_, err := svc.GetContact(ctx, uuid.New(), uuid.New())
	if err != service.ErrContactNotFound {
		t.Errorf("Expected ErrContactNotFound, got %v", err)
	}
}

func TestListContacts(t *testing.T) {
	svc := setupContactService()
	ctx := context.Background()
	userID := uuid.New()

	_ = svc.CreateContact(ctx, &models.Contact{UserID: userID, Name: "Alice"})
	_ = svc.CreateContact(ctx, &models.Contact{UserID: userID, Name: "Bob"})

	contacts, err := svc.ListContacts(ctx, userID)
	if err != nil {
		t.Fatalf("ListContacts() error = %v", err)
	}
	if len(contacts) != 2 {
		t.Errorf("ListContacts() count = %d, want 2", len(contacts))
	}
}

func TestSearchContacts(t *testing.T) {
	svc := setupContactService()
	ctx := context.Background()
	userID := uuid.New()

	_ = svc.CreateContact(ctx, &models.Contact{UserID: userID, Name: "Alice Smith"})

	results, err := svc.SearchContacts(ctx, userID, "alice", 10)
	if err != nil {
		t.Fatalf("SearchContacts() error = %v", err)
	}
	if len(results) != 1 {
		t.Errorf("SearchContacts() count = %d, want 1", len(results))
	}
}

func TestUpdateContact(t *testing.T) {
	svc := setupContactService()
	ctx := context.Background()
	userID := uuid.New()

	contact := &models.Contact{UserID: userID, Name: "Old Name"}
	_ = svc.CreateContact(ctx, contact)

	contact.Name = "New Name"
	err := svc.UpdateContact(ctx, contact, userID)
	if err != nil {
		t.Fatalf("UpdateContact() error = %v", err)
	}
}

func TestUpdateContactUnauthorized(t *testing.T) {
	svc := setupContactService()
	ctx := context.Background()
	userID := uuid.New()

	contact := &models.Contact{UserID: userID, Name: "Test"}
	_ = svc.CreateContact(ctx, contact)

	contact.Name = "Updated"
	err := svc.UpdateContact(ctx, contact, uuid.New())
	if err != service.ErrUnauthorizedContact {
		t.Errorf("Expected ErrUnauthorizedContact, got %v", err)
	}
}

func TestDeleteContact(t *testing.T) {
	svc := setupContactService()
	ctx := context.Background()
	userID := uuid.New()

	contact := &models.Contact{UserID: userID, Name: "To Delete"}
	_ = svc.CreateContact(ctx, contact)

	err := svc.DeleteContact(ctx, contact.ID, userID)
	if err != nil {
		t.Fatalf("DeleteContact() error = %v", err)
	}

	_, err = svc.GetContact(ctx, contact.ID, userID)
	if err != service.ErrContactNotFound {
		t.Errorf("Expected ErrContactNotFound after delete, got %v", err)
	}
}

func TestDeleteContactNotFound(t *testing.T) {
	svc := setupContactService()
	ctx := context.Background()

	err := svc.DeleteContact(ctx, uuid.New(), uuid.New())
	if err != service.ErrContactNotFound {
		t.Errorf("Expected ErrContactNotFound, got %v", err)
	}
}

func TestFindOrCreateContact_CreatesNew(t *testing.T) {
	svc := setupContactService()
	ctx := context.Background()
	userID := uuid.New()

	contact, created, err := svc.FindOrCreateContact(ctx, userID, "New Person")
	if err != nil {
		t.Fatalf("FindOrCreateContact() error = %v", err)
	}
	if !created {
		t.Error("Expected created=true for new contact")
	}
	if contact.Name != "New Person" {
		t.Errorf("Name = %q, want 'New Person'", contact.Name)
	}
	if contact.UserID != userID {
		t.Error("Contact should belong to the correct user")
	}
}

func TestFindOrCreateContact_FindsExisting(t *testing.T) {
	svc := setupContactService()
	ctx := context.Background()
	userID := uuid.New()

	// Create first
	c1, created1, err := svc.FindOrCreateContact(ctx, userID, "Existing Person")
	if err != nil {
		t.Fatalf("First FindOrCreateContact() error = %v", err)
	}
	if !created1 {
		t.Error("Expected created=true for first call")
	}

	// Find again
	c2, created2, err := svc.FindOrCreateContact(ctx, userID, "Existing Person")
	if err != nil {
		t.Fatalf("Second FindOrCreateContact() error = %v", err)
	}
	if created2 {
		t.Error("Expected created=false for second call")
	}
	if c2.ID != c1.ID {
		t.Error("Expected same contact ID for existing contact")
	}
}

func TestFindOrCreateContact_EmptyName(t *testing.T) {
	svc := setupContactService()
	ctx := context.Background()

	_, _, err := svc.FindOrCreateContact(ctx, uuid.New(), "")
	if err == nil {
		t.Error("Expected error for empty name")
	}
}

func TestFindOrCreateContact_WhitespaceName(t *testing.T) {
	svc := setupContactService()
	ctx := context.Background()

	_, _, err := svc.FindOrCreateContact(ctx, uuid.New(), "   ")
	if err == nil {
		t.Error("Expected error for whitespace-only name")
	}
}
