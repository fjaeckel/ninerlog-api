package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service/currency"
	"github.com/fjaeckel/ninerlog-api/pkg/email"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// customCurrencyLister lists a user's custom currency rules with their
// current evaluation. Satisfied by *currency.CustomService.
type customCurrencyLister interface {
	List(ctx context.Context, userID uuid.UUID) ([]currency.CustomRuleWithStatus, error)
}

// aircraftReminderLister lists a user's aircraft reminders across aircraft.
// Satisfied by *AircraftReminderService.
type aircraftReminderLister interface {
	ListAll(ctx context.Context, userID uuid.UUID, dueWithinDays *int) ([]*models.AircraftReminder, error)
}

type NotificationService struct {
	notifRepo       repository.NotificationRepository
	credentialRepo  repository.CredentialRepository
	flightRepo      repository.FlightRepository
	licenseRepo     repository.LicenseRepository
	userRepo        repository.UserRepository
	emailSender     *email.Sender
	currencyService *currency.Service
	customService   customCurrencyLister
	now             func() time.Time
	reminders       aircraftReminderLister
}

func NewNotificationService(
	notifRepo repository.NotificationRepository,
	credentialRepo repository.CredentialRepository,
	flightRepo repository.FlightRepository,
	licenseRepo repository.LicenseRepository,
	userRepo repository.UserRepository,
	emailSender *email.Sender,
	currencyService *currency.Service,
	customService customCurrencyLister,
) *NotificationService {
	return &NotificationService{
		notifRepo:       notifRepo,
		credentialRepo:  credentialRepo,
		flightRepo:      flightRepo,
		licenseRepo:     licenseRepo,
		userRepo:        userRepo,
		emailSender:     emailSender,
		currencyService: currencyService,
		customService:   customService,
		now:             time.Now,
	}
}

// SetAircraftReminderSource wires the aircraft reminder lister; nil turns
// aircraft reminder notices off.
func (s *NotificationService) SetAircraftReminderSource(r aircraftReminderLister) {
	s.reminders = r
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
	slog.Info("Triggered notification check (bypassing check-hour)...")
	s.runChecks(ctx, -1)
}

// defaultCheckInterval is the notification check interval when
// NOTIFICATION_CHECK_INTERVAL is unset or invalid.
const defaultCheckInterval = time.Hour

// GetCheckInterval reads NOTIFICATION_CHECK_INTERVAL; an unparseable or
// non-positive value logs a warning and returns the 1h default.
func GetCheckInterval() time.Duration {
	raw := os.Getenv("NOTIFICATION_CHECK_INTERVAL")
	if raw == "" {
		return defaultCheckInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		slog.Warn("Ignoring invalid environment value, using default",
			"key", "NOTIFICATION_CHECK_INTERVAL", "value", raw, "default", defaultCheckInterval.String())
		return defaultCheckInterval
	}
	return d
}

// StartBackgroundChecker starts a goroutine that checks for notifications
// periodically; a non-positive interval falls back to the 1h default.
func (s *NotificationService) StartBackgroundChecker(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		slog.Warn("Invalid notification check interval, using default",
			"interval", interval.String(), "default", defaultCheckInterval.String())
		interval = defaultCheckInterval
	}
	go func() {
		slog.Info("Notification checker started", "interval", interval.String())
		s.checkAndSendNotifications(ctx)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.Info("Notification checker stopped")
				return
			case <-ticker.C:
				s.checkAndSendNotifications(ctx)
			}
		}
	}()
}

func (s *NotificationService) checkAndSendNotifications(ctx context.Context) {
	slog.Info("Checking for notification triggers...")
	start := time.Now()
	NotificationCheckRunsTotal.Inc()
	defer func() {
		NotificationCheckDurationSeconds.Observe(time.Since(start).Seconds())
	}()

	if s.runChecks(ctx, time.Now().UTC().Hour()) {
		NotificationLastSuccessTimestampSeconds.SetToCurrentTime()
	}
}

