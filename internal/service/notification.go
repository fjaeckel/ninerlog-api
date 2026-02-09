package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/fjaeckel/pilotlog-api/internal/repository"
	"github.com/fjaeckel/pilotlog-api/pkg/email"
	"github.com/google/uuid"
)

type NotificationService struct {
	notifRepo      repository.NotificationRepository
	credentialRepo repository.CredentialRepository
	flightRepo     repository.FlightRepository
	licenseRepo    repository.LicenseRepository
	userRepo       repository.UserRepository
	emailSender    *email.Sender
}

func NewNotificationService(
	notifRepo repository.NotificationRepository,
	credentialRepo repository.CredentialRepository,
	flightRepo repository.FlightRepository,
	licenseRepo repository.LicenseRepository,
	userRepo repository.UserRepository,
	emailSender *email.Sender,
) *NotificationService {
	return &NotificationService{
		notifRepo:      notifRepo,
		credentialRepo: credentialRepo,
		flightRepo:     flightRepo,
		licenseRepo:    licenseRepo,
		userRepo:       userRepo,
		emailSender:    emailSender,
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

		// Check credential expiry warnings
		if prefs.CredentialWarnings {
			s.checkCredentialExpiry(ctx, prefs, user.Email, user.Name)
		}

		// Check currency warnings
		if prefs.CurrencyWarnings {
			s.checkCurrencyExpiry(ctx, prefs, user.Email, user.Name)
		}
	}
}

func (s *NotificationService) checkCredentialExpiry(ctx context.Context, prefs *models.NotificationPreferences, userEmail, userName string) {
	credentials, err := s.credentialRepo.GetByUserID(ctx, prefs.UserID)
	if err != nil {
		return
	}

	now := time.Now()
	for _, cred := range credentials {
		if cred.ExpiryDate == nil {
			continue
		}

		daysUntilExpiry := int(cred.ExpiryDate.Sub(now).Hours() / 24)

		for _, warningDay := range prefs.WarningDays {
			wd := int(warningDay)
			if daysUntilExpiry <= wd && daysUntilExpiry >= 0 {
				// Check if already sent
				sent, err := s.notifRepo.HasBeenSent(ctx, prefs.UserID, "credential_warning", cred.ID, wd)
				if err != nil || sent {
					continue
				}

				subject := fmt.Sprintf("PilotLog: %s expires in %d days", cred.CredentialType, daysUntilExpiry)
				body := fmt.Sprintf(`<h2>Credential Expiry Warning</h2>
					<p>Hi %s,</p>
					<p>Your <strong>%s</strong> expires on <strong>%s</strong> (%d days from now).</p>
					<p>Please renew it before it expires to maintain compliance.</p>
					<p>— PilotLog</p>`,
					userName, cred.CredentialType, cred.ExpiryDate.Format("02 Jan 2006"), daysUntilExpiry)

				if err := s.emailSender.Send(userEmail, subject, body); err != nil {
					log.Printf("🔔 Failed to send credential warning email: %v", err)
					continue
				}

				// Log that we sent it
				_ = s.notifRepo.LogNotification(ctx, &models.NotificationLog{
					UserID:           prefs.UserID,
					NotificationType: "credential_warning",
					ReferenceID:      &cred.ID,
					ReferenceType:    strPtr("credential"),
					DaysBeforeExpiry: &wd,
				})
				break // Only send one warning per credential per check
			}
		}
	}
}

func (s *NotificationService) checkCurrencyExpiry(ctx context.Context, prefs *models.NotificationPreferences, userEmail, userName string) {
	licenses, err := s.licenseRepo.GetByUserID(ctx, prefs.UserID)
	if err != nil {
		return
	}

	now := time.Now()
	since90Days := now.AddDate(0, 0, -90)

	for _, lic := range licenses {
		if !lic.IsActive {
			continue
		}

		// Check currency data
		data, err := s.flightRepo.GetCurrencyData(ctx, lic.ID, since90Days)
		if err != nil {
			continue
		}

		// If already not current, warn about it
		requiredDay := 3
		if data.DayLandings < requiredDay {
			remaining := requiredDay - data.DayLandings
			for _, warningDay := range prefs.WarningDays {
				wd := int(warningDay)
				sent, err := s.notifRepo.HasBeenSent(ctx, prefs.UserID, "currency_warning", lic.ID, wd)
				if err != nil || sent {
					continue
				}

				subject := fmt.Sprintf("PilotLog: %s currency — %d more landings needed", lic.LicenseType, remaining)
				body := fmt.Sprintf(`<h2>Currency Warning</h2>
					<p>Hi %s,</p>
					<p>Your <strong>%s</strong> license currency requires attention.</p>
					<p>You have <strong>%d day landings</strong> in the last 90 days. You need <strong>%d more</strong> to maintain currency.</p>
					<p>Log a flight with landings to stay current!</p>
					<p>— PilotLog</p>`,
					userName, lic.LicenseType, data.DayLandings, remaining)

				if err := s.emailSender.Send(userEmail, subject, body); err != nil {
					log.Printf("🔔 Failed to send currency warning email: %v", err)
					continue
				}

				_ = s.notifRepo.LogNotification(ctx, &models.NotificationLog{
					UserID:           prefs.UserID,
					NotificationType: "currency_warning",
					ReferenceID:      &lic.ID,
					ReferenceType:    strPtr("license"),
					DaysBeforeExpiry: &wd,
				})
				break // One warning per license per check
			}
		}
	}
}

func strPtr(s string) *string {
	return &s
}
