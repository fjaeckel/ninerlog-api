package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/pkg/email"
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

func (m *mockNotificationRepo) HasBeenSent(ctx context.Context, userID uuid.UUID, notificationType string, referenceID uuid.UUID, daysBeforeExpiry int, expiryReferenceDate *time.Time) (bool, error) {
	for _, l := range m.logs {
		if l.UserID == userID && l.NotificationType == notificationType && l.ReferenceID != nil && *l.ReferenceID == referenceID {
			if l.DaysBeforeExpiry != nil && *l.DaysBeforeExpiry == daysBeforeExpiry {
				// Check expiry reference date match
				if expiryReferenceDate == nil && l.ExpiryReferenceDate == nil {
					return true, nil
				}
				if expiryReferenceDate != nil && l.ExpiryReferenceDate != nil {
					if expiryReferenceDate.Format("2006-01-02") == l.ExpiryReferenceDate.Format("2006-01-02") {
						return true, nil
					}
				}
			}
		}
	}
	return false, nil
}

func (m *mockNotificationRepo) GetAllUsersWithPreferences(ctx context.Context) ([]*models.NotificationPreferences, error) {
	return m.allPrefs, nil
}

func (m *mockNotificationRepo) GetNotificationHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*models.NotificationLog, int, error) {
	var result []*models.NotificationLog
	for _, l := range m.logs {
		if l.UserID == userID {
			result = append(result, l)
		}
	}
	total := len(result)
	if offset >= len(result) {
		return []*models.NotificationLog{}, total, nil
	}
	end := offset + limit
	if end > len(result) {
		end = len(result)
	}
	return result[offset:end], total, nil
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

func (m *mockNotifUserRepo) IncrementFailedLoginAttempts(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockNotifUserRepo) ResetFailedLoginAttempts(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockNotifUserRepo) LockAccount(ctx context.Context, id uuid.UUID, until time.Time) error {
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
func (m *mockNotifFlightRepo) Update(ctx context.Context, flight *models.Flight) error { return nil }
func (m *mockNotifFlightRepo) Delete(ctx context.Context, id uuid.UUID) error          { return nil }
func (m *mockNotifFlightRepo) DeleteAllByUserID(ctx context.Context, userID uuid.UUID) (int64, error) {
	return 0, nil
}
func (m *mockNotifFlightRepo) CountByUserID(ctx context.Context, userID uuid.UUID, opts *repository.FlightQueryOptions) (int, error) {
	return 0, nil
}
func (m *mockNotifFlightRepo) GetStatsByUserID(ctx context.Context, userID uuid.UUID, startDate, endDate *time.Time) (*models.FlightStatistics, error) {
	return &models.FlightStatistics{}, nil
}
func (m *mockNotifFlightRepo) GetCurrencyData(ctx context.Context, userID uuid.UUID, since time.Time) (*models.CurrencyData, error) {
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

func (m *mockNotifLicenseRepo) Update(ctx context.Context, lic *models.License) error {
	m.licenses[lic.ID] = lic
	return nil
}

func (m *mockNotifLicenseRepo) Delete(ctx context.Context, id uuid.UUID) error {
	delete(m.licenses, id)
	return nil
}

func newTestNotifService(notifRepo *mockNotificationRepo) *service.NotificationService {
	return service.NewNotificationService(
		notifRepo,
		newMockNotifCredentialRepo(),
		newMockNotifFlightRepo(),
		newMockNotifLicenseRepo(),
		newMockNotifUserRepo(),
		email.NewSender(&email.SMTPConfig{}),
		nil, // currencyService — nil is OK for preference tests
	)
}

func TestGetPreferences(t *testing.T) {
	notifRepo := newMockNotificationRepo()
	userID := uuid.New()
	notifRepo.prefs[userID] = &models.NotificationPreferences{
		ID:                uuid.New(),
		UserID:            userID,
		EmailEnabled:      true,
		EnabledCategories: models.AllNotificationCategories,
		WarningDays:       pq.Int64Array{30, 14, 7},
		CheckHour:         8,
	}

	svc := newTestNotifService(notifRepo)
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
	if prefs.CheckHour != 8 {
		t.Errorf("CheckHour = %d, want 8", prefs.CheckHour)
	}
	if len(prefs.EnabledCategories) != 10 {
		t.Errorf("EnabledCategories length = %d, want 10", len(prefs.EnabledCategories))
	}
}

func TestGetPreferencesNotFound(t *testing.T) {
	svc := newTestNotifService(newMockNotificationRepo())
	ctx := context.Background()

	_, err := svc.GetPreferences(ctx, uuid.New())
	if err == nil {
		t.Error("Expected error for non-existent preferences")
	}
}

func TestUpdatePreferences(t *testing.T) {
	svc := newTestNotifService(newMockNotificationRepo())
	ctx := context.Background()
	userID := uuid.New()

	prefs := &models.NotificationPreferences{
		UserID:            userID,
		EmailEnabled:      true,
		EnabledCategories: pq.StringArray{string(models.NotifCategoryCredentialMedical), string(models.NotifCategoryRatingExpiry)},
		WarningDays:       pq.Int64Array{90, 60, 30},
		CheckHour:         14,
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
	if len(retrieved.EnabledCategories) != 2 {
		t.Errorf("EnabledCategories length = %d, want 2", len(retrieved.EnabledCategories))
	}
	if retrieved.CheckHour != 14 {
		t.Errorf("CheckHour = %d, want 14", retrieved.CheckHour)
	}
}

func TestUpdatePreferencesOverwrite(t *testing.T) {
	svc := newTestNotifService(newMockNotificationRepo())
	ctx := context.Background()
	userID := uuid.New()

	prefs1 := &models.NotificationPreferences{
		UserID:            userID,
		EmailEnabled:      true,
		EnabledCategories: models.AllNotificationCategories,
		WarningDays:       pq.Int64Array{30},
		CheckHour:         8,
	}
	_ = svc.UpdatePreferences(ctx, prefs1)

	prefs2 := &models.NotificationPreferences{
		UserID:            userID,
		EmailEnabled:      false,
		EnabledCategories: pq.StringArray{string(models.NotifCategoryCredentialMedical)},
		WarningDays:       pq.Int64Array{60, 30},
		CheckHour:         12,
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
	if len(retrieved.EnabledCategories) != 1 {
		t.Errorf("EnabledCategories length = %d, want 1", len(retrieved.EnabledCategories))
	}
	if retrieved.CheckHour != 12 {
		t.Errorf("CheckHour = %d, want 12", retrieved.CheckHour)
	}
}

func TestGetNotificationHistory(t *testing.T) {
	notifRepo := newMockNotificationRepo()
	userID := uuid.New()
	refID := uuid.New()
	subj := "Test notification"
	days := 30

	notifRepo.logs = []*models.NotificationLog{
		{
			ID:               uuid.New(),
			UserID:           userID,
			NotificationType: string(models.NotifCategoryCredentialMedical),
			ReferenceID:      &refID,
			DaysBeforeExpiry: &days,
			Subject:          &subj,
			SentAt:           time.Now(),
		},
	}

	svc := newTestNotifService(notifRepo)
	ctx := context.Background()

	history, total, err := svc.GetNotificationHistory(ctx, userID, 20, 0)
	if err != nil {
		t.Fatalf("GetNotificationHistory() error = %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(history) != 1 {
		t.Errorf("history length = %d, want 1", len(history))
	}
	if history[0].NotificationType != string(models.NotifCategoryCredentialMedical) {
		t.Errorf("NotificationType = %s, want %s", history[0].NotificationType, models.NotifCategoryCredentialMedical)
	}
}

func TestGetCheckInterval(t *testing.T) {
	// Default
	d := service.GetCheckInterval()
	if d != 1*time.Hour {
		t.Errorf("default interval = %v, want 1h", d)
	}

	// Custom
	t.Setenv("NOTIFICATION_CHECK_INTERVAL", "30m")
	d = service.GetCheckInterval()
	if d != 30*time.Minute {
		t.Errorf("custom interval = %v, want 30m", d)
	}
}