// runChecks checks every user with email enabled whose check hour is
// checkHour, or every such user when checkHour is negative. It returns false
// when preferences could not be loaded.
func (s *NotificationService) runChecks(ctx context.Context, checkHour int) bool {
	allPrefs, err := s.notifRepo.GetAllUsersWithPreferences(ctx)
	if err != nil {
		slog.Error("Error loading preferences", "error", err)
		NotificationCheckErrorsTotal.Inc()
		return false
	}

	for _, prefs := range allPrefs {
		if !prefs.EmailEnabled {
			continue
		}
		if checkHour >= 0 && prefs.CheckHour != checkHour {
			continue
		}

		user, err := s.userRepo.GetByID(ctx, prefs.UserID)
		if err != nil {
			continue
		}

		s.checkCredentialExpiry(ctx, prefs, user)
		s.checkCurrencyNotifications(ctx, prefs, user)
		s.checkCustomCurrencyNotifications(ctx, prefs, user)
		s.checkAircraftReminders(ctx, prefs, user)
	}
	return true
}

// notice is one notification keyed by (category, refID, days, date).
type notice struct {
	category models.NotificationCategory
	refID    uuid.UUID
	refType  string
	// days is the warning threshold; 0 for a lapse notice.
	days int
	// date is the date the notice concerns.
	date    time.Time
	subject string
	body    string
	// metric is the notifications_sent_total type label.
	metric string
}

// logRow builds the notification_log row for n.
func (n notice) logRow(userID uuid.UUID, skipped bool) *models.NotificationLog {
	refID, days, date, refType, subject := n.refID, n.days, n.date, n.refType, n.subject
	return &models.NotificationLog{
		UserID:              userID,
		NotificationType:    string(n.category),
		ReferenceID:         &refID,
		ReferenceType:       &refType,
		DaysBeforeExpiry:    &days,
		ExpiryReferenceDate: &date,
		Subject:             &subject,
		Skipped:             skipped,
	}
}

// deliver claims n and sends it; a failed send releases the claim. lapse
// selects ClaimLapseNotification. A claim already held sends nothing.
func (s *NotificationService) deliver(ctx context.Context, user *models.User, n notice, lapse bool) error {
	row := n.logRow(user.ID, false)
	claim := s.notifRepo.ClaimNotification
	if lapse {
		claim = s.notifRepo.ClaimLapseNotification
	}
	claimed, err := claim(ctx, row)
	if err != nil {
		return fmt.Errorf("claim %s notification: %w", n.category, err)
	}
	if !claimed {
		return nil
	}
	if err := s.emailSender.SendMessage(ctx, email.Message{
		To: user.Email, Subject: n.subject, HTMLBody: n.body, Type: email.TypeNotification,
	}); err != nil {
		sendErr := fmt.Errorf("send %s notification: %w", n.category, err)
		if relErr := s.notifRepo.ReleaseNotification(ctx, row.ID); relErr != nil {
			return errors.Join(sendErr, fmt.Errorf("release %s notification claim: %w", n.category, relErr))
		}
		return sendErr
	}
	NotificationsSentTotal.WithLabelValues(n.metric).Inc()
	return nil
}

// sendWarning sends n for the nearest warning threshold daysUntil has
// crossed, and records every farther crossed threshold as skipped. n.date is
// the expiry date.
func (s *NotificationService) sendWarning(ctx context.Context, prefs *models.NotificationPreferences, user *models.User, n notice, daysUntil int) error {
	nearest, farther, ok := crossedThresholds(prefs.WarningDays, daysUntil)
	if !ok {
		return nil
	}
	for _, wd := range farther {
		skip := n
		skip.days = wd
		if _, err := s.notifRepo.ClaimNotification(ctx, skip.logRow(user.ID, true)); err != nil {
			return fmt.Errorf("record skipped %s threshold: %w", n.category, err)
		}
	}
	n.days = nearest
	return s.deliver(ctx, user, n, false)
}

