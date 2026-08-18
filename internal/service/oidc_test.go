package service_test

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/pkg/jwt"
	"github.com/google/uuid"
)

const testClientID = "ninerlog-test-client"

// oidcUserRepo is mockUserRepo with the auto-verify behaviour removed;
// email_verified must come from the ID token.
type oidcUserRepo struct{ *mockUserRepo }

func newOIDCUserRepo() *oidcUserRepo { return &oidcUserRepo{newMockUserRepo()} }

func (m *oidcUserRepo) Create(ctx context.Context, user *models.User) error {
	if _, exists := m.users[user.Email]; exists {
		return repository.ErrDuplicateEmail
	}
	user.ID = uuid.New()
	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now()
	}
	user.UpdatedAt = time.Now()
	m.users[user.Email] = user
	return nil
}

// mockOIDCIdentityRepo is an in-memory OIDCIdentityRepository with the same
// exactly-once consumption semantics as the Postgres implementation.
type mockOIDCIdentityRepo struct {
	mu         sync.Mutex
	identities []*models.OIDCIdentity
	states     map[string]*models.OIDCLoginState
	handoffs   map[string]*models.OIDCHandoffCode
}

func newMockOIDCIdentityRepo() *mockOIDCIdentityRepo {
	return &mockOIDCIdentityRepo{
		states:   map[string]*models.OIDCLoginState{},
		handoffs: map[string]*models.OIDCHandoffCode{},
	}
}

