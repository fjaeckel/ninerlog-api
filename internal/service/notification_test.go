package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/fjaeckel/pilotlog-api/internal/repository"
	"github.com/fjaeckel/pilotlog-api/internal/service"
	"github.com/fjaeckel/pilotlog-api/pkg/email"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// mockNotificationRepo implements repository.NotificationRepository
type mockNotificationRepo struct {
	prefs    map[uuid.UUID]*models.NotificationPreferences
	logs     []*models.NotificationLog
	allPrefs []*models.NotificationPreferences
}

func newMockNotificationRepo() *mockNotificationRepo {
	return &mockNotificationRepo{
		prefs: make(map[uuid.UUID]*models.NotificationPreferences),
	}
}

func (m *mockNotificationRepo) GetPreferences(ctx context.Context, userID uuid.UUID) (*models.NotificationPreferences, error) {
	p, ok := m.prefs[userID]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return p, nil
}

func (m *mockNotificationRepo) UpsertPreferences(ctx context.Context, prefs *models.NotificationPreferences) error {
	if prefs.ID == uuid.Nil {
		prefs.ID = uuid.New()
	}
	prefs.UpdatedAt = time.Now()
	m.prefs[prefs.UserID] = prefs
	return nil
}

func (m *mockNotificationRepo) LogNotification(ctx context.Context, log *models.NotificationLog) error {
	log.ID = uuid.New()
	log.SentAt = time.Now()
	m.logs = append(m.logs, log)
	return nil
}

func (m *mockNotificationRepo) HasBeenSent(ctx context.Context, userID uuid.UUID, notificationType string, referenceID uuid.UUID, daysBeforeExpiry int) (bool, error) {
	for _, l := range m.logs {
		if l.UserID == userID && l.NotificationType == notificationType && l.ReferenceID != nil && *l.ReferenceID == referenceID {
			if l.DaysBeforeExpiry != nil && *l.DaysBeforeExpiry == daysBeforeExpiry {
				return true, nil
			}
		}
	}
	return false, nil
}

func (m *mockNotificationRepo) GetAllUsersWithPreferences(ctx context.Context) ([]*models.NotificationPreferences, error) {
	return m.allPrefs, nil
}

// mockNotifUserRepo for notification tests
type mockNotifUserRepo struct {
	users map[uuid.UUID]*models.User
}

func newMockNotifUserRepo() *mockNotifUserRepo {
	return &mockNotifUserRepo{users: make(map[uuid.UUID]*models.User)}
}

func (m *mockNotifUserRepo) Create(ctx context.Context, user *models.User) error {
	user.ID = uuid.New()
	m.users[user.ID] = user
	return nil
}

func (m *mockNotifUserRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return u, nil
}

func (m *mockNotifUserRepo) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (m *mockNotifUserRepo) Update(ctx context.Context, user *models.User) error {
	m.users[user.ID] = user
	return nil
}

func (m *mockNotifUserRepo) Delete(ctx context.Context, id uuid.UUID) error {
	delete(m.users, id)
	return nil
}

// mockNotifCredentialRepo for notification tests
type mockNotifCredentialRepo struct {
	creds map[uuid.UUID]*models.Credential
}

func newMockNotifCredentialRepo() *mockNotifCredentialRepo {
	return &mockNotifCredentialRepo{creds: make(map[uuid.UUID]*models.Credential)}
}

func (m *mockNotifCredentialRepo) Create(ctx context.Context, cred *models.Credential) error {
	cred.ID = uuid.New()
	m.creds[cred.ID] = cred
	return nil
}

func (m *mockNotifCredentialRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Credential, error) {
	c, ok := m.creds[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return c, nil
}

func (m *mockNotifCredentialRepo) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*models.Credential, error) {
	var result []*models.Credential
	for _, c := range m.creds {
		if c.UserID == userID {
			result = append(result, c)
		}
	}
	return result, nil
}

