package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"sync"
	"time"

	oidclib "github.com/coreos/go-oidc/v3/oidc"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

// OIDC-specific sentinel errors. Handlers map these to status codes; none of
// them are ever echoed back to the browser verbatim.
var (
	// ErrOIDCProviderUnavailable means the issuer's discovery document could
	// not be fetched.
	ErrOIDCProviderUnavailable = errors.New("oidc provider is unavailable")

	// ErrOIDCInvalidState covers every unusable authorization response,
	// indistinguishably: an unknown, expired or already-consumed state, and a
	// state presented by a different browser than the one that started the
	// login.
	ErrOIDCInvalidState = errors.New("oidc login state is invalid or has expired")

	// ErrOIDCExchangeFailed means the provider rejected the authorization code
	// or returned a token response without an ID token.
	ErrOIDCExchangeFailed = errors.New("oidc token exchange failed")

	// ErrOIDCInvalidToken means the ID token failed signature, issuer,
	// audience, expiry or nonce validation.
	ErrOIDCInvalidToken = errors.New("oidc id token is invalid")

	// ErrOIDCEmailMissing means the ID token carried no usable email address;
	// an account cannot be provisioned without one.
	ErrOIDCEmailMissing = errors.New("oidc id token contains no email address")

	// ErrOIDCEmailConflict means the address belongs to an existing local
	// account that this identity is not allowed to adopt.
	ErrOIDCEmailConflict = errors.New("an account with this email address already exists")

	// ErrOIDCHandoffInvalid means the single-use handoff code was unknown,
	// expired or already redeemed.
	ErrOIDCHandoffInvalid = errors.New("oidc handoff code is invalid or has expired")
)

// oidcRandomBytes is the entropy used for the state, nonce, browser-binding
// value and handoff code.
const oidcRandomBytes = 32

// OIDCService implements the authorization-code + PKCE login flow against a
// single configured provider.
//
// It is constructed only when OIDC_ISSUER is set; a nil *OIDCService means the
// deployment runs in local-credential mode, and handlers check for that before
// touching any method here.
type OIDCService struct {
	cfg          OIDCConfig
	userRepo     repository.UserRepository
	identityRepo repository.OIDCIdentityRepository
	authService  *AuthService

	// Discovery is performed lazily and cached; a failure is retried on the
	// next login attempt.
	mu           sync.Mutex
	provider     *oidclib.Provider
	lastAttempt  time.Time
	discoveryErr error
}

// oidcDiscoveryRetryInterval throttles rediscovery after a failure.
const oidcDiscoveryRetryInterval = 15 * time.Second

// NewOIDCService returns a service for the supplied configuration, or nil when
// OIDC is not enabled. It does not contact the provider.
func NewOIDCService(
	cfg OIDCConfig,
	userRepo repository.UserRepository,
	identityRepo repository.OIDCIdentityRepository,
	authService *AuthService,
) (*OIDCService, error) {
	if !cfg.Enabled() {
		return nil, nil
	}
	if userRepo == nil || identityRepo == nil || authService == nil {
		return nil, errors.New("oidc service requires user repository, identity repository and auth service")
	}
	if cfg.LoginStateTTL <= 0 {
		cfg.LoginStateTTL = DefaultOIDCLoginStateTTL
	}
	if cfg.HandoffTTL <= 0 {
		cfg.HandoffTTL = DefaultOIDCHandoffTTL
	}
	return &OIDCService{
		cfg:          cfg,
		userRepo:     userRepo,
		identityRepo: identityRepo,
		authService:  authService,
	}, nil
}

// Config returns the effective configuration (no secrets are exposed by the
// callers that use it).
func (s *OIDCService) Config() OIDCConfig { return s.cfg }

// PostLoginRedirect is the fixed frontend URL the callback redirects to.
func (s *OIDCService) PostLoginRedirect() string { return s.cfg.PostLoginRedirect }

// resolveProvider returns the cached provider, performing discovery on first
// use. Failures are cached briefly so a down provider is retried, not hammered.
func (s *OIDCService) resolveProvider(ctx context.Context) (*oidclib.Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.provider != nil {
		return s.provider, nil
	}
	if time.Since(s.lastAttempt) < oidcDiscoveryRetryInterval && s.discoveryErr != nil {
		return nil, ErrOIDCProviderUnavailable
	}
	s.lastAttempt = time.Now()

	provider, err := oidclib.NewProvider(ctx, s.cfg.Issuer)
	if err != nil {
		s.discoveryErr = err
		slog.Error("OIDC discovery failed", "issuer", s.cfg.Issuer, "error", err)
		return nil, ErrOIDCProviderUnavailable
	}
	s.provider = provider
	s.discoveryErr = nil
	slog.Info("OIDC provider discovered", "issuer", s.cfg.Issuer)
	return provider, nil
}

