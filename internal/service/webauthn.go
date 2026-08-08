package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

var (
	ErrWebAuthnNotConfigured = errors.New("webauthn is not configured")
	// ErrWebAuthnSessionNotFound is returned for every unusable ceremony
	// handle — expired, already consumed, wrong ceremony, scoped to another
	// user, or never issued. Keeping these indistinguishable denies an
	// attacker any signal about which handles once existed.
	ErrWebAuthnSessionNotFound   = errors.New("webauthn session not found or expired")
	ErrWebAuthnInvalidResponse   = errors.New("invalid webauthn response")
	ErrWebAuthnUnknownCredential = errors.New("unknown webauthn credential")
	ErrWebAuthnVerification      = errors.New("webauthn verification failed")
)

const (
	// DefaultWebAuthnSessionTTL is used when WEBAUTHN_SESSION_TTL is unset.
	DefaultWebAuthnSessionTTL = 5 * time.Minute
	// DefaultWebAuthnMaxOpenCeremonies is used when
	// WEBAUTHN_MAX_OPEN_CEREMONIES is unset. Generous for legitimate use
	// (phone, laptop, tablet, plus retries) while bounding a compromised
	// account to a trivial footprint.
	DefaultWebAuthnMaxOpenCeremonies = 10

	// webauthnHandleBytes is the entropy of the opaque handle that binds the
	// begin and finish halves of a ceremony. For discoverable login the handle
	// is the only thing linking the two requests, so it must be unguessable.
	webauthnHandleBytes = 16
)

// WebAuthnService implements passkey registration & login flows.
type WebAuthnService struct {
	wa                *webauthn.WebAuthn
	credRepo          repository.WebAuthnCredentialRepository
	sessionRepo       repository.WebAuthnSessionRepository
	userRepo          repository.UserRepository
	authService       *AuthService
	sessionTTL        time.Duration
	maxOpenCeremonies int
}

// NewWebAuthnService creates a new WebAuthnService. Returns nil and an error if the
// webauthn library cannot be initialized with the given config (e.g. invalid origins).
//
// sessionTTL bounds how long ceremony state stays usable; non-positive values
// fall back to DefaultWebAuthnSessionTTL. maxOpenCeremonies caps how many
// ceremonies one user may hold open at once; non-positive values fall back to
// DefaultWebAuthnMaxOpenCeremonies.
func NewWebAuthnService(
	rpID, rpName string,
	rpOrigins []string,
	credRepo repository.WebAuthnCredentialRepository,
	sessionRepo repository.WebAuthnSessionRepository,
	userRepo repository.UserRepository,
	authService *AuthService,
	sessionTTL time.Duration,
	maxOpenCeremonies int,
) (*WebAuthnService, error) {
	if rpID == "" || rpName == "" || len(rpOrigins) == 0 {
		return nil, ErrWebAuthnNotConfigured
	}
	if sessionTTL <= 0 {
		sessionTTL = DefaultWebAuthnSessionTTL
	}
	if maxOpenCeremonies <= 0 {
		maxOpenCeremonies = DefaultWebAuthnMaxOpenCeremonies
	}
	// Drive the client-side ceremony timeout from the same value as the row
	// TTL. This keeps the stored challenge from outliving the browser ceremony
	// no matter how the TTL is configured — a challenge that is still valid
	// after the browser has given up is pure attack surface. Enforce also has
	// the library re-check expiry server-side during verification.
	timeouts := webauthn.TimeoutConfig{
		Enforce:    true,
		Timeout:    sessionTTL,
		TimeoutUVD: sessionTTL,
	}
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          rpID,
		RPDisplayName: rpName,
		RPOrigins:     rpOrigins,
		Timeouts: webauthn.TimeoutsConfig{
			Login:        timeouts,
			Registration: timeouts,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("init webauthn: %w", err)
	}
	return &WebAuthnService{
		wa:                wa,
		credRepo:          credRepo,
		sessionRepo:       sessionRepo,
		userRepo:          userRepo,
		authService:       authService,
		sessionTTL:        sessionTTL,
		maxOpenCeremonies: maxOpenCeremonies,
	}, nil
}

// webauthnUser adapts a *models.User + its credentials to the webauthn.User interface.
type webauthnUser struct {
	user        *models.User
	credentials []webauthn.Credential
}