func (m *mockNotifCredentialRepo) Update(ctx context.Context, cred *models.Credential) error {
	m.creds[cred.ID] = cred
	return nil
}

func (m *mockNotifCredentialRepo) Delete(ctx context.Context, id uuid.UUID) error {
	delete(m.creds, id)
	return nil
}

// mockNotifFlightRepo for notification tests
type mockNotifFlightRepo struct {
	currencyData *models.CurrencyData
}

func newMockNotifFlightRepo() *mockNotifFlightRepo {
	return &mockNotifFlightRepo{
		currencyData: &models.CurrencyData{Flights: 5, TotalLandings: 10, DayLandings: 5, NightLandings: 5},
	}
}

func (m *mockNotifFlightRepo) Create(ctx context.Context, flight *models.Flight) error { return nil }
func (m *mockNotifFlightRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Flight, error) {
	return nil, repository.ErrNotFound
}
func (m *mockNotifFlightRepo) GetByUserID(ctx context.Context, userID uuid.UUID, opts *repository.FlightQueryOptions) ([]*models.Flight, error) {
	return nil, nil
}
func (m *mockNotifFlightRepo) GetByLicenseID(ctx context.Context, licenseID uuid.UUID, opts *repository.FlightQueryOptions) ([]*models.Flight, error) {
	return nil, nil
}
func (m *mockNotifFlightRepo) Update(ctx context.Context, flight *models.Flight) error { return nil }
func (m *mockNotifFlightRepo) Delete(ctx context.Context, id uuid.UUID) error          { return nil }
func (m *mockNotifFlightRepo) CountByUserID(ctx context.Context, userID uuid.UUID, opts *repository.FlightQueryOptions) (int, error) {
	return 0, nil
}
func (m *mockNotifFlightRepo) GetStatsByLicenseID(ctx context.Context, licenseID uuid.UUID, startDate, endDate *time.Time) (*models.FlightStatistics, error) {
	return &models.FlightStatistics{}, nil
}
func (m *mockNotifFlightRepo) GetCurrencyData(ctx context.Context, licenseID uuid.UUID, since time.Time) (*models.CurrencyData, error) {
	return m.currencyData, nil
}

// mockNotifLicenseRepo for notification tests
type mockNotifLicenseRepo struct {
	licenses map[uuid.UUID]*models.License
}

func newMockNotifLicenseRepo() *mockNotifLicenseRepo {
	return &mockNotifLicenseRepo{licenses: make(map[uuid.UUID]*models.License)}
}

func (m *mockNotifLicenseRepo) Create(ctx context.Context, lic *models.License) error {
	lic.ID = uuid.New()
	m.licenses[lic.ID] = lic
	return nil
}

func (m *mockNotifLicenseRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.License, error) {
	l, ok := m.licenses[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return l, nil
}

func (m *mockNotifLicenseRepo) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*models.License, error) {
	var result []*models.License
	for _, l := range m.licenses {
		if l.UserID == userID {
			result = append(result, l)
		}
	}
	return result, nil
}

func (m *mockNotifLicenseRepo) GetActiveByUserID(ctx context.Context, userID uuid.UUID) ([]*models.License, error) {
	var result []*models.License
	for _, l := range m.licenses {
		if l.UserID == userID && l.IsActive {
			result = append(result, l)
		}
	}
	return result, nil
}

func (m *mockNotifLicenseRepo) Update(ctx context.Context, lic *models.License) error {
	m.licenses[lic.ID] = lic
	return nil
}

func (m *mockNotifLicenseRepo) Delete(ctx context.Context, id uuid.UUID) error {
	delete(m.licenses, id)
	return nil
}