// sendLapse sends n as a lapse notice for the first non-current day
// lapsedOn, or for models.NeverCurrentDate when lapsedOn is nil.
func (s *NotificationService) sendLapse(ctx context.Context, user *models.User, n notice, lapsedOn *time.Time) error {
	n.days = 0
	n.date = models.NeverCurrentDate
	if lapsedOn != nil {
		n.date = models.CalendarDate(*lapsedOn)
	}
	return s.deliver(ctx, user, n, true)
}

// crossedThresholds returns the smallest warning threshold at or above
// daysUntil and the larger ones, ascending; ok is false when daysUntil is
// negative or above every threshold.
func crossedThresholds(warningDays pq.Int64Array, daysUntil int) (nearest int, farther []int, ok bool) {
	if daysUntil < 0 {
		return 0, nil, false
	}
	var crossed []int
	for _, w := range warningDays {
		wd := int(w)
		if wd >= daysUntil && !slices.Contains(crossed, wd) {
			crossed = append(crossed, wd)
		}
	}
	if len(crossed) == 0 {
		return 0, nil, false
	}
	slices.Sort(crossed)
	return crossed[0], crossed[1:], true
}

// logNotifyError logs a notification that was not delivered.
func logNotifyError(user *models.User, err error) {
	if err != nil {
		slog.Error("Notification not delivered", "user_id", user.ID, "error", err)
	}
}

func (s *NotificationService) checkCredentialExpiry(ctx context.Context, prefs *models.NotificationPreferences, user *models.User) {
	credentials, err := s.credentialRepo.GetByUserID(ctx, prefs.UserID, nil)
	if err != nil {
		slog.Error("Error loading credentials for notifications", "user_id", prefs.UserID, "error", err)
		return
	}
	tmpl := email.Templates(user.PreferredLocale)

	now := s.now()
	for _, cred := range credentials {
		if cred.ExpiryDate == nil {
			continue
		}
		category := models.CredentialCategoryForType(cred.CredentialType)
		if !prefs.IsCategoryEnabled(category) {
			continue
		}

		expiryDate := models.CalendarDate(*cred.ExpiryDate)
		daysUntilExpiry := models.DaysUntilExpiry(expiryDate, now)
		subject, body := tmpl.CredentialExpiry(email.CredentialExpiryParams{
			UserName:       user.Name,
			CredentialType: string(cred.CredentialType),
			ExpiryDate:     formatDateForUser(expiryDate, user.DateFormat),
			DaysRemaining:  daysUntilExpiry,
		})
		logNotifyError(user, s.sendWarning(ctx, prefs, user, notice{
			category: category, refID: cred.ID, refType: "credential", date: expiryDate,
			subject: subject, body: body, metric: "credential_expiry",
		}, daysUntilExpiry))
	}
}

func (s *NotificationService) checkCurrencyNotifications(ctx context.Context, prefs *models.NotificationPreferences, user *models.User) {
	if s.currencyService == nil {
		return
	}

	currencyStatus, err := s.currencyService.EvaluateAll(currency.WithLapseDates(ctx), prefs.UserID)
	if err != nil {
		slog.Error("Error evaluating currency for user", "user_id", prefs.UserID, "error", err)
		return
	}

	for _, rating := range currencyStatus.Ratings {
		s.checkRatingCurrency(ctx, prefs, rating, user)
	}
	for _, pax := range currencyStatus.PassengerCurrency {
		s.checkPassengerCurrency(ctx, prefs, pax, user)
	}
	if currencyStatus.FlightReview != nil {
		s.checkFlightReviewNotification(ctx, prefs, *currencyStatus.FlightReview, user)
	}

	for _, p := range currencyStatus.Privileges {
		s.checkPrivilegeExpiry(ctx, prefs, p, user)
	}
}

