package service

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service/currency"
	"github.com/fjaeckel/ninerlog-api/pkg/email"
	"github.com/google/uuid"
)

type NotificationService struct {
	notifRepo       repository.NotificationRepository
	credentialRepo  repository.CredentialRepository
	flightRepo      repository.FlightRepository
	licenseRepo     repository.LicenseRepository
	userRepo        repository.UserRepository
	emailSender     *email.Sender
	currencyService *currency.Service
}

func NewNotificationService(
	notifRepo repository.NotificationRepository,
	credentialRepo repository.CredentialRepository,
	flightRepo repository.FlightRepository,
	licenseRepo repository.LicenseRepository,
	userRepo repository.UserRepository,
	emailSender *email.Sender,
	currencyService *currency.Service,
) *NotificationService {
	return &NotificationService{
		notifRepo:       notifRepo,
		credentialRepo:  credentialRepo,
		flightRepo:      flightRepo,
		licenseRepo:     licenseRepo,
		userRepo:        userRepo,
		emailSender:     emailSender,
		currencyService: currencyService,
	}
}

// GetPreferences returns notification preferences for a user
func (s *NotificationService) GetPreferences(ctx context.Context, userID uuid.UUID) (*models.NotificationPreferences, error) {
	return s.notifRepo.GetPreferences(ctx, userID)
}

// UpdatePreferences updates notification preferences for a user
func (s *NotificationService) UpdatePreferences(ctx context.Context, prefs *models.NotificationPreferences) error {
	return s.notifRepo.UpsertPreferences(ctx, prefs)
}

// GetNotificationHistory returns paginated notification history for a user
func (s *NotificationService) GetNotificationHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*models.NotificationLog, int, error) {
	return s.notifRepo.GetNotificationHistory(ctx, userID, limit, offset)
}

// TriggerCheck runs the notification check immediately for all users,
// bypassing the check-hour gating. Used by admin maintenance endpoint and E2E tests.
func (s *NotificationService) TriggerCheck(ctx context.Context) {
	log.Println("🔔 Triggered notification check (bypassing check-hour)...")

	allPrefs, err := s.notifRepo.GetAllUsersWithPreferences(ctx)
	if err != nil {
		log.Printf("🔔 Error loading preferences: %v", err)
		return
	}

	for _, prefs := range allPrefs {
		if !prefs.EmailEnabled {
			continue
		}

		user, err := s.userRepo.GetByID(ctx, prefs.UserID)
		if err != nil {
			continue
		}

		s.checkCredentialExpiry(ctx, prefs, user.Email, user.Name)
		s.checkCurrencyNotifications(ctx, prefs, user.Email, user.Name)
	}
}

// GetCheckInterval reads NOTIFICATION_CHECK_INTERVAL env var, defaults to 1h
func GetCheckInterval() time.Duration {
	if val := os.Getenv("NOTIFICATION_CHECK_INTERVAL"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return 1 * time.Hour
}

// StartBackgroundChecker starts a goroutine that checks for notifications periodically
func (s *NotificationService) StartBackgroundChecker(ctx context.Context, interval time.Duration) {
	go func() {
		log.Printf("🔔 Notification checker started (interval: %s)", interval)
		// Run immediately on start
		s.checkAndSendNotifications(ctx)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Println("🔔 Notification checker stopped")
				return
			case <-ticker.C:
				s.checkAndSendNotifications(ctx)
			}
		}
	}()
}