func TestGetPreferences(t *testing.T) {
	notifRepo := newMockNotificationRepo()
	userID := uuid.New()
	notifRepo.prefs[userID] = &models.NotificationPreferences{
		ID:                 uuid.New(),
		UserID:             userID,
		EmailEnabled:       true,
		CurrencyWarnings:   true,
		CredentialWarnings: true,
		WarningDays:        pq.Int64Array{30, 14, 7},
	}

	svc := service.NewNotificationService(notifRepo, newMockNotifCredentialRepo(), newMockNotifFlightRepo(), newMockNotifLicenseRepo(), newMockNotifUserRepo(), email.NewSender(&email.SMTPConfig{}))
	ctx := context.Background()

	prefs, err := svc.GetPreferences(ctx, userID)
	if err != nil {
		t.Fatalf("GetPreferences() error = %v", err)
	}
	if !prefs.EmailEnabled {
		t.Error("Expected EmailEnabled to be true")
	}
	if len(prefs.WarningDays) != 3 {
		t.Errorf("WarningDays length = %d, want 3", len(prefs.WarningDays))
	}
}

func TestGetPreferencesNotFound(t *testing.T) {
	svc := service.NewNotificationService(newMockNotificationRepo(), newMockNotifCredentialRepo(), newMockNotifFlightRepo(), newMockNotifLicenseRepo(), newMockNotifUserRepo(), email.NewSender(&email.SMTPConfig{}))
	ctx := context.Background()

	_, err := svc.GetPreferences(ctx, uuid.New())
	if err == nil {
		t.Error("Expected error for non-existent preferences")
	}
}

func TestUpdatePreferences(t *testing.T) {
	svc := service.NewNotificationService(newMockNotificationRepo(), newMockNotifCredentialRepo(), newMockNotifFlightRepo(), newMockNotifLicenseRepo(), newMockNotifUserRepo(), email.NewSender(&email.SMTPConfig{}))
	ctx := context.Background()
	userID := uuid.New()

	prefs := &models.NotificationPreferences{
		UserID:             userID,
		EmailEnabled:       true,
		CurrencyWarnings:   true,
		CredentialWarnings: false,
		WarningDays:        pq.Int64Array{90, 60, 30},
	}

	err := svc.UpdatePreferences(ctx, prefs)
	if err != nil {
		t.Fatalf("UpdatePreferences() error = %v", err)
	}
	if prefs.ID == uuid.Nil {
		t.Error("Expected ID to be set after upsert")
	}

	// Verify by retrieving
	retrieved, err := svc.GetPreferences(ctx, userID)
	if err != nil {
		t.Fatalf("GetPreferences() after update error = %v", err)
	}
	if !retrieved.EmailEnabled {
		t.Error("EmailEnabled should be true")
	}
	if retrieved.CredentialWarnings {
		t.Error("CredentialWarnings should be false")
	}
}

func TestUpdatePreferencesOverwrite(t *testing.T) {
	svc := service.NewNotificationService(newMockNotificationRepo(), newMockNotifCredentialRepo(), newMockNotifFlightRepo(), newMockNotifLicenseRepo(), newMockNotifUserRepo(), email.NewSender(&email.SMTPConfig{}))
	ctx := context.Background()
	userID := uuid.New()

	prefs1 := &models.NotificationPreferences{
		UserID:       userID,
		EmailEnabled: true,
		WarningDays:  pq.Int64Array{30},
	}
	_ = svc.UpdatePreferences(ctx, prefs1)

	prefs2 := &models.NotificationPreferences{
		UserID:       userID,
		EmailEnabled: false,
		WarningDays:  pq.Int64Array{60, 30},
	}
	err := svc.UpdatePreferences(ctx, prefs2)
	if err != nil {
		t.Fatalf("UpdatePreferences() overwrite error = %v", err)
	}

	retrieved, _ := svc.GetPreferences(ctx, userID)
	if retrieved.EmailEnabled {
		t.Error("EmailEnabled should be false after overwrite")
	}
	if len(retrieved.WarningDays) != 2 {
		t.Errorf("WarningDays length = %d, want 2", len(retrieved.WarningDays))
	}
}