// checkPrivilegeExpiry sends the rating expiry warning for a licence
// privilege with an expiry date.
func (s *NotificationService) checkPrivilegeExpiry(ctx context.Context, prefs *models.NotificationPreferences, p currency.PrivilegeCurrency, user *models.User) {
	if p.ExpiresOn == nil || !prefs.IsCategoryEnabled(models.NotifCategoryRatingExpiry) {
		return
	}
	name := string(p.Kind)
	if p.Detail != nil {
		name += " (" + *p.Detail + ")"
	}
	expiry := models.CalendarDate(*p.ExpiresOn)
	days := models.DaysUntilExpiry(expiry, s.now())
	subject, body := email.Templates(user.PreferredLocale).RatingExpiry(email.RatingExpiryParams{
		UserName:      user.Name,
		LicenseType:   p.LicenseType,
		ClassType:     name,
		ExpiryDate:    formatDateForUser(expiry, user.DateFormat),
		DaysRemaining: days,
	})
	logNotifyError(user, s.sendWarning(ctx, prefs, user, notice{
		category: models.NotifCategoryRatingExpiry, refID: p.PrivilegeID, refType: "licence_privilege", date: expiry,
		subject: subject, body: body, metric: string(models.NotifCategoryRatingExpiry),
	}, days))
}

func (s *NotificationService) checkRatingCurrency(ctx context.Context, prefs *models.NotificationPreferences, rating currency.ClassRatingCurrency, user *models.User) {
	if rating.Status == currency.StatusCurrent {
		return
	}
	tmpl := email.Templates(user.PreferredLocale)

	var expiry *time.Time
	if rating.ExpiryDate != nil {
		if t, err := time.Parse("2006-01-02", *rating.ExpiryDate); err == nil {
			expiry = &t
		}
	}

	if expiry != nil && prefs.IsCategoryEnabled(models.NotifCategoryRatingExpiry) {
		daysUntilExpiry := models.DaysUntilExpiry(*expiry, s.now())
		subject, body := tmpl.RatingExpiry(email.RatingExpiryParams{
			UserName:      user.Name,
			LicenseType:   rating.LicenseType,
			ClassType:     string(rating.ClassType),
			ExpiryDate:    formatDateForUser(*expiry, user.DateFormat),
			DaysRemaining: daysUntilExpiry,
		})
		logNotifyError(user, s.sendWarning(ctx, prefs, user, notice{
			category: models.NotifCategoryRatingExpiry, refID: rating.ClassRatingID, refType: "rating", date: *expiry,
			subject: subject, body: body, metric: string(models.NotifCategoryRatingExpiry),
		}, daysUntilExpiry))
	}

	if ratingRequirementsNotMet(rating.Status) && prefs.IsCategoryEnabled(models.NotifCategoryCurrencyRevalidation) {
		subject, body := tmpl.Revalidation(email.RevalidationParams{
			UserName:      user.Name,
			LicenseType:   rating.LicenseType,
			ClassType:     string(rating.ClassType),
			MessageKey:    rating.MessageKey,
			MessageParams: emailParams(rating.MessageParams),
		})
		n := notice{
			category: models.NotifCategoryCurrencyRevalidation, refID: rating.ClassRatingID, refType: "rating",
			subject: subject, body: body, metric: "revalidation",
		}
		var err error
		if rating.LapsedOn == nil && expiry != nil {
			n.date = *expiry
			err = s.deliver(ctx, user, n, false)
		} else {
			err = s.sendLapse(ctx, user, n, rating.LapsedOn)
		}
		logNotifyError(user, err)
	}
}

// ratingRequirementsNotMet reports whether a rating status calls for the
// revalidation/recency notice: expiring, expired or lapsed.
func ratingRequirementsNotMet(status currency.Status) bool {
	switch status {
	case currency.StatusExpiring, currency.StatusExpired, currency.StatusLapsed:
		return true
	default:
		return false
	}
}