func (s *NotificationService) checkAndSendNotifications(ctx context.Context) {
	log.Println("🔔 Checking for notification triggers...")
	start := time.Now()
	NotificationCheckRunsTotal.Inc()
	defer func() {
		NotificationCheckDurationSeconds.Observe(time.Since(start).Seconds())
	}()

	currentHour := time.Now().UTC().Hour()

	allPrefs, err := s.notifRepo.GetAllUsersWithPreferences(ctx)
	if err != nil {
		log.Printf("🔔 Error loading preferences: %v", err)
		return
	}

	for _, prefs := range allPrefs {
		if !prefs.EmailEnabled {
			continue
		}

		// Only process users whose check_hour matches current UTC hour
		if prefs.CheckHour != currentHour {
			continue
		}

		user, err := s.userRepo.GetByID(ctx, prefs.UserID)
		if err != nil {
			continue
		}

		// Check credential expiry warnings (per-category)
		s.checkCredentialExpiry(ctx, prefs, user.Email, user.Name)

		// Check currency/rating warnings using the two-tier currency system
		s.checkCurrencyNotifications(ctx, prefs, user.Email, user.Name)
	}
}

func (s *NotificationService) checkCredentialExpiry(ctx context.Context, prefs *models.NotificationPreferences, userEmail, userName string) {
	credentials, err := s.credentialRepo.GetByUserID(ctx, prefs.UserID)
	if err != nil {
		return
	}

	user, err := s.userRepo.GetByID(ctx, prefs.UserID)
	if err != nil {
		return
	}
	tmpl := email.Templates(user.PreferredLocale)

	now := time.Now()
	for _, cred := range credentials {
		if cred.ExpiryDate == nil {
			continue
		}

		// Map credential type to notification category
		category := models.CredentialCategoryForType(cred.CredentialType)
		if !prefs.IsCategoryEnabled(category) {
			continue
		}

		daysUntilExpiry := int(cred.ExpiryDate.Sub(now).Hours() / 24)
		expiryDate := *cred.ExpiryDate

		for _, warningDay := range prefs.WarningDays {
			wd := int(warningDay)
			if daysUntilExpiry <= wd && daysUntilExpiry >= 0 {
				// Cycle-aware dedup: include the expiry date so renewals trigger fresh warnings
				sent, err := s.notifRepo.HasBeenSent(ctx, prefs.UserID, string(category), cred.ID, wd, &expiryDate)
				if err != nil || sent {
					continue
				}

				subject, body := tmpl.CredentialExpiry(email.CredentialExpiryParams{
					UserName:       userName,
					CredentialType: string(cred.CredentialType),
					ExpiryDate:     formatDateForUser(expiryDate, user.DateFormat),
					DaysRemaining:  daysUntilExpiry,
				})

				if err := s.emailSender.Send(userEmail, subject, body); err != nil {
					log.Printf("🔔 Failed to send credential warning email: %v", err)
					continue
				}
				NotificationsSentTotal.WithLabelValues("credential_expiry").Inc()

				_ = s.notifRepo.LogNotification(ctx, &models.NotificationLog{
					UserID:              prefs.UserID,
					NotificationType:    string(category),
					ReferenceID:         &cred.ID,
					ReferenceType:       strPtr("credential"),
					DaysBeforeExpiry:    &wd,
					ExpiryReferenceDate: &expiryDate,
					Subject:             &subject,
				})
				break // Only send one warning per credential per check
			}
		}
	}
}

func (s *NotificationService) checkCurrencyNotifications(ctx context.Context, prefs *models.NotificationPreferences, userEmail, userName string) {
	if s.currencyService == nil {
		return
	}

	// Use the two-tier currency service to get current status
	currencyStatus, err := s.currencyService.EvaluateAll(ctx, prefs.UserID)
	if err != nil {
		log.Printf("🔔 Error evaluating currency for user %s: %v", prefs.UserID, err)
		return
	}

	// Check Tier 1: Rating/license currency
	for _, rating := range currencyStatus.Ratings {
		s.checkRatingCurrency(ctx, prefs, rating, userEmail, userName)
	}

	// Check Tier 2: Passenger currency
	for _, pax := range currencyStatus.PassengerCurrency {
		s.checkPassengerCurrency(ctx, prefs, pax, userEmail, userName)
	}

	// Check flight review (FAA)
	if currencyStatus.FlightReview != nil {
		s.checkFlightReviewNotification(ctx, prefs, *currencyStatus.FlightReview, userEmail, userName)
	}
}