func (s *OIDCService) oauthConfig(provider *oidclib.Provider) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     s.cfg.ClientID,
		ClientSecret: s.cfg.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  s.cfg.RedirectURL,
		Scopes:       s.cfg.Scopes,
	}
}

// OIDCAuthorization is what the authorize step hands back: the provider URL
// to send the browser to, and the value to plant in the browser as a cookie
// for the callback's same-browser check.
type OIDCAuthorization struct {
	AuthorizationURL string
	BrowserToken     string
	// Expiry is when the pending login stops being usable; used as the
	// cookie's Max-Age.
	Expiry time.Time
}

// BeginLogin creates a pending authorization request and returns the provider
// URL to redirect the browser to. Three independent single-use values are
// minted: `state`, `nonce`, and the PKCE verifier. Only hashes of the state
// and the browser token are persisted.
func (s *OIDCService) BeginLogin(ctx context.Context) (*OIDCAuthorization, error) {
	provider, err := s.resolveProvider(ctx)
	if err != nil {
		return nil, err
	}

	state, err := randomOIDCToken()
	if err != nil {
		return nil, err
	}
	nonce, err := randomOIDCToken()
	if err != nil {
		return nil, err
	}
	browserToken, err := randomOIDCToken()
	if err != nil {
		return nil, err
	}
	verifier := oauth2.GenerateVerifier()
	expiresAt := time.Now().Add(s.cfg.LoginStateTTL)

	if err := s.identityRepo.CreateLoginState(ctx, &models.OIDCLoginState{
		StateHash:    hashOIDCToken(state),
		BrowserHash:  hashOIDCToken(browserToken),
		Nonce:        nonce,
		CodeVerifier: verifier,
		ExpiresAt:    expiresAt,
	}); err != nil {
		return nil, err
	}

	authURL := s.oauthConfig(provider).AuthCodeURL(state,
		oidclib.Nonce(nonce),
		oauth2.S256ChallengeOption(verifier),
	)
	return &OIDCAuthorization{
		AuthorizationURL: authURL,
		BrowserToken:     browserToken,
		Expiry:           expiresAt,
	}, nil
}

// CompleteCallback validates the provider's response, provisions or updates
// the local user, and returns only a single-use handoff code for the browser;
// the access and refresh tokens are minted later, by ExchangeHandoff.
func (s *OIDCService) CompleteCallback(ctx context.Context, code, state, browserToken string) (string, error) {
	if code == "" || state == "" || browserToken == "" {
		return "", ErrOIDCInvalidState
	}
	provider, err := s.resolveProvider(ctx)
	if err != nil {
		return "", err
	}

	// Consuming the state makes an authorization response usable exactly
	// once.
	loginState, err := s.identityRepo.ConsumeLoginState(ctx, hashOIDCToken(state))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return "", ErrOIDCInvalidState
		}
		return "", err
	}
	// Constant-time compare of the browser token.
	if subtle.ConstantTimeCompare(loginState.BrowserHash, hashOIDCToken(browserToken)) != 1 {
		return "", ErrOIDCInvalidState
	}

	oauthCfg := s.oauthConfig(provider)
	token, err := oauthCfg.Exchange(ctx, code, oauth2.VerifierOption(loginState.CodeVerifier))
	if err != nil {
		slog.Warn("OIDC code exchange failed", "error", err)
		return "", ErrOIDCExchangeFailed
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		slog.Warn("OIDC token response contained no id_token")
		return "", ErrOIDCExchangeFailed
	}

	// Verifier checks the signature against the provider's JWKS and validates
	// issuer, audience and expiry; the nonce is checked here.
	idToken, err := provider.Verifier(&oidclib.Config{ClientID: s.cfg.ClientID}).Verify(ctx, rawIDToken)
	if err != nil {
		slog.Warn("OIDC id_token verification failed", "error", err)
		return "", ErrOIDCInvalidToken
	}
	if subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(loginState.Nonce)) != 1 {
		slog.Warn("OIDC id_token nonce mismatch")
		return "", ErrOIDCInvalidToken
	}

	claims, err := parseOIDCClaims(idToken, s.cfg.NameClaim)
	if err != nil {
		return "", err
	}

	user, err := s.provisionUser(ctx, idToken.Issuer, claims)
	if err != nil {
		return "", err
	}

	handoff, err := randomOIDCToken()
	if err != nil {
		return "", err
	}
	if err := s.identityRepo.CreateHandoffCode(ctx, &models.OIDCHandoffCode{
		CodeHash:  hashOIDCToken(handoff),
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(s.cfg.HandoffTTL),
	}); err != nil {
		return "", err
	}
	return handoff, nil
}