func (u *webauthnUser) WebAuthnID() []byte {
	b, _ := u.user.ID.MarshalBinary()
	return b
}
func (u *webauthnUser) WebAuthnName() string                       { return u.user.Email }
func (u *webauthnUser) WebAuthnDisplayName() string                { return u.user.Name }
func (u *webauthnUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

func (s *WebAuthnService) loadUserWithCredentials(ctx context.Context, user *models.User) (*webauthnUser, error) {
	creds, err := s.credRepo.GetByUserID(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	out := make([]webauthn.Credential, 0, len(creds))
	for _, c := range creds {
		out = append(out, modelToWebAuthnCredential(c))
	}
	return &webauthnUser{user: user, credentials: out}, nil
}

func modelToWebAuthnCredential(c *models.WebAuthnCredential) webauthn.Credential {
	transports := make([]protocol.AuthenticatorTransport, 0, len(c.Transports))
	for _, t := range c.Transports {
		transports = append(transports, protocol.AuthenticatorTransport(t))
	}
	return webauthn.Credential{
		ID:              c.CredentialID,
		PublicKey:       c.PublicKey,
		AttestationType: c.AttestationType,
		Transport:       transports,
		Flags: webauthn.CredentialFlags{
			UserPresent:    c.UserPresent,
			UserVerified:   c.UserVerified,
			BackupEligible: c.BackupEligible,
			BackupState:    c.BackupState,
		},
		Authenticator: webauthn.Authenticator{
			AAGUID:    c.AAGUID,
			SignCount: c.SignCount,
		},
	}
}

// newWebAuthnHandle returns an opaque, unguessable ceremony handle. The raw
// value is returned to the client exactly once and never stored.
func newWebAuthnHandle() (string, error) {
	b := make([]byte, webauthnHandleBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate webauthn handle: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// hashWebAuthnHandle derives the storage key for a handle. Only the hash is
// persisted, so a database dump or read-only SQL injection yields no usable
// ceremony state.
func hashWebAuthnHandle(handle string) []byte {
	sum := sha256.Sum256([]byte(handle))
	return sum[:]
}

func (s *WebAuthnService) saveSession(ctx context.Context, userID *uuid.UUID, ceremony string, sd *webauthn.SessionData) (string, error) {
	raw, err := json.Marshal(sd)
	if err != nil {
		return "", err
	}
	handle, err := newWebAuthnHandle()
	if err != nil {
		return "", err
	}
	session := &models.WebAuthnSession{
		IDHash:    hashWebAuthnHandle(handle),
		UserID:    userID,
		Ceremony:  ceremony,
		Data:      raw,
		ExpiresAt: time.Now().Add(s.sessionTTL).UTC(),
	}
	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return "", err
	}
	WebAuthnSessionsCreatedTotal.WithLabelValues(ceremony).Inc()

	// Bound how many ceremonies a user can hold open, evicting oldest-first so
	// the attempt they just started always survives. Deliberately outside the
	// insert's transaction: a race between two concurrent begin calls leaves
	// briefly N+1 rows, which is harmless. A failure here must not fail the
	// ceremony the user is actually starting.
	if userID != nil {
		evicted, err := s.sessionRepo.DeleteOldestForUser(ctx, *userID, s.maxOpenCeremonies)
		if err != nil {
			slog.Warn("failed to evict old webauthn sessions", "user_id", *userID, "error", err)
		} else if evicted > 0 {
			WebAuthnSessionsEvictedTotal.Add(float64(evicted))
		}
	}
	return handle, nil
}

// StartSessionReaper deletes expired ceremony rows on a timer until ctx is
// cancelled.
//
// This is hygiene, not a control. Correctness is enforced by the
// `expires_at > NOW()` predicate in Consume, so a stopped or lagging reaper
// can never make a stale challenge usable — it only lets dead rows accumulate.
func (s *WebAuthnService) StartSessionReaper(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	go func() {
		slog.Info("WebAuthn session reaper started", "interval", interval.String())
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.Info("WebAuthn session reaper stopped")
				return
			case <-ticker.C:
				n, err := s.sessionRepo.DeleteExpired(ctx)
				if err != nil {
					slog.Warn("WebAuthn session cleanup failed", "error", err)
					continue
				}
				if n > 0 {
					WebAuthnSessionsExpiredTotal.Add(float64(n))
					slog.Debug("Expired WebAuthn sessions removed", "count", n)
				}
			}
		}
	}()
}

// consumeSession atomically claims a ceremony session. Every failure mode
// collapses to ErrWebAuthnSessionNotFound so callers cannot distinguish an
// expired handle from a consumed, mismatched or forged one.
func (s *WebAuthnService) consumeSession(ctx context.Context, handle, ceremony string) (*webauthn.SessionData, *models.WebAuthnSession, error) {
	if handle == "" {
		WebAuthnSessionsConsumedTotal.WithLabelValues(ceremony, "rejected").Inc()
		return nil, nil, ErrWebAuthnSessionNotFound
	}
	row, err := s.sessionRepo.Consume(ctx, hashWebAuthnHandle(handle), ceremony)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			WebAuthnSessionsConsumedTotal.WithLabelValues(ceremony, "rejected").Inc()
			return nil, nil, ErrWebAuthnSessionNotFound
		}
		return nil, nil, err
	}
	sd := &webauthn.SessionData{}
	if err := json.Unmarshal(row.Data, sd); err != nil {
		// A row whose payload will not decode is corrupt, not a transient
		// fault. It has already been consumed by the DELETE, so reject it the
		// same way as any other unusable handle rather than surfacing a 500.
		slog.Warn("discarding corrupt webauthn session", "ceremony", ceremony, "error", err)
		WebAuthnSessionsConsumedTotal.WithLabelValues(ceremony, "rejected").Inc()
		return nil, nil, ErrWebAuthnSessionNotFound
	}
	WebAuthnSessionsConsumedTotal.WithLabelValues(ceremony, "ok").Inc()
	return sd, row, nil
}