func (s *NotificationService) checkPassengerCurrency(ctx context.Context, prefs *models.NotificationPreferences, pax currency.PassengerCurrency, user *models.User) {
	if pax.LapseUnknown {
		slog.Error("Passenger currency lapse dates unavailable, skipping", "user_id", user.ID, "class", pax.ClassType)
		return
	}
	tmpl := email.Templates(user.PreferredLocale)

	if pax.DayStatus == currency.StatusExpired && pax.MessageKey != currency.MsgPaxULAuthorisationMissing && prefs.IsCategoryEnabled(models.NotifCategoryCurrencyPassenger) {
		subject, body := tmpl.PassengerCurrency(email.PassengerCurrencyParams{
			UserName:  user.Name,
			ClassType: string(pax.ClassType),
			Landings:  pax.DayLandings,
			Required:  pax.DayRequired,
			Remaining: pax.DayRequired - pax.DayLandings,
			Period:    "day",
		})
		logNotifyError(user, s.sendLapse(ctx, user, notice{
			category: models.NotifCategoryCurrencyPassenger,
			refID:    uuidFromString(string(pax.ClassType) + ":" + pax.RegulatoryAuthority + ":day"),
			refType:  "passenger_currency", subject: subject, body: body, metric: "passenger_currency",
		}, pax.DayLapsedOn))
	}

	if pax.NightPrivilege && pax.NightStatus == currency.StatusExpired && prefs.IsCategoryEnabled(models.NotifCategoryCurrencyNight) {
		subject, body := tmpl.PassengerCurrency(email.PassengerCurrencyParams{
			UserName:  user.Name,
			ClassType: string(pax.ClassType),
			Landings:  pax.NightLandings,
			Required:  pax.NightRequired,
			Remaining: pax.NightRequired - pax.NightLandings,
			Period:    "night",
		})
		logNotifyError(user, s.sendLapse(ctx, user, notice{
			category: models.NotifCategoryCurrencyNight,
			refID:    uuidFromString(string(pax.ClassType) + ":" + pax.RegulatoryAuthority + ":night"),
			refType:  "night_currency", subject: subject, body: body, metric: "night_currency",
		}, pax.NightLapsedOn))
	}
}

func (s *NotificationService) checkFlightReviewNotification(ctx context.Context, prefs *models.NotificationPreferences, fr currency.FlightReviewStatus, user *models.User) {
	if fr.Status == currency.StatusCurrent {
		return
	}
	if !prefs.IsCategoryEnabled(models.NotifCategoryCurrencyFlightReview) {
		return
	}

	refID := uuidFromString("flight_review:" + prefs.UserID.String())
	tmpl := email.Templates(user.PreferredLocale)

	if fr.ExpiresOn != nil {
		expiryTime, err := time.Parse("2006-01-02", *fr.ExpiresOn)
		if err == nil {
			daysUntilExpiry := models.DaysUntilExpiry(expiryTime, s.now())
			subject, body := tmpl.FlightReviewExpiry(email.FlightReviewExpiryParams{
				UserName:      user.Name,
				ExpiryDate:    formatDateForUser(expiryTime, user.DateFormat),
				DaysRemaining: daysUntilExpiry,
			})
			logNotifyError(user, s.sendWarning(ctx, prefs, user, notice{
				category: models.NotifCategoryCurrencyFlightReview, refID: refID, refType: "flight_review", date: expiryTime,
				subject: subject, body: body, metric: string(models.NotifCategoryCurrencyFlightReview),
			}, daysUntilExpiry))
			return
		}
	}

	subject, body := tmpl.FlightReviewRequired(email.FlightReviewRequiredParams{
		UserName:      user.Name,
		MessageKey:    fr.MessageKey,
		MessageParams: emailParams(fr.MessageParams),
	})
	logNotifyError(user, s.sendLapse(ctx, user, notice{
		category: models.NotifCategoryCurrencyFlightReview, refID: refID, refType: "flight_review",
		subject: subject, body: body, metric: "flight_review",
	}, nil))
}