// ExchangeHandoff redeems a handoff code for the user and a fresh token pair.
func (s *OIDCService) ExchangeHandoff(ctx context.Context, handoff string) (*models.User, *TokenPair, error) {
	if handoff == "" {
		return nil, nil, ErrOIDCHandoffInvalid
	}
	entry, err := s.identityRepo.ConsumeHandoffCode(ctx, hashOIDCToken(handoff))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil, ErrOIDCHandoffInvalid
		}
		return nil, nil, err
	}

	user, err := s.userRepo.GetByID(ctx, entry.UserID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil, ErrOIDCHandoffInvalid
		}
		return nil, nil, err
	}
	// Disabled state is re-checked at exchange time.
	if user.Disabled {
		return nil, nil, ErrAccountDisabled
	}

	// One active session per user, matching password login.
	if err := s.authService.refreshTokenRepo.DeleteForUser(ctx, user.ID); err != nil {
		slog.Warn("failed to clear refresh tokens on OIDC login", "error", err)
	}
	tokens, err := s.authService.GenerateTokensForUser(ctx, user.ID)
	if err != nil {
		return nil, nil, err
	}

	s.authService.RecordLogin(ctx, user)
	return user, tokens, nil
}

// oidcClaims is the subset of the ID token NinerLog consumes.
type oidcClaims struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
}

// parseOIDCClaims extracts and normalises the claims an account is built
// from. The display name falls back through the configured claim, `name`,
// `preferred_username` and finally the local part of the address.
func parseOIDCClaims(idToken *oidclib.IDToken, nameClaim string) (oidcClaims, error) {
	var raw map[string]any
	if err := idToken.Claims(&raw); err != nil {
		return oidcClaims{}, ErrOIDCInvalidToken
	}

	out := oidcClaims{Subject: idToken.Subject}
	if out.Subject == "" {
		return oidcClaims{}, ErrOIDCInvalidToken
	}

	out.Email = strings.ToLower(strings.TrimSpace(claimString(raw, "email")))
	if out.Email == "" {
		return oidcClaims{}, ErrOIDCEmailMissing
	}
	if _, err := mail.ParseAddress(out.Email); err != nil || len(out.Email) > 255 {
		return oidcClaims{}, ErrOIDCEmailMissing
	}
	out.EmailVerified = claimBool(raw, "email_verified")

	for _, key := range []string{nameClaim, "name", "preferred_username"} {
		if key == "" {
			continue
		}
		if v := strings.TrimSpace(claimString(raw, key)); v != "" {
			out.Name = v
			break
		}
	}
	if out.Name == "" {
		out.Name = out.Email[:strings.Index(out.Email, "@")]
	}
	if len(out.Name) > 100 {
		out.Name = out.Name[:100]
	}
	return out, nil
}

func claimString(raw map[string]any, key string) string {
	if v, ok := raw[key].(string); ok {
		return v
	}
	return ""
}

// claimBool accepts the boolean and the stringified spellings ("true").
func claimBool(raw map[string]any, key string) bool {
	switch v := raw[key].(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	default:
		return false
	}
}