// BeginRegistration starts a passkey registration ceremony for an authenticated
// user. The returned handle must be presented to FinishRegistration.
func (s *WebAuthnService) BeginRegistration(ctx context.Context, userID uuid.UUID) (handle string, options *protocol.CredentialCreation, err error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return "", nil, err
	}
	wu, err := s.loadUserWithCredentials(ctx, user)
	if err != nil {
		return "", nil, err
	}

	// Require a discoverable (resident) credential with user verification so
	// the resulting credential is a real passkey: it gets saved into the
	// platform's credential manager, can be used for username-less sign-in,
	// and shows up in browser autofill on the login page.
	requireResident := true
	authenticatorSelection := protocol.AuthenticatorSelection{
		ResidentKey:        protocol.ResidentKeyRequirementRequired,
		RequireResidentKey: &requireResident,
		UserVerification:   protocol.VerificationRequired,
	}
	creation, sd, err := s.wa.BeginRegistration(wu, webauthn.WithAuthenticatorSelection(authenticatorSelection))
	if err != nil {
		return "", nil, fmt.Errorf("begin registration: %w", err)
	}
	uid := userID
	handle, err = s.saveSession(ctx, &uid, models.WebAuthnCeremonyRegistration, sd)
	if err != nil {
		return "", nil, err
	}
	return handle, creation, nil
}

// FinishRegistration verifies an attestation and stores the new credential.
func (s *WebAuthnService) FinishRegistration(ctx context.Context, userID uuid.UUID, handle string, label *string, responseJSON []byte) (*models.WebAuthnCredential, error) {
	sd, row, err := s.consumeSession(ctx, handle, models.WebAuthnCeremonyRegistration)
	if err != nil {
		return nil, err
	}

	// A registration session is scoped to the user who opened it. Without this
	// check a stolen handle would let its holder attach a credential to their
	// own account using someone else's challenge. Rejected uniformly so the
	// mismatch is not distinguishable from an unknown handle.
	if row.UserID == nil || *row.UserID != userID {
		slog.Warn("rejected webauthn registration session scoped to a different user",
			"authenticated_user_id", userID)
		return nil, ErrWebAuthnSessionNotFound
	}

	parsed, err := protocol.ParseCredentialCreationResponseBytes(responseJSON)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrWebAuthnInvalidResponse, err)
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	wu, err := s.loadUserWithCredentials(ctx, user)
	if err != nil {
		return nil, err
	}

	credential, err := s.wa.CreateCredential(wu, *sd, parsed)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrWebAuthnVerification, err)
	}

	transports := make(pq.StringArray, 0, len(credential.Transport))
	for _, t := range credential.Transport {
		transports = append(transports, string(t))
	}

	model := &models.WebAuthnCredential{
		ID:              uuid.New(),
		UserID:          userID,
		CredentialID:    credential.ID,
		PublicKey:       credential.PublicKey,
		AttestationType: credential.AttestationType,
		AAGUID:          credential.Authenticator.AAGUID,
		SignCount:       credential.Authenticator.SignCount,
		Transports:      transports,
		Label:           label,
		UserPresent:     credential.Flags.UserPresent,
		UserVerified:    credential.Flags.UserVerified,
		BackupEligible:  credential.Flags.BackupEligible,
		BackupState:     credential.Flags.BackupState,
		CreatedAt:       time.Now().UTC(),
	}
	if err := s.credRepo.Create(ctx, model); err != nil {
		return nil, err
	}
	return model, nil
}