// checkCustomCurrencyNotifications emails for user-authored rules the user has
// opted into (rule.Notify), when a rule is expiring (lead-time warnings) or has
// lapsed. Paused rules are skipped (they carry an unknown status and are not
// evaluated). Opt-in is per rule; the only global gate is EmailEnabled.
func (s *NotificationService) checkCustomCurrencyNotifications(ctx context.Context, prefs *models.NotificationPreferences, user *models.User) {
	if s.customService == nil {
		return
	}
	rules, err := s.customService.List(currency.WithLapseDates(ctx), prefs.UserID)
	if err != nil {
		slog.Error("Error evaluating custom currency rules for notifications", "user_id", prefs.UserID, "error", err)
		return
	}
	tmpl := email.Templates(user.PreferredLocale)

	for _, item := range rules {
		rule := item.Rule
		if !rule.Enabled || !rule.Notify {
			continue
		}
		switch item.Evaluation.Status {
		case currency.StatusExpiring:
			if item.Evaluation.ExpiresOn == nil {
				continue
			}
			expiry, err := time.Parse("2006-01-02", *item.Evaluation.ExpiresOn)
			if err != nil {
				continue
			}
			subject, body := tmpl.CustomCurrency(email.CustomCurrencyParams{
				UserName:  user.Name,
				RuleName:  rule.Name,
				Expiring:  true,
				ExpiresOn: formatDateForUser(expiry, user.DateFormat),
			})
			logNotifyError(user, s.sendWarning(ctx, prefs, user, notice{
				category: models.NotifCategoryCustomCurrency, refID: rule.ID, refType: "custom_currency_rule", date: expiry,
				subject: subject, body: body, metric: string(models.NotifCategoryCustomCurrency),
			}, models.DaysUntilExpiry(expiry, s.now())))

		case currency.StatusExpired:
			subject, body := tmpl.CustomCurrency(email.CustomCurrencyParams{
				UserName: user.Name,
				RuleName: rule.Name,
				Expiring: false,
			})
			logNotifyError(user, s.sendLapse(ctx, user, notice{
				category: models.NotifCategoryCustomCurrency, refID: rule.ID, refType: "custom_currency_rule",
				subject: subject, body: body, metric: string(models.NotifCategoryCustomCurrency),
			}, item.Evaluation.LapsedOn))
		}
	}
}

// checkAircraftReminders emails for aircraft reminders that fall within a
// warning day (one notice per threshold per due date) or are overdue (one
// notice per due date).
func (s *NotificationService) checkAircraftReminders(ctx context.Context, prefs *models.NotificationPreferences, user *models.User) {
	if s.reminders == nil || !prefs.IsCategoryEnabled(models.NotifCategoryAircraftReminder) {
		return
	}
	reminders, err := s.reminders.ListAll(ctx, prefs.UserID, nil)
	if err != nil || len(reminders) == 0 {
		return
	}
	tmpl := email.Templates(user.PreferredLocale)
	today := models.CalendarDate(s.now())
	category := models.NotifCategoryAircraftReminder

	for _, rem := range reminders {
		due := rem.DueDate
		days := rem.DaysUntilDue(today)
		label := ""
		if rem.Label != nil {
			label = *rem.Label
		}
		subject, body := tmpl.AircraftReminder(email.AircraftReminderParams{
			UserName:      user.Name,
			Registration:  rem.AircraftRegistration,
			Kind:          string(rem.Kind),
			Label:         label,
			DueDate:       formatDateForUser(due, user.DateFormat),
			DaysRemaining: days,
			Overdue:       days < 0,
		})
		n := notice{
			category: category, refID: rem.ID, refType: "aircraft_reminder", date: due,
			subject: subject, body: body, metric: string(category),
		}
		if days >= 0 {
			logNotifyError(user, s.sendWarning(ctx, prefs, user, n, days))
			continue
		}
		logNotifyError(user, s.sendLapse(ctx, user, n, &due))
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

// emailParams adapts currency message params for the email templates.
func emailParams(p *currency.MessageParams) email.CurrencyMessageParams {
	if p == nil {
		return email.CurrencyMessageParams{}
	}
	return email.CurrencyMessageParams{Days: p.Days, Needed: p.Needed, Date: p.Date}
}