func (s *NotificationService) checkRatingCurrency(ctx context.Context, prefs *models.NotificationPreferences, rating currency.ClassRatingCurrency, userEmail, userName string) {
	// Determine which category based on rating status
	if rating.Status == currency.StatusCurrent {
		return // No notification needed
	}

	user, err := s.userRepo.GetByID(ctx, prefs.UserID)
	if err != nil {
		return
	}
	tmpl := email.Templates(user.PreferredLocale)

	// Rating expiry notification
	if rating.ExpiryDate != nil && prefs.IsCategoryEnabled(models.NotifCategoryRatingExpiry) {
		expiryTime, err := time.Parse("2006-01-02", *rating.ExpiryDate)
		if err == nil {
			daysUntilExpiry := int(time.Until(expiryTime).Hours() / 24)
			subject, body := tmpl.RatingExpiry(email.RatingExpiryParams{
				UserName:      userName,
				LicenseType:   rating.LicenseType,
				ClassType:     string(rating.ClassType),
				ExpiryDate:    formatDateForUser(expiryTime, user.DateFormat),
				DaysRemaining: daysUntilExpiry,
			})
			s.sendWarningForDays(ctx, prefs, models.NotifCategoryRatingExpiry, rating.ClassRatingID,
				"rating", daysUntilExpiry, &expiryTime,
				subject, body, userEmail)
		}
	}

	// Revalidation currency notification (EASA — requirements not met approaching expiry)
	if (rating.Status == currency.StatusExpiring || rating.Status == currency.StatusExpired) &&
		prefs.IsCategoryEnabled(models.NotifCategoryCurrencyRevalidation) {
		// Use class rating ID as reference, no specific expiry date for revalidation checks
		sent, err := s.notifRepo.HasBeenSent(ctx, prefs.UserID, string(models.NotifCategoryCurrencyRevalidation), rating.ClassRatingID, 0, nil)
		if err != nil || sent {
			return
		}

		subject, body := tmpl.Revalidation(email.RevalidationParams{
			UserName:    userName,
			LicenseType: rating.LicenseType,
			ClassType:   string(rating.ClassType),
			Message:     rating.Message,
		})

		if err := s.emailSender.Send(userEmail, subject, body); err != nil {
			log.Printf("🔔 Failed to send revalidation warning email: %v", err)
			return
		}
		NotificationsSentTotal.WithLabelValues("revalidation").Inc()

		zero := 0
		_ = s.notifRepo.LogNotification(ctx, &models.NotificationLog{
			UserID:           prefs.UserID,
			NotificationType: string(models.NotifCategoryCurrencyRevalidation),
			ReferenceID:      &rating.ClassRatingID,
			ReferenceType:    strPtr("rating"),
			DaysBeforeExpiry: &zero,
			Subject:          &subject,
		})
	}
}