// BeginLogin starts a passkey login ceremony. If email is non-empty, the user's existing
// credentials are advertised; otherwise a discoverable-credential challenge is issued.
func (s *WebAuthnService) BeginLogin(ctx context.Context, email string) (handle string, options *protocol.CredentialAssertion, err error) {
	email = strings.ToLower(strings.TrimSpace(email))

	if email == "" {
		assertion, sd, err := s.wa.BeginDiscoverableLogin()
		if err != nil {
			return "", nil, fmt.Errorf("begin discoverable login: %w", err)
		}
		handle, err = s.saveSession(ctx, nil, models.WebAuthnCeremonyLogin, sd)
		if err != nil {
			return "", nil, err
		}
		return handle, assertion, nil
	}

	if _, err := mail.ParseAddress(email); err != nil {
		return "", nil, ErrInvalidEmail
	}

	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		// Fall back to discoverable login to avoid email enumeration.
		assertion, sd, beginErr := s.wa.BeginDiscoverableLogin()
		if beginErr != nil {
			return "", nil, fmt.Errorf("begin discoverable login: %w", beginErr)
		}
		handle, beginErr = s.saveSession(ctx, nil, models.WebAuthnCeremonyLogin, sd)
		if beginErr != nil {
			return "", nil, beginErr
		}
		return handle, assertion, nil
	}

	wu, err := s.loadUserWithCredentials(ctx, user)
	if err != nil {
		return "", nil, err
	}

	assertion, sd, err := s.wa.BeginLogin(wu)
	if err != nil {
		// User may not have credentials yet — fall back to discoverable.
		assertion, sd, err = s.wa.BeginDiscoverableLogin()
		if err != nil {
			return "", nil, fmt.Errorf("begin login: %w", err)
		}
		handle, err = s.saveSession(ctx, nil, models.WebAuthnCeremonyLogin, sd)
		if err != nil {
			return "", nil, err
		}
		return handle, assertion, nil
	}
	uid := user.ID
	handle, err = s.saveSession(ctx, &uid, models.WebAuthnCeremonyLogin, sd)
	if err != nil {
		return "", nil, err
	}
	return handle, assertion, nil
}

// FinishLogin verifies an assertion and returns the authenticated user with a new TokenPair.
func (s *WebAuthnService) FinishLogin(ctx context.Context, handle string, responseJSON []byte) (*models.User, *TokenPair, error) {
	sd, _, err := s.consumeSession(ctx, handle, models.WebAuthnCeremonyLogin)
	if err != nil {
		return nil, nil, err
	}

	parsed, err := protocol.ParseCredentialRequestResponseBytes(responseJSON)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrWebAuthnInvalidResponse, err)
	}

	// Look up the credential and the user it belongs to via the rawID.
	storedCred, err := s.credRepo.GetByCredentialID(ctx, parsed.RawID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil, ErrWebAuthnUnknownCredential
		}
		return nil, nil, err
	}

	user, err := s.userRepo.GetByID(ctx, storedCred.UserID)
	if err != nil {
		return nil, nil, err
	}
	if user.Disabled {
		return nil, nil, ErrAccountDisabled
	}
	wu, err := s.loadUserWithCredentials(ctx, user)
	if err != nil {
		return nil, nil, err
	}

	// A session created by BeginDiscoverableLogin has an empty UserID, and must
	// be finished via ValidateDiscoverableLogin (which dispatches user lookup to
	// the provided handler). A session created by BeginLogin(user) has the
	// user's WebAuthnID and must be finished via ValidateLogin.
	var verified *webauthn.Credential
	if len(sd.UserID) == 0 {
		handler := func(rawID, userHandle []byte) (webauthn.User, error) {
			// userHandle from the authenticator MUST match the user we resolved
			// from rawID, otherwise someone could try to impersonate via a
			// crafted handle. Both should equal user.WebAuthnID().
			expected := wu.WebAuthnID()
			if len(userHandle) > 0 && !bytes.Equal(userHandle, expected) {
				return nil, ErrWebAuthnUnknownCredential
			}
			return wu, nil
		}
		verified, err = s.wa.ValidateDiscoverableLogin(handler, *sd, parsed)
	} else {
		verified, err = s.wa.ValidateLogin(wu, *sd, parsed)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrWebAuthnVerification, err)
	}

	// Update sign count for replay-clone detection.
	_ = s.credRepo.UpdateSignCount(ctx, storedCred.ID, verified.Authenticator.SignCount, time.Now().UTC())

	// Issue access + refresh tokens. Passkeys count as 2FA, so we skip the 2FA challenge.
	tokens, err := s.authService.GenerateTokensForUser(ctx, user.ID)
	if err != nil {
		return nil, nil, err
	}

	s.authService.RecordLogin(ctx, user)

	return user, tokens, nil
}

// ListCredentials returns the registered passkeys for a user.
func (s *WebAuthnService) ListCredentials(ctx context.Context, userID uuid.UUID) ([]*models.WebAuthnCredential, error) {
	return s.credRepo.GetByUserID(ctx, userID)
}

// DeleteCredential revokes a passkey owned by the given user.
func (s *WebAuthnService) DeleteCredential(ctx context.Context, userID, credentialID uuid.UUID) error {
	return s.credRepo.Delete(ctx, credentialID, userID)
}
