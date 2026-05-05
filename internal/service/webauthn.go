package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	ErrWebAuthnNotConfigured     = errors.New("webauthn is not configured")
	ErrWebAuthnSessionNotFound   = errors.New("webauthn session not found or expired")
	ErrWebAuthnSessionExpired    = errors.New("webauthn session expired")
	ErrWebAuthnInvalidResponse   = errors.New("invalid webauthn response")
	ErrWebAuthnUnknownCredential = errors.New("unknown webauthn credential")
	ErrWebAuthnVerification      = errors.New("webauthn verification failed")
)

const (
	webauthnSessionPurposeRegistration = "registration"
	webauthnSessionPurposeLogin        = "login"
	webauthnSessionTTL                 = 5 * time.Minute
)

// WebAuthnService implements passkey registration & login flows.
type WebAuthnService struct {
	wa          *webauthn.WebAuthn
	credRepo    repository.WebAuthnCredentialRepository
	sessionRepo repository.WebAuthnSessionRepository
	userRepo    repository.UserRepository
	authService *AuthService
}

// NewWebAuthnService creates a new WebAuthnService. Returns nil and an error if the
// webauthn library cannot be initialized with the given config (e.g. invalid origins).
func NewWebAuthnService(
	rpID, rpName string,
	rpOrigins []string,
	credRepo repository.WebAuthnCredentialRepository,
	sessionRepo repository.WebAuthnSessionRepository,
	userRepo repository.UserRepository,
	authService *AuthService,
) (*WebAuthnService, error) {
	if rpID == "" || rpName == "" || len(rpOrigins) == 0 {
		return nil, ErrWebAuthnNotConfigured
	}
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          rpID,
		RPDisplayName: rpName,
		RPOrigins:     rpOrigins,
	})
	if err != nil {
		return nil, fmt.Errorf("init webauthn: %w", err)
	}
	return &WebAuthnService{
		wa:          wa,
		credRepo:    credRepo,
		sessionRepo: sessionRepo,
		userRepo:    userRepo,
		authService: authService,
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

func (s *WebAuthnService) saveSession(ctx context.Context, userID *uuid.UUID, purpose string, sd *webauthn.SessionData) (string, map[string]interface{}, error) {
	raw, err := json.Marshal(sd)
	if err != nil {
		return "", nil, err
	}
	session := &models.WebAuthnSession{
		ID:          uuid.New(),
		UserID:      userID,
		Challenge:   sd.Challenge,
		SessionData: raw,
		Purpose:     purpose,
		ExpiresAt:   time.Now().Add(webauthnSessionTTL).UTC(),
	}
	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return "", nil, err
	}
	return session.ID.String(), nil, nil
}

func (s *WebAuthnService) consumeSession(ctx context.Context, sessionID, purpose string) (*webauthn.SessionData, *models.WebAuthnSession, error) {
	id, err := uuid.Parse(sessionID)
	if err != nil {
		return nil, nil, ErrWebAuthnSessionNotFound
	}
	row, err := s.sessionRepo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil, ErrWebAuthnSessionNotFound
		}
		return nil, nil, err
	}
	// Always delete after a single use.
	defer func() { _ = s.sessionRepo.Delete(ctx, id) }()

	if row.Purpose != purpose {
		return nil, nil, ErrWebAuthnSessionNotFound
	}
	if time.Now().After(row.ExpiresAt) {
		return nil, nil, ErrWebAuthnSessionExpired
	}
	sd := &webauthn.SessionData{}
	if err := json.Unmarshal(row.SessionData, sd); err != nil {
		return nil, nil, err
	}
	return sd, row, nil
}

// BeginRegistration starts a passkey registration ceremony for an authenticated user.
func (s *WebAuthnService) BeginRegistration(ctx context.Context, userID uuid.UUID) (sessionID string, options *protocol.CredentialCreation, err error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return "", nil, err
	}
	wu, err := s.loadUserWithCredentials(ctx, user)
	if err != nil {
		return "", nil, err
	}

	creation, sd, err := s.wa.BeginRegistration(wu)
	if err != nil {
		return "", nil, fmt.Errorf("begin registration: %w", err)
	}
	uid := userID
	sessionID, _, err = s.saveSession(ctx, &uid, webauthnSessionPurposeRegistration, sd)
	if err != nil {
		return "", nil, err
	}
	return sessionID, creation, nil
}

// FinishRegistration verifies an attestation and stores the new credential.
func (s *WebAuthnService) FinishRegistration(ctx context.Context, userID uuid.UUID, sessionID string, label *string, responseJSON []byte) (*models.WebAuthnCredential, error) {
	sd, _, err := s.consumeSession(ctx, sessionID, webauthnSessionPurposeRegistration)
	if err != nil {
		return nil, err
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
func (s *WebAuthnService) BeginLogin(ctx context.Context, email string) (sessionID string, options *protocol.CredentialAssertion, err error) {
	email = strings.ToLower(strings.TrimSpace(email))

	if email == "" {
		assertion, sd, err := s.wa.BeginDiscoverableLogin()
		if err != nil {
			return "", nil, fmt.Errorf("begin discoverable login: %w", err)
		}
		sessionID, _, err = s.saveSession(ctx, nil, webauthnSessionPurposeLogin, sd)
		if err != nil {
			return "", nil, err
		}
		return sessionID, assertion, nil
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
		sessionID, _, beginErr = s.saveSession(ctx, nil, webauthnSessionPurposeLogin, sd)
		if beginErr != nil {
			return "", nil, beginErr
		}
		return sessionID, assertion, nil
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
		sessionID, _, err = s.saveSession(ctx, nil, webauthnSessionPurposeLogin, sd)
		if err != nil {
			return "", nil, err
		}
		return sessionID, assertion, nil
	}
	uid := user.ID
	sessionID, _, err = s.saveSession(ctx, &uid, webauthnSessionPurposeLogin, sd)
	if err != nil {
		return "", nil, err
	}
	return sessionID, assertion, nil
}

// FinishLogin verifies an assertion and returns the authenticated user with a new TokenPair.
func (s *WebAuthnService) FinishLogin(ctx context.Context, sessionID string, responseJSON []byte) (*models.User, *TokenPair, error) {
	sd, _, err := s.consumeSession(ctx, sessionID, webauthnSessionPurposeLogin)
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

	verified, err := s.wa.ValidateLogin(wu, *sd, parsed)
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

	now := time.Now()
	user.LastLoginAt = &now
	_ = s.userRepo.Update(ctx, user)

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