func (s *NotificationService) checkPassengerCurrency(ctx context.Context, prefs *models.NotificationPreferences, pax currency.PassengerCurrency, userEmail, userName string) {
	user, err := s.userRepo.GetByID(ctx, prefs.UserID)
	if err != nil {
		return
	}
	tmpl := email.Templates(user.PreferredLocale)

	// Day passenger currency
	if pax.DayStatus != currency.StatusCurrent && prefs.IsCategoryEnabled(models.NotifCategoryCurrencyPassenger) {
		// Generate a stable reference ID from class type + authority
		refID := uuidFromString(string(pax.ClassType) + ":" + pax.RegulatoryAuthority + ":day")
		sent, err := s.notifRepo.HasBeenSent(ctx, prefs.UserID, string(models.NotifCategoryCurrencyPassenger), refID, 0, nil)
		if err != nil || sent {
			goto checkNight
		}

		{
			remaining := pax.DayRequired - pax.DayLandings
			subject, body := tmpl.PassengerCurrency(email.PassengerCurrencyParams{
				UserName:  userName,
				ClassType: string(pax.ClassType),
				Landings:  pax.DayLandings,
				Required:  pax.DayRequired,
				Remaining: remaining,
				Period:    "day",
			})

			if err := s.emailSender.Send(userEmail, subject, body); err != nil {
				log.Printf("🔔 Failed to send passenger currency email: %v", err)
			} else {
				NotificationsSentTotal.WithLabelValues("passenger_currency").Inc()
				zero := 0
				_ = s.notifRepo.LogNotification(ctx, &models.NotificationLog{
					UserID:           prefs.UserID,
					NotificationType: string(models.NotifCategoryCurrencyPassenger),
					ReferenceID:      &refID,
					ReferenceType:    strPtr("passenger_currency"),
					DaysBeforeExpiry: &zero,
					Subject:          &subject,
				})
			}
		}
	}

checkNight:
	// Night passenger currency
	if pax.NightPrivilege && pax.NightStatus != currency.StatusCurrent && prefs.IsCategoryEnabled(models.NotifCategoryCurrencyNight) {
		refID := uuidFromString(string(pax.ClassType) + ":" + pax.RegulatoryAuthority + ":night")
		sent, err := s.notifRepo.HasBeenSent(ctx, prefs.UserID, string(models.NotifCategoryCurrencyNight), refID, 0, nil)
		if err != nil || sent {
			return
		}

		remaining := pax.NightRequired - pax.NightLandings
		subject, body := tmpl.PassengerCurrency(email.PassengerCurrencyParams{
			UserName:  userName,
			ClassType: string(pax.ClassType),
			Landings:  pax.NightLandings,
			Required:  pax.NightRequired,
			Remaining: remaining,
			Period:    "night",
		})

		if err := s.emailSender.Send(userEmail, subject, body); err != nil {
			log.Printf("🔔 Failed to send night currency email: %v", err)
			return
		}
		NotificationsSentTotal.WithLabelValues("night_currency").Inc()

		zero := 0
		_ = s.notifRepo.LogNotification(ctx, &models.NotificationLog{
			UserID:           prefs.UserID,
			NotificationType: string(models.NotifCategoryCurrencyNight),
			ReferenceID:      &refID,
			ReferenceType:    strPtr("night_currency"),
			DaysBeforeExpiry: &zero,
			Subject:          &subject,
		})
	}
}

