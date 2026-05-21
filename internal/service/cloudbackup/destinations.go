package cloudbackup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup/provider"
	"github.com/google/uuid"
)

// CreateDestinationInput is the service-level input for creating a backup
// destination. Credentials are accepted as a map of strings keyed by the
// provider's CredentialSchema.
type CreateDestinationInput struct {
	UserID             uuid.UUID
	Provider           string
	DisplayName        string
	Config             provider.Config
	Credentials        provider.Credentials
	Schedule           models.BackupSchedule
	ScheduleHourUTC    int
	ScheduleDayOfWeek  *int
	ScheduleDayOfMonth *int
	RetentionCount     int
	Enabled            bool
}

// UpdateDestinationInput is the service-level input for partial updates. All
// pointer fields are optional.
type UpdateDestinationInput struct {
	DisplayName        *string
	Schedule           *models.BackupSchedule
	ScheduleHourUTC    *int
	ScheduleDayOfWeek  *int
	ScheduleDayOfMonth *int
	RetentionCount     *int
	Enabled            *bool
}

// CreateDestination validates the provider/config/credentials combination,
// authenticates against the provider, encrypts the credential blob, and
// persists the destination. The supplied credentials are never written to
// disk in plaintext nor returned in the response.
func (s *Service) CreateDestination(ctx context.Context, in CreateDestinationInput) (*models.BackupDestination, error) {
	p, err := s.Provider(in.Provider)
	if err != nil {
		return nil, err
	}
	if in.DisplayName == "" {
		return nil, fmt.Errorf("displayName is required")
	}
	if err := validateSchedule(in.Schedule, in.ScheduleHourUTC, in.ScheduleDayOfWeek, in.ScheduleDayOfMonth); err != nil {
		return nil, err
	}
	if in.RetentionCount < 0 {
		return nil, fmt.Errorf("retentionCount must be >= 0")
	}

	// Validate config/credential field presence based on the provider's
	// declared schema; gives clearer errors than a remote auth failure.
	if err := requireFields(p.ConfigSchema(), in.Config); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if err := requireFields(p.CredentialSchema(), in.Credentials); err != nil {
		return nil, fmt.Errorf("credentials: %w", err)
	}

	// Verify the credentials actually work before we persist them.
	if err := p.Validate(ctx, in.Config, in.Credentials); err != nil {
		return nil, err
	}

	credsJSON, err := json.Marshal(in.Credentials)
	if err != nil {
		return nil, fmt.Errorf("marshal credentials: %w", err)
	}
	credsEnc, nonce, err := s.crypto.Encrypt(credsJSON)
	if err != nil {
		return nil, fmt.Errorf("encrypt credentials: %w", err)
	}

	hint := credentialHint(p.CredentialSchema(), in.Credentials)

	d := &models.BackupDestination{
		UserID:             in.UserID,
		Provider:           in.Provider,
		DisplayName:        in.DisplayName,
		Config:             cloneConfig(in.Config),
		CredentialHint:     hint,
		CredentialsEnc:     credsEnc,
		CredentialsNonce:   nonce,
		Schedule:           in.Schedule,
		ScheduleHourUTC:    in.ScheduleHourUTC,
		ScheduleDayOfWeek:  in.ScheduleDayOfWeek,
		ScheduleDayOfMonth: in.ScheduleDayOfMonth,
		RetentionCount:     in.RetentionCount,
		Status:             models.BackupStatusActive,
		Enabled:            in.Enabled,
	}

	if err := s.destRepo.Create(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// UpdateDestination applies a partial update. Credentials are intentionally
// not modifiable here — to rotate, delete and recreate the destination.
func (s *Service) UpdateDestination(ctx context.Context, destinationID, userID uuid.UUID, in UpdateDestinationInput) (*models.BackupDestination, error) {
	d, err := s.requireOwned(ctx, destinationID, userID)
	if err != nil {
		return nil, err
	}
	if in.DisplayName != nil {
		if *in.DisplayName == "" {
			return nil, fmt.Errorf("displayName cannot be empty")
		}
		d.DisplayName = *in.DisplayName
	}

	// Schedule + companions are applied atomically; we evaluate the *intended*
	// final shape and validate before persisting.
	sched := d.Schedule
	hour := d.ScheduleHourUTC
	dow := d.ScheduleDayOfWeek
	dom := d.ScheduleDayOfMonth
	if in.Schedule != nil {
		sched = *in.Schedule
	}
	if in.ScheduleHourUTC != nil {
		hour = *in.ScheduleHourUTC
	}
	if in.ScheduleDayOfWeek != nil {
		v := *in.ScheduleDayOfWeek
		dow = &v
	}
	if in.ScheduleDayOfMonth != nil {
		v := *in.ScheduleDayOfMonth
		dom = &v
	}
	if err := validateSchedule(sched, hour, dow, dom); err != nil {
		return nil, err
	}
	d.Schedule = sched
	d.ScheduleHourUTC = hour
	d.ScheduleDayOfWeek = dow
	d.ScheduleDayOfMonth = dom

	if in.RetentionCount != nil {
		if *in.RetentionCount < 0 {
			return nil, fmt.Errorf("retentionCount must be >= 0")
		}
		d.RetentionCount = *in.RetentionCount
	}
	if in.Enabled != nil {
		d.Enabled = *in.Enabled
		// Manually re-enabling a destination that was in error clears the
		// error so the scheduler picks it back up.
		if d.Enabled && d.Status == models.BackupStatusError {
			d.Status = models.BackupStatusActive
			d.LastError = ""
			d.ConsecutiveFailures = 0
		}
	}

	if err := s.destRepo.Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// DeleteDestination removes a destination (and cascades to its runs).
func (s *Service) DeleteDestination(ctx context.Context, destinationID, userID uuid.UUID) error {
	if _, err := s.requireOwned(ctx, destinationID, userID); err != nil {
		return err
	}
	return s.destRepo.Delete(ctx, destinationID)
}

// TestDestination decrypts the stored credentials and re-runs the provider's
// Validate routine. It also updates last_error / status to reflect the result
// so the UI's freshly-displayed status matches reality.
func (s *Service) TestDestination(ctx context.Context, destinationID, userID uuid.UUID) (bool, string, error) {
	d, err := s.requireOwned(ctx, destinationID, userID)
	if err != nil {
		return false, "", err
	}
	p, err := s.Provider(d.Provider)
	if err != nil {
		return false, "", err
	}
	creds, err := s.decryptCredentials(d)
	if err != nil {
		return false, "", err
	}
	cfg := provider.Config(d.Config)
	if err := p.Validate(ctx, cfg, creds); err != nil {
		msg := sanitizeMessage(err)
		d.LastError = msg
		d.Status = models.BackupStatusError
		_ = s.destRepo.Update(ctx, d)
		return false, msg, nil
	}
	// Clear any previously-recorded error.
	if d.LastError != "" || d.Status == models.BackupStatusError {
		d.LastError = ""
		if d.Status == models.BackupStatusError {
			d.Status = models.BackupStatusActive
		}
		d.ConsecutiveFailures = 0
		_ = s.destRepo.Update(ctx, d)
	}
	return true, "", nil
}

// decryptCredentials retrieves and decrypts the stored credentials blob.
func (s *Service) decryptCredentials(d *models.BackupDestination) (provider.Credentials, error) {
	plain, err := s.crypto.Decrypt(d.CredentialsEnc, d.CredentialsNonce)
	if err != nil {
		return nil, fmt.Errorf("decrypt credentials: %w", err)
	}
	var out provider.Credentials
	if err := json.Unmarshal(plain, &out); err != nil {
		return nil, fmt.Errorf("unmarshal credentials: %w", err)
	}
	return out, nil
}

// requireFields verifies the required fields named in `schema` are present
// and non-empty in `data`.
func requireFields(schema []provider.Field, data map[string]any) error {
	for _, f := range schema {
		if !f.Required {
			continue
		}
		v, ok := data[f.Name]
		if !ok {
			return fmt.Errorf("missing required field %q", f.Name)
		}
		switch typed := v.(type) {
		case string:
			if typed == "" {
				return fmt.Errorf("missing required field %q", f.Name)
			}
		case nil:
			return fmt.Errorf("missing required field %q", f.Name)
		}
	}
	return nil
}

// credentialHint produces a short, non-secret label for a credential blob:
// the last 4 visible characters of the first non-sensitive field, or "•••"
// if no such field exists. The hint is what the UI shows for an existing
// destination since the secret itself never leaves the server.
func credentialHint(schema []provider.Field, creds map[string]any) string {
	for _, f := range schema {
		if f.Type == provider.FieldTypePassword || f.Sensitive {
			continue
		}
		if v, ok := creds[f.Name]; ok {
			if s, ok := v.(string); ok && len(s) > 4 {
				return "…" + s[len(s)-4:]
			}
		}
	}
	return "•••"
}

func cloneConfig(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// recordError flips a destination into the error state after enough
// consecutive failures and stores a sanitised message.
func (s *Service) recordError(ctx context.Context, d *models.BackupDestination, err error) {
	d.ConsecutiveFailures++
	d.LastError = sanitizeMessage(err)
	d.LastRunAt = timeOrNil(s.clock())
	if d.ConsecutiveFailures >= 3 {
		d.Status = models.BackupStatusError
	}
	if updateErr := s.destRepo.Update(ctx, d); updateErr != nil {
		// Logging is the responsibility of the caller; we want to surface the
		// original cause to the user, not the persistence hiccup.
		_ = updateErr
	}
}

// recordSuccess updates a destination's status, last-run/last-success
// timestamps, and clears the failure counter.
func (s *Service) recordSuccess(ctx context.Context, d *models.BackupDestination, sha string) {
	now := s.clock()
	d.LastRunAt = &now
	d.LastSuccessAt = &now
	d.LastSuccessSHA256 = sha
	d.ConsecutiveFailures = 0
	d.LastError = ""
	if d.Status == models.BackupStatusError {
		d.Status = models.BackupStatusActive
	}
	if err := s.destRepo.Update(ctx, d); err != nil {
		_ = err
	}
}

// nowWithin returns true if last_success_at is within the window relative to
// now. Used by the runner's skip-if-unchanged short-circuit. Currently
// unused but kept for clarity in tests.
func nowWithin(now time.Time, last *time.Time, window time.Duration) bool {
	if last == nil {
		return false
	}
	return now.Sub(*last) <= window
}

// guardUserRepoErr maps a repository error to the appropriate service error.
func guardUserRepoErr(err error) error {
	if errors.Is(err, repository.ErrNotFound) {
		return ErrDestinationNotFound
	}
	return err
}