// provisionUser resolves the external identity to a local user, creating one
// on first sight. Lookup is always by (issuer, subject) — never by email.
func (s *OIDCService) provisionUser(ctx context.Context, issuer string, claims oidcClaims) (*models.User, error) {
	identity, err := s.identityRepo.GetBySubject(ctx, issuer, claims.Subject)
	switch {
	case err == nil:
		user, err := s.userRepo.GetByID(ctx, identity.UserID)
		if err != nil {
			return nil, err
		}
		if user.Disabled {
			return nil, ErrAccountDisabled
		}
		if err := s.syncUserFromClaims(ctx, user, claims); err != nil {
			return nil, err
		}
		if err := s.identityRepo.TouchLogin(ctx, identity.ID, claims.Email, time.Now()); err != nil {
			slog.Warn("failed to record OIDC identity login", "error", err)
		}
		return user, nil
	case errors.Is(err, repository.ErrNotFound):
		// fall through to first-login handling
	default:
		return nil, err
	}

	existing, err := s.userRepo.GetByEmail(ctx, claims.Email)
	switch {
	case err == nil:
		// An account already holds this address. Adopting it requires the
		// provider to assert the address is verified AND the operator opt-in.
		if !s.cfg.LinkByVerifiedEmail || !claims.EmailVerified {
			slog.Warn("OIDC login refused: email belongs to an existing account",
				"link_by_verified_email", s.cfg.LinkByVerifiedEmail,
				"email_verified", claims.EmailVerified)
			return nil, ErrOIDCEmailConflict
		}
		if existing.Disabled {
			return nil, ErrAccountDisabled
		}
		if err := s.linkIdentity(ctx, existing.ID, issuer, claims); err != nil {
			return nil, err
		}
		if err := s.syncUserFromClaims(ctx, existing, claims); err != nil {
			return nil, err
		}
		slog.Info("OIDC identity linked to existing account", "user_id", existing.ID)
		return existing, nil
	case errors.Is(err, repository.ErrNotFound):
		// fall through to account creation
	default:
		return nil, err
	}

	now := time.Now()
	user := &models.User{
		Email: claims.Email,
		// An OIDC account has no local password; an empty hash never
		// validates.
		PasswordHash:  "",
		Name:          claims.Name,
		EmailVerified: claims.EmailVerified || s.cfg.TrustEmailVerified,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.userRepo.Create(ctx, user); err != nil {
		if errors.Is(err, repository.ErrDuplicateEmail) {
			return nil, ErrOIDCEmailConflict
		}
		return nil, err
	}
	if err := s.linkIdentity(ctx, user.ID, issuer, claims); err != nil {
		return nil, err
	}
	slog.Info("Provisioned user from OIDC", "user_id", user.ID)
	return user, nil
}

// linkIdentity records the (issuer, subject) → user mapping, tolerating the
// race where a concurrent login for the same subject won.
func (s *OIDCService) linkIdentity(ctx context.Context, userID uuid.UUID, issuer string, claims oidcClaims) error {
	now := time.Now()
	err := s.identityRepo.Create(ctx, &models.OIDCIdentity{
		UserID:      userID,
		Issuer:      issuer,
		Subject:     claims.Subject,
		Email:       claims.Email,
		LastLoginAt: &now,
	})
	if errors.Is(err, repository.ErrDuplicate) {
		return nil
	}
	return err
}

// syncUserFromClaims keeps the local account in step with the provider, which
// owns email and display name in OIDC mode. Only changed fields are written.
func (s *OIDCService) syncUserFromClaims(ctx context.Context, user *models.User, claims oidcClaims) error {
	verified := claims.EmailVerified || s.cfg.TrustEmailVerified
	changed := false

	if !strings.EqualFold(user.Email, claims.Email) {
		// The address moved at the provider. On a collision with another
		// local account, keep the old address and log.
		if other, err := s.userRepo.GetByEmail(ctx, claims.Email); err == nil && other.ID != user.ID {
			slog.Warn("OIDC email change ignored: address already in use by another account",
				"user_id", user.ID)
		} else {
			user.Email = claims.Email
			changed = true
		}
	}
	if claims.Name != "" && user.Name != claims.Name {
		user.Name = claims.Name
		changed = true
	}
	if user.EmailVerified != verified {
		user.EmailVerified = verified
		changed = true
	}
	if !changed {
		return nil
	}
	user.UpdatedAt = time.Now()
	if err := s.userRepo.Update(ctx, user); err != nil {
		if errors.Is(err, repository.ErrDuplicateEmail) {
			return ErrOIDCEmailConflict
		}
		return err
	}
	return nil
}

// StartStateReaper deletes expired login states and handoff codes on a timer.
// Hygiene only: consumption already refuses expired rows.
func (s *OIDCService) StartStateReaper(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if n, err := s.identityRepo.DeleteExpired(ctx); err != nil {
					slog.Warn("OIDC state cleanup failed", "error", err)
				} else if n > 0 {
					slog.Debug("Swept expired OIDC state", "rows", n)
				}
			}
		}
	}()
}

func randomOIDCToken() (string, error) {
	buf := make([]byte, oidcRandomBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate oidc token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashOIDCToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