func (m *mockOIDCIdentityRepo) GetBySubject(_ context.Context, issuer, subject string) (*models.OIDCIdentity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, i := range m.identities {
		if i.Issuer == issuer && i.Subject == subject {
			return i, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (m *mockOIDCIdentityRepo) GetByUserID(_ context.Context, userID uuid.UUID) (*models.OIDCIdentity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, i := range m.identities {
		if i.UserID == userID {
			return i, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (m *mockOIDCIdentityRepo) Create(_ context.Context, identity *models.OIDCIdentity) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, i := range m.identities {
		if i.Issuer == identity.Issuer && i.Subject == identity.Subject {
			return repository.ErrDuplicate
		}
	}
	if identity.ID == uuid.Nil {
		identity.ID = uuid.New()
	}
	m.identities = append(m.identities, identity)
	return nil
}

func (m *mockOIDCIdentityRepo) TouchLogin(_ context.Context, id uuid.UUID, email string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, i := range m.identities {
		if i.ID == id {
			i.Email = email
			i.LastLoginAt = &at
			return nil
		}
	}
	return repository.ErrNotFound
}

func (m *mockOIDCIdentityRepo) CreateLoginState(_ context.Context, state *models.OIDCLoginState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states[string(state.StateHash)] = state
	return nil
}

func (m *mockOIDCIdentityRepo) ConsumeLoginState(_ context.Context, stateHash []byte) (*models.OIDCLoginState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.states[string(stateHash)]
	if !ok || !s.ExpiresAt.After(time.Now()) {
		return nil, repository.ErrNotFound
	}
	delete(m.states, string(stateHash))
	return s, nil
}

func (m *mockOIDCIdentityRepo) CreateHandoffCode(_ context.Context, code *models.OIDCHandoffCode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handoffs[string(code.CodeHash)] = code
	return nil
}

func (m *mockOIDCIdentityRepo) ConsumeHandoffCode(_ context.Context, codeHash []byte) (*models.OIDCHandoffCode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.handoffs[string(codeHash)]
	if !ok || !h.ExpiresAt.After(time.Now()) {
		return nil, repository.ErrNotFound
	}
	delete(m.handoffs, string(codeHash))
	return h, nil
}

func (m *mockOIDCIdentityRepo) DeleteExpired(context.Context) (int64, error) { return 0, nil }

// oidcHarness bundles a fake provider, a service under test and its repos.
type oidcHarness struct {
	idp       *fakeIDP
	svc       *service.OIDCService
	users     *oidcUserRepo
	ids       *mockOIDCIdentityRepo
	auth      *service.AuthService
	refreshes *mockRefreshTokenRepo
}

func newOIDCHarness(t *testing.T, mutate func(*service.OIDCConfig)) *oidcHarness {
	t.Helper()
	idp := newFakeIDP(t, testClientID)

	users := newOIDCUserRepo()
	refreshes := newMockRefreshTokenRepo()
	jwtManager := jwt.NewManager("test-access-secret", "test-refresh-secret", 15*time.Minute, 7*24*time.Hour)
	authService := service.NewAuthService(
		users, refreshes, newMockPasswordResetRepo(), newMockEmailVerificationRepo(),
		jwtManager, service.NewTwoFactorService(users, jwtManager, nil),
	)

	cfg := service.OIDCConfig{
		Issuer:            idp.issuer(),
		ClientID:          testClientID,
		ClientSecret:      "test-client-secret",
		RedirectURL:       "https://logbook.example/api/v1/auth/oidc/callback",
		PostLoginRedirect: "https://logbook.example/auth/callback",
		Scopes:            []string{"openid", "profile", "email"},
		ProviderName:      "Test IdP",
		NameClaim:         "name",
		LoginStateTTL:     10 * time.Minute,
		HandoffTTL:        time.Minute,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	ids := newMockOIDCIdentityRepo()
	svc, err := service.NewOIDCService(cfg, users, ids, authService)
	if err != nil {
		t.Fatalf("NewOIDCService: %v", err)
	}
	if svc == nil {
		t.Fatal("NewOIDCService returned nil for an enabled config")
	}

	idp.setClaims(map[string]any{
		"sub":            "subject-1",
		"email":          "pilot@example.com",
		"email_verified": true,
		"name":           "Amelia Earhart",
	})
	return &oidcHarness{idp: idp, svc: svc, users: users, ids: ids, auth: authService, refreshes: refreshes}
}

// login runs one complete browser round trip and returns the handoff code.
func (h *oidcHarness) login(t *testing.T) (string, error) {
	t.Helper()
	auth, err := h.svc.BeginLogin(context.Background())
	if err != nil {
		return "", err
	}
	h.idp.echoNonce(auth.AuthorizationURL)
	return h.svc.CompleteCallback(context.Background(),
		"authorization-code", stateFrom(t, auth.AuthorizationURL), auth.BrowserToken)
}

func TestOIDCLoginProvisionsUserAndIssuesTokens(t *testing.T) {
	h := newOIDCHarness(t, nil)

	handoff, err := h.login(t)
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	user, tokens, err := h.svc.ExchangeHandoff(context.Background(), handoff)
	if err != nil {
		t.Fatalf("ExchangeHandoff: %v", err)
	}
	if user.Email != "pilot@example.com" {
		t.Errorf("email = %q, want pilot@example.com", user.Email)
	}
	if user.Name != "Amelia Earhart" {
		t.Errorf("name = %q, want Amelia Earhart", user.Name)
	}
	if !user.EmailVerified {
		t.Error("email_verified claim was true but the account is unverified")
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		t.Error("expected a full token pair")
	}
	// The whole point of provisioning: no local credential is created.
	if user.HasPassword() {
		t.Error("OIDC-provisioned account must not have a usable password hash")
	}
	if _, err := h.ids.GetBySubject(context.Background(), h.idp.issuer(), "subject-1"); err != nil {
		t.Errorf("identity was not linked: %v", err)
	}
}

func TestOIDCLoginSendsPKCEVerifier(t *testing.T) {
	h := newOIDCHarness(t, nil)
	auth, err := h.svc.BeginLogin(context.Background())
	if err != nil {
		t.Fatalf("BeginLogin: %v", err)
	}
	u, err := url.Parse(auth.AuthorizationURL)
	if err != nil {
		t.Fatalf("parse authorization url: %v", err)
	}
	if got := u.Query().Get("code_challenge_method"); got != "S256" {
		t.Errorf("code_challenge_method = %q, want S256", got)
	}
	if u.Query().Get("code_challenge") == "" {
		t.Error("authorization url carried no code_challenge")
	}

	h.idp.echoNonce(auth.AuthorizationURL)
	if _, err := h.svc.CompleteCallback(context.Background(),
		"authorization-code", stateFrom(t, auth.AuthorizationURL), auth.BrowserToken); err != nil {
		t.Fatalf("CompleteCallback: %v", err)
	}
	if v := h.idp.lastForm.Get("code_verifier"); v == "" {
		t.Error("token request carried no code_verifier")
	}
}

func TestOIDCCallbackRejectsReplayedState(t *testing.T) {
	h := newOIDCHarness(t, nil)
	auth, err := h.svc.BeginLogin(context.Background())
	if err != nil {
		t.Fatalf("BeginLogin: %v", err)
	}
	h.idp.echoNonce(auth.AuthorizationURL)
	state := stateFrom(t, auth.AuthorizationURL)

	if _, err := h.svc.CompleteCallback(context.Background(), "code", state, auth.BrowserToken); err != nil {
		t.Fatalf("first callback: %v", err)
	}
	_, err = h.svc.CompleteCallback(context.Background(), "code", state, auth.BrowserToken)
	if !errors.Is(err, service.ErrOIDCInvalidState) {
		t.Errorf("replayed state error = %v, want ErrOIDCInvalidState", err)
	}
}

func TestOIDCCallbackRejectsForeignBrowser(t *testing.T) {
	h := newOIDCHarness(t, nil)
	auth, err := h.svc.BeginLogin(context.Background())
	if err != nil {
		t.Fatalf("BeginLogin: %v", err)
	}
	h.idp.echoNonce(auth.AuthorizationURL)

	// The attacker relays their own state into a victim's browser, which
	// carries a different cookie (or none at all).
	_, err = h.svc.CompleteCallback(context.Background(), "code",
		stateFrom(t, auth.AuthorizationURL), "some-other-browser-token")
	if !errors.Is(err, service.ErrOIDCInvalidState) {
		t.Errorf("foreign browser error = %v, want ErrOIDCInvalidState", err)
	}
	_, err = h.svc.CompleteCallback(context.Background(), "code",
		stateFrom(t, auth.AuthorizationURL), "")
	if !errors.Is(err, service.ErrOIDCInvalidState) {
		t.Errorf("missing browser token error = %v, want ErrOIDCInvalidState", err)
	}
}

func TestOIDCCallbackRejectsUnknownState(t *testing.T) {
	h := newOIDCHarness(t, nil)
	_, err := h.svc.CompleteCallback(context.Background(), "code", "never-issued", "browser")
	if !errors.Is(err, service.ErrOIDCInvalidState) {
		t.Errorf("unknown state error = %v, want ErrOIDCInvalidState", err)
	}
}

func TestOIDCCallbackRejectsNonceMismatch(t *testing.T) {
	h := newOIDCHarness(t, nil)
	auth, err := h.svc.BeginLogin(context.Background())
	if err != nil {
		t.Fatalf("BeginLogin: %v", err)
	}
	// A token minted for a different login must not be accepted.
	h.idp.nonceOverride = "some-other-logins-nonce"

	_, err = h.svc.CompleteCallback(context.Background(), "code",
		stateFrom(t, auth.AuthorizationURL), auth.BrowserToken)
	if !errors.Is(err, service.ErrOIDCInvalidToken) {
		t.Errorf("nonce mismatch error = %v, want ErrOIDCInvalidToken", err)
	}
}

func TestOIDCCallbackRejectsMissingNonce(t *testing.T) {
	h := newOIDCHarness(t, nil)
	auth, err := h.svc.BeginLogin(context.Background())
	if err != nil {
		t.Fatalf("BeginLogin: %v", err)
	}
	// No echoNonce call: the ID token carries no nonce at all.
	_, err = h.svc.CompleteCallback(context.Background(), "code",
		stateFrom(t, auth.AuthorizationURL), auth.BrowserToken)
	if !errors.Is(err, service.ErrOIDCInvalidToken) {
		t.Errorf("missing nonce error = %v, want ErrOIDCInvalidToken", err)
	}
}

func TestOIDCCallbackRejectsForeignSigningKey(t *testing.T) {
	h := newOIDCHarness(t, nil)
	other := newFakeIDP(t, testClientID)
	h.idp.signWith = other.key

	auth, err := h.svc.BeginLogin(context.Background())
	if err != nil {
		t.Fatalf("BeginLogin: %v", err)
	}
	h.idp.echoNonce(auth.AuthorizationURL)

	_, err = h.svc.CompleteCallback(context.Background(), "code",
		stateFrom(t, auth.AuthorizationURL), auth.BrowserToken)
	if !errors.Is(err, service.ErrOIDCInvalidToken) {
		t.Errorf("foreign key error = %v, want ErrOIDCInvalidToken", err)
	}
}

func TestOIDCCallbackRejectsForeignAudience(t *testing.T) {
	h := newOIDCHarness(t, nil)
	// A token the provider legitimately issued — to a different client.
	h.idp.audience = "some-other-application"

	auth, err := h.svc.BeginLogin(context.Background())
	if err != nil {
		t.Fatalf("BeginLogin: %v", err)
	}
	h.idp.echoNonce(auth.AuthorizationURL)

	_, err = h.svc.CompleteCallback(context.Background(), "code",
		stateFrom(t, auth.AuthorizationURL), auth.BrowserToken)
	if !errors.Is(err, service.ErrOIDCInvalidToken) {
		t.Errorf("foreign audience error = %v, want ErrOIDCInvalidToken", err)
	}
}

func TestOIDCCallbackRejectsExpiredToken(t *testing.T) {
	h := newOIDCHarness(t, nil)
	h.idp.expiry = -time.Minute

	auth, err := h.svc.BeginLogin(context.Background())
	if err != nil {
		t.Fatalf("BeginLogin: %v", err)
	}
	h.idp.echoNonce(auth.AuthorizationURL)

	_, err = h.svc.CompleteCallback(context.Background(), "code",
		stateFrom(t, auth.AuthorizationURL), auth.BrowserToken)
	if !errors.Is(err, service.ErrOIDCInvalidToken) {
		t.Errorf("expired token error = %v, want ErrOIDCInvalidToken", err)
	}
}

func TestOIDCRequiresEmailClaim(t *testing.T) {
	for _, tc := range []struct {
		name  string
		email any
	}{
		{"absent", nil},
		{"empty", ""},
		{"malformed", "not-an-address"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newOIDCHarness(t, nil)
			claims := map[string]any{"sub": "subject-1", "name": "No Mail"}
			if tc.email != nil {
				claims["email"] = tc.email
			}
			h.idp.setClaims(claims)

			_, err := h.login(t)
			if !errors.Is(err, service.ErrOIDCEmailMissing) {
				t.Errorf("error = %v, want ErrOIDCEmailMissing", err)
			}
		})
	}
}

func TestOIDCSecondLoginReusesTheSameAccount(t *testing.T) {
	h := newOIDCHarness(t, nil)

	first, err := h.login(t)
	if err != nil {
		t.Fatalf("first login: %v", err)
	}
	userA, _, err := h.svc.ExchangeHandoff(context.Background(), first)
	if err != nil {
		t.Fatalf("first exchange: %v", err)
	}

	second, err := h.login(t)
	if err != nil {
		t.Fatalf("second login: %v", err)
	}
	userB, _, err := h.svc.ExchangeHandoff(context.Background(), second)
	if err != nil {
		t.Fatalf("second exchange: %v", err)
	}

	if userA.ID != userB.ID {
		t.Errorf("second login created a new account (%s → %s)", userA.ID, userB.ID)
	}
	if len(h.ids.identities) != 1 {
		t.Errorf("identity rows = %d, want 1", len(h.ids.identities))
	}
}

func TestOIDCSyncsRenamedUserFromClaims(t *testing.T) {
	h := newOIDCHarness(t, nil)
	first, err := h.login(t)
	if err != nil {
		t.Fatalf("first login: %v", err)
	}
	if _, _, err := h.svc.ExchangeHandoff(context.Background(), first); err != nil {
		t.Fatalf("first exchange: %v", err)
	}

	// The provider is the source of truth: a rename there must land locally.
	h.idp.setClaims(map[string]any{
		"sub":            "subject-1",
		"email":          "pilot@example.com",
		"email_verified": true,
		"name":           "Amelia M. Earhart",
	})
	second, err := h.login(t)
	if err != nil {
		t.Fatalf("second login: %v", err)
	}
	user, _, err := h.svc.ExchangeHandoff(context.Background(), second)
	if err != nil {
		t.Fatalf("second exchange: %v", err)
	}
	if user.Name != "Amelia M. Earhart" {
		t.Errorf("name = %q, want the updated claim value", user.Name)
	}
}

func TestOIDCRefusesToAdoptExistingAccountByDefault(t *testing.T) {
	h := newOIDCHarness(t, nil)
	// A pre-existing local account with the same address, e.g. left over from
	// before the deployment switched to OIDC.
	if err := h.users.Create(context.Background(), &models.User{
		Email: "pilot@example.com", Name: "Someone Else", PasswordHash: "$2a$12$hash",
	}); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	_, err := h.login(t)
	if !errors.Is(err, service.ErrOIDCEmailConflict) {
		t.Errorf("error = %v, want ErrOIDCEmailConflict", err)
	}
}

func TestOIDCAdoptsExistingAccountWhenLinkingEnabled(t *testing.T) {
	h := newOIDCHarness(t, func(c *service.OIDCConfig) { c.LinkByVerifiedEmail = true })
	seeded := &models.User{Email: "pilot@example.com", Name: "Someone Else", PasswordHash: "$2a$12$hash"}
	if err := h.users.Create(context.Background(), seeded); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	handoff, err := h.login(t)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	user, _, err := h.svc.ExchangeHandoff(context.Background(), handoff)
	if err != nil {
		t.Fatalf("ExchangeHandoff: %v", err)
	}
	if user.ID != seeded.ID {
		t.Errorf("linked to %s, want the pre-existing account %s", user.ID, seeded.ID)
	}
}

func TestOIDCLinkingRequiresVerifiedEmailClaim(t *testing.T) {
	h := newOIDCHarness(t, func(c *service.OIDCConfig) { c.LinkByVerifiedEmail = true })
	if err := h.users.Create(context.Background(), &models.User{
		Email: "pilot@example.com", Name: "Someone Else", PasswordHash: "$2a$12$hash",
	}); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	// Linking is opt-in AND requires the provider to vouch for the address.
	h.idp.setClaims(map[string]any{
		"sub":            "subject-1",
		"email":          "pilot@example.com",
		"email_verified": false,
		"name":           "Impostor",
	})

	_, err := h.login(t)
	if !errors.Is(err, service.ErrOIDCEmailConflict) {
		t.Errorf("error = %v, want ErrOIDCEmailConflict", err)
	}
}

func TestOIDCTrustEmailVerifiedOverride(t *testing.T) {
	h := newOIDCHarness(t, func(c *service.OIDCConfig) { c.TrustEmailVerified = true })
	h.idp.setClaims(map[string]any{
		"sub":   "subject-1",
		"email": "pilot@example.com",
		"name":  "Amelia Earhart",
	})

	handoff, err := h.login(t)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	user, _, err := h.svc.ExchangeHandoff(context.Background(), handoff)
	if err != nil {
		t.Fatalf("ExchangeHandoff: %v", err)
	}
	if !user.EmailVerified {
		t.Error("OIDC_TRUST_EMAIL_VERIFIED should mark the account verified")
	}
}

func TestOIDCUnverifiedEmailStaysUnverified(t *testing.T) {
	h := newOIDCHarness(t, nil)
	h.idp.setClaims(map[string]any{
		"sub":   "subject-1",
		"email": "pilot@example.com",
		"name":  "Amelia Earhart",
	})

	handoff, err := h.login(t)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	user, _, err := h.svc.ExchangeHandoff(context.Background(), handoff)
	if err != nil {
		t.Fatalf("ExchangeHandoff: %v", err)
	}
	if user.EmailVerified {
		t.Error("no email_verified claim should leave the account unverified")
	}
}

func TestOIDCDisabledAccountCannotLogIn(t *testing.T) {
	h := newOIDCHarness(t, nil)
	handoff, err := h.login(t)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	user, _, err := h.svc.ExchangeHandoff(context.Background(), handoff)
	if err != nil {
		t.Fatalf("ExchangeHandoff: %v", err)
	}

	user.Disabled = true
	if err := h.users.Update(context.Background(), user); err != nil {
		t.Fatalf("disable user: %v", err)
	}

	// Refused at provisioning time…
	if _, err := h.login(t); !errors.Is(err, service.ErrAccountDisabled) {
		t.Errorf("callback error = %v, want ErrAccountDisabled", err)
	}
	// …and again at exchange time, for an account disabled mid-flow.
	h.users.users[user.Email].Disabled = false
	handoff, err = h.login(t)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	h.users.users[user.Email].Disabled = true
	if _, _, err := h.svc.ExchangeHandoff(context.Background(), handoff); !errors.Is(err, service.ErrAccountDisabled) {
		t.Errorf("exchange error = %v, want ErrAccountDisabled", err)
	}
}

func TestOIDCHandoffCodeIsSingleUse(t *testing.T) {
	h := newOIDCHarness(t, nil)
	handoff, err := h.login(t)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, _, err := h.svc.ExchangeHandoff(context.Background(), handoff); err != nil {
		t.Fatalf("first exchange: %v", err)
	}
	if _, _, err := h.svc.ExchangeHandoff(context.Background(), handoff); !errors.Is(err, service.ErrOIDCHandoffInvalid) {
		t.Errorf("replayed handoff error = %v, want ErrOIDCHandoffInvalid", err)
	}
}

func TestOIDCHandoffRejectsUnknownAndEmptyCodes(t *testing.T) {
	h := newOIDCHarness(t, nil)
	for _, code := range []string{"", "not-a-real-code"} {
		if _, _, err := h.svc.ExchangeHandoff(context.Background(), code); !errors.Is(err, service.ErrOIDCHandoffInvalid) {
			t.Errorf("code %q: error = %v, want ErrOIDCHandoffInvalid", code, err)
		}
	}
}

func TestOIDCHandoffCodeExpires(t *testing.T) {
	h := newOIDCHarness(t, func(c *service.OIDCConfig) { c.HandoffTTL = time.Nanosecond })
	handoff, err := h.login(t)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, _, err := h.svc.ExchangeHandoff(context.Background(), handoff); !errors.Is(err, service.ErrOIDCHandoffInvalid) {
		t.Errorf("expired handoff error = %v, want ErrOIDCHandoffInvalid", err)
	}
}

func TestOIDCProvisionedUserCannotPasswordLogin(t *testing.T) {
	h := newOIDCHarness(t, nil)
	handoff, err := h.login(t)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	user, _, err := h.svc.ExchangeHandoff(context.Background(), handoff)
	if err != nil {
		t.Fatalf("ExchangeHandoff: %v", err)
	}

	// The empty hash must never authenticate, in any mode.
	for _, password := range []string{"", " ", "password", "$2a$12$hash"} {
		if _, _, err := h.auth.Login(context.Background(), service.LoginInput{
			Email: user.Email, Password: password,
		}); !errors.Is(err, service.ErrInvalidCredentials) {
			t.Errorf("password %q: error = %v, want ErrInvalidCredentials", password, err)
		}
	}
}

func TestOIDCProviderUnavailable(t *testing.T) {
	h := newOIDCHarness(t, func(c *service.OIDCConfig) {
		c.Issuer = "http://127.0.0.1:1/not-a-provider"
	})
	if _, err := h.svc.BeginLogin(context.Background()); !errors.Is(err, service.ErrOIDCProviderUnavailable) {
		t.Errorf("error = %v, want ErrOIDCProviderUnavailable", err)
	}
}

func TestOIDCAuthorizationURLCarriesConfiguredScopes(t *testing.T) {
	h := newOIDCHarness(t, func(c *service.OIDCConfig) {
		c.Scopes = []string{"openid", "email", "groups"}
	})
	auth, err := h.svc.BeginLogin(context.Background())
	if err != nil {
		t.Fatalf("BeginLogin: %v", err)
	}
	u, _ := url.Parse(auth.AuthorizationURL)
	scope := u.Query().Get("scope")
	for _, want := range []string{"openid", "email", "groups"} {
		if !strings.Contains(scope, want) {
			t.Errorf("scope %q missing %q", scope, want)
		}
	}
	if u.Query().Get("redirect_uri") != "https://logbook.example/api/v1/auth/oidc/callback" {
		t.Errorf("redirect_uri = %q", u.Query().Get("redirect_uri"))
	}
}

func TestNewOIDCServiceDisabledWithoutIssuer(t *testing.T) {
	svc, err := service.NewOIDCService(service.OIDCConfig{}, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc != nil {
		t.Error("expected a nil service when OIDC is not configured")
	}
}