func (s *NotificationService) checkFlightReviewNotification(ctx context.Context, prefs *models.NotificationPreferences, fr currency.FlightReviewStatus, userEmail, userName string) {
	if fr.Status == currency.StatusCurrent {
		return
	}
	if !prefs.IsCategoryEnabled(models.NotifCategoryCurrencyFlightReview) {
		return
	}

	refID := uuidFromString("flight_review:" + prefs.UserID.String())

	user, err := s.userRepo.GetByID(ctx, prefs.UserID)
	if err != nil {
		return
	}
	tmpl := email.Templates(user.PreferredLocale)

	// Parse expiry to get days-based warning
	if fr.ExpiresOn != nil {
		expiryTime, err := time.Parse("2006-01-02", *fr.ExpiresOn)
		if err == nil {
			daysUntilExpiry := int(time.Until(expiryTime).Hours() / 24)
			subject, body := tmpl.FlightReviewExpiry(email.FlightReviewExpiryParams{
				UserName:      userName,
				ExpiryDate:    formatDateForUser(expiryTime, user.DateFormat),
				DaysRemaining: daysUntilExpiry,
			})
			s.sendWarningForDays(ctx, prefs, models.NotifCategoryCurrencyFlightReview, refID,
				"flight_review", daysUntilExpiry, &expiryTime,
				subject, body, userEmail)
			return
		}
	}

	// Expired with no expiry date info — send immediately
	sent, err := s.notifRepo.HasBeenSent(ctx, prefs.UserID, string(models.NotifCategoryCurrencyFlightReview), refID, 0, nil)
	if err != nil || sent {
		return
	}

	subject := "NinerLog: Flight review required"
	body := fmt.Sprintf(`<h2>Flight Review Required</h2>
		<p>Hi %s,</p>
		<p>%s</p>
		<p>Complete a flight review (14 CFR §61.56) to regain flying privileges.</p>
		<p>— NinerLog</p>`, userName, fr.Message)
	// Use locale-aware template if available
	if user != nil {
		tmplFR := email.Templates(user.PreferredLocale)
		subject, body = tmplFR.FlightReviewRequired(email.FlightReviewRequiredParams{
			UserName: userName,
			Message:  fr.Message,
		})
	}

	if err := s.emailSender.Send(userEmail, subject, body); err != nil {
		log.Printf("🔔 Failed to send flight review email: %v", err)
		return
	}
	NotificationsSentTotal.WithLabelValues("flight_review").Inc()

	zero := 0
	_ = s.notifRepo.LogNotification(ctx, &models.NotificationLog{
		UserID:           prefs.UserID,
		NotificationType: string(models.NotifCategoryCurrencyFlightReview),
		ReferenceID:      &refID,
		ReferenceType:    strPtr("flight_review"),
		DaysBeforeExpiry: &zero,
		Subject:          &subject,
	})
}

// sendWarningForDays checks warning day thresholds and sends the first matching notification
func (s *NotificationService) sendWarningForDays(ctx context.Context, prefs *models.NotificationPreferences, category models.NotificationCategory, refID uuid.UUID, refType string, daysUntilExpiry int, expiryDate *time.Time, subject, body, userEmail string) {
	for _, warningDay := range prefs.WarningDays {
		wd := int(warningDay)
		if daysUntilExpiry <= wd && daysUntilExpiry >= 0 {
			sent, err := s.notifRepo.HasBeenSent(ctx, prefs.UserID, string(category), refID, wd, expiryDate)
			if err != nil || sent {
				continue
			}

			if err := s.emailSender.Send(userEmail, subject, body); err != nil {
				log.Printf("🔔 Failed to send %s warning email: %v", category, err)
				continue
			}
			NotificationsSentTotal.WithLabelValues(string(category)).Inc()

			_ = s.notifRepo.LogNotification(ctx, &models.NotificationLog{
				UserID:              prefs.UserID,
				NotificationType:    string(category),
				ReferenceID:         &refID,
				ReferenceType:       strPtr(refType),
				DaysBeforeExpiry:    &wd,
				ExpiryReferenceDate: expiryDate,
				Subject:             &subject,
			})
			break // Only send one warning per item per check
		}
	}
}

func strPtr(s string) *string {
	return &s
}

// uuidFromString generates a deterministic UUID v5 from a string (for stable reference IDs)
func uuidFromString(s string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(s))
}

// parseCheckInterval parses a duration string, used for NOTIFICATION_CHECK_INTERVAL
func parseCheckInterval(s string) (time.Duration, error) {
	// Try as duration first (e.g. "1h", "30m")
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	// Try as minutes integer
	if mins, err := strconv.Atoi(s); err == nil {
		return time.Duration(mins) * time.Minute, nil
	}
	return 0, fmt.Errorf("invalid check interval: %s", s)
}

// formatDateForUser formats a time.Time using the user's preferred date format.
func formatDateForUser(t time.Time, dateFormat string) string {
	switch dateFormat {
	case "DD.MM.YYYY":
		return t.Format("02.01.2006")
	case "MM/DD/YYYY":
		return t.Format("01/02/2006")
	case "YYYY-MM-DD":
		return t.Format("2006-01-02")
	default:
		return t.Format("02.01.2006")
	}
}
