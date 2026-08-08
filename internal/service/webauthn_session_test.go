package service

import (
	"context"
	"encoding/base64"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

// --- fakes -------------------------------------------------------------

// fakeSessionRepo mirrors the Postgres semantics that matter to the service:
// Consume is single-use and gated on both ceremony and expiry.
type fakeSessionRepo struct {
	rows map[string]*models.WebAuthnSession // keyed by hex of IDHash
	now  func() time.Time
}

func newFakeSessionRepo() *fakeSessionRepo {
	return &fakeSessionRepo{rows: map[string]*models.WebAuthnSession{}, now: time.Now}
}

func key(idHash []byte) string { return string(idHash) }

func (f *fakeSessionRepo) Create(_ context.Context, s *models.WebAuthnSession) error {
	if s.CreatedAt.IsZero() {
		s.CreatedAt = f.now()
	}
	cp := *s
	f.rows[key(s.IDHash)] = &cp
	return nil
}

func (f *fakeSessionRepo) Consume(_ context.Context, idHash []byte, ceremony string) (*models.WebAuthnSession, error) {
	row, ok := f.rows[key(idHash)]
	if !ok || row.Ceremony != ceremony || !row.ExpiresAt.After(f.now()) {
		return nil, repository.ErrNotFound
	}
	delete(f.rows, key(idHash))
	return row, nil
}

func (f *fakeSessionRepo) DeleteOldestForUser(_ context.Context, userID uuid.UUID, keep int) (int64, error) {
	var owned []*models.WebAuthnSession
	for _, r := range f.rows {
		if r.UserID != nil && *r.UserID == userID {
			owned = append(owned, r)
		}
	}
	sort.Slice(owned, func(i, j int) bool { return owned[i].CreatedAt.After(owned[j].CreatedAt) })
	var deleted int64
	for _, r := range owned[min(keep, len(owned)):] {
		delete(f.rows, key(r.IDHash))
		deleted++
	}
	return deleted, nil
}

func (f *fakeSessionRepo) DeleteExpired(_ context.Context) (int64, error) {
	var deleted int64
	for k, r := range f.rows {
		if !r.ExpiresAt.After(f.now()) {
			delete(f.rows, k)
			deleted++
		}
	}
	return deleted, nil
}

func (f *fakeSessionRepo) count() int { return len(f.rows) }

type fakeUserRepoWA struct{ users map[uuid.UUID]*models.User }

func (f *fakeUserRepoWA) Create(context.Context, *models.User) error { return nil }
func (f *fakeUserRepoWA) GetByID(_ context.Context, id uuid.UUID) (*models.User, error) {
	u, ok := f.users[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return u, nil
}
func (f *fakeUserRepoWA) GetByEmail(_ context.Context, email string) (*models.User, error) {
	for _, u := range f.users {
		if strings.EqualFold(u.Email, email) {
			return u, nil
		}
	}
	return nil, repository.ErrNotFound
}
func (f *fakeUserRepoWA) Update(context.Context, *models.User) error                    { return nil }
func (f *fakeUserRepoWA) Delete(context.Context, uuid.UUID) error                       { return nil }
func (f *fakeUserRepoWA) IncrementFailedLoginAttempts(context.Context, uuid.UUID) error { return nil }
func (f *fakeUserRepoWA) ResetFailedLoginAttempts(context.Context, uuid.UUID) error     { return nil }
func (f *fakeUserRepoWA) LockAccount(context.Context, uuid.UUID, time.Time) error       { return nil }
func (f *fakeUserRepoWA) ConsumeRecoveryCode(context.Context, uuid.UUID, string) (bool, error) {
	return false, nil
}
func (f *fakeUserRepoWA) MarkEmailVerified(context.Context, uuid.UUID) error { return nil }
func (f *fakeUserRepoWA) UpdateLastLogin(context.Context, uuid.UUID, time.Time) error {
	return nil
}

type fakeCredRepo struct{}

func (fakeCredRepo) Create(context.Context, *models.WebAuthnCredential) error { return nil }
func (fakeCredRepo) GetByID(context.Context, uuid.UUID) (*models.WebAuthnCredential, error) {
	return nil, repository.ErrNotFound
}
func (fakeCredRepo) GetByCredentialID(context.Context, []byte) (*models.WebAuthnCredential, error) {
	return nil, repository.ErrNotFound
}
func (fakeCredRepo) GetByUserID(context.Context, uuid.UUID) ([]*models.WebAuthnCredential, error) {
	return nil, nil
}
func (fakeCredRepo) UpdateSignCount(context.Context, uuid.UUID, uint32, time.Time) error {
	return nil
}
func (fakeCredRepo) Delete(context.Context, uuid.UUID, uuid.UUID) error { return nil }

// --- harness -----------------------------------------------------------

func newTestWebAuthnService(t *testing.T, sessions *fakeSessionRepo, maxOpen int, users ...*models.User) *WebAuthnService {
	t.Helper()
	byID := map[uuid.UUID]*models.User{}
	for _, u := range users {
		byID[u.ID] = u
	}
	svc, err := NewWebAuthnService(
		"localhost", "NinerLog", []string{"http://localhost:5173"},
		fakeCredRepo{}, sessions, &fakeUserRepoWA{users: byID}, nil,
		5*time.Minute, maxOpen,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

func testUser(email string) *models.User {
	return &models.User{ID: uuid.New(), Email: email, Name: "Test Pilot"}
}

// garbage is a syntactically valid JSON body that is not a credential
// response. Reaching the parser means session handling already succeeded, so
// ErrWebAuthnInvalidResponse is the "session accepted" signal in these tests.
var garbage = []byte(`{"id":"nope"}`)

// --- tests -------------------------------------------------------------

func TestWebAuthnHandleIsUnguessableAndHashedAtRest(t *testing.T) {
	sessions := newFakeSessionRepo()
	user := testUser("handle@example.com")
	svc := newTestWebAuthnService(t, sessions, 10, user)

	handle, _, err := svc.BeginRegistration(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("begin registration: %v", err)
	}

	raw, err := base64.RawURLEncoding.DecodeString(handle)
	if err != nil {
		t.Fatalf("handle is not base64url: %v", err)
	}
	if len(raw) != webauthnHandleBytes {
		t.Errorf("expected %d bytes of entropy, got %d", webauthnHandleBytes, len(raw))
	}

	// The stored key must be the hash, never the handle itself.
	for k, row := range sessions.rows {
		if k == handle {
			t.Fatal("raw handle used as the storage key")
		}
		if string(row.IDHash) != string(hashWebAuthnHandle(handle)) {
			t.Error("stored id_hash is not sha256(handle)")
		}
	}
}

func TestWebAuthnHandlesAreUnique(t *testing.T) {
	sessions := newFakeSessionRepo()
	user := testUser("unique@example.com")
	svc := newTestWebAuthnService(t, sessions, 100, user)

	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		h, _, err := svc.BeginRegistration(context.Background(), user.ID)
		if err != nil {
			t.Fatalf("begin registration: %v", err)
		}
		if seen[h] {
			t.Fatalf("duplicate handle issued: %s", h)
		}
		seen[h] = true
	}
}

func TestWebAuthnSessionIsSingleUse(t *testing.T) {
	sessions := newFakeSessionRepo()
	user := testUser("singleuse@example.com")
	svc := newTestWebAuthnService(t, sessions, 10, user)
	ctx := context.Background()

	handle, _, err := svc.BeginRegistration(ctx, user.ID)
	if err != nil {
		t.Fatalf("begin registration: %v", err)
	}

	// First use gets past session handling and fails at attestation parsing.
	if _, err := svc.FinishRegistration(ctx, user.ID, handle, nil, garbage); !isInvalidResponse(err) {
		t.Fatalf("expected the session to be accepted on first use, got %v", err)
	}
	// Replay must be rejected as an unknown session, not re-parsed.
	if _, err := svc.FinishRegistration(ctx, user.ID, handle, nil, garbage); err != ErrWebAuthnSessionNotFound {
		t.Errorf("expected replay to be rejected, got %v", err)
	}
}

func TestWebAuthnRegistrationHandleRejectedByLogin(t *testing.T) {
	sessions := newFakeSessionRepo()
	user := testUser("ceremony@example.com")
	svc := newTestWebAuthnService(t, sessions, 10, user)
	ctx := context.Background()

	handle, _, err := svc.BeginRegistration(ctx, user.ID)
	if err != nil {
		t.Fatalf("begin registration: %v", err)
	}

	if _, _, err := svc.FinishLogin(ctx, handle, garbage); err != ErrWebAuthnSessionNotFound {
		t.Errorf("registration handle must not satisfy a login finish, got %v", err)
	}
	// The mismatched attempt must not have burned the registration session.
	if _, err := svc.FinishRegistration(ctx, user.ID, handle, nil, garbage); !isInvalidResponse(err) {
		t.Errorf("registration session should still be usable, got %v", err)
	}
}

func TestWebAuthnRegistrationSessionIsScopedToItsUser(t *testing.T) {
	sessions := newFakeSessionRepo()
	owner := testUser("owner@example.com")
	attacker := testUser("attacker@example.com")
	svc := newTestWebAuthnService(t, sessions, 10, owner, attacker)
	ctx := context.Background()

	handle, _, err := svc.BeginRegistration(ctx, owner.ID)
	if err != nil {
		t.Fatalf("begin registration: %v", err)
	}

	// A stolen handle must not let another account attach a credential.
	if _, err := svc.FinishRegistration(ctx, attacker.ID, handle, nil, garbage); err != ErrWebAuthnSessionNotFound {
		t.Errorf("expected uniform rejection for a mismatched user, got %v", err)
	}
}

func TestWebAuthnCorruptSessionDataIsRejected(t *testing.T) {
	sessions := newFakeSessionRepo()
	user := testUser("corrupt@example.com")
	svc := newTestWebAuthnService(t, sessions, 10, user)
	ctx := context.Background()

	handle, _, err := svc.BeginRegistration(ctx, user.ID)
	if err != nil {
		t.Fatalf("begin registration: %v", err)
	}
	// Corrupt the stored payload behind the service's back.
	sessions.rows[key(hashWebAuthnHandle(handle))].Data = []byte(`{not json`)

	// Must be rejected as an unusable session rather than panicking or
	// surfacing a decode error the caller cannot act on.
	if _, err := svc.FinishRegistration(ctx, user.ID, handle, nil, garbage); err != ErrWebAuthnSessionNotFound {
		t.Errorf("expected corrupt session to be rejected, got %v", err)
	}
}

func TestWebAuthnUnknownAndEmptyHandlesRejectedUniformly(t *testing.T) {
	sessions := newFakeSessionRepo()
	user := testUser("unknown@example.com")
	svc := newTestWebAuthnService(t, sessions, 10, user)
	ctx := context.Background()

	for _, handle := range []string{"", "not-a-real-handle", base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef"))} {
		if _, err := svc.FinishRegistration(ctx, user.ID, handle, nil, garbage); err != ErrWebAuthnSessionNotFound {
			t.Errorf("handle %q: expected ErrWebAuthnSessionNotFound, got %v", handle, err)
		}
	}
}

func TestWebAuthnExpiredSessionIsRejected(t *testing.T) {
	sessions := newFakeSessionRepo()
	user := testUser("expired@example.com")
	svc := newTestWebAuthnService(t, sessions, 10, user)
	ctx := context.Background()

	handle, _, err := svc.BeginRegistration(ctx, user.ID)
	if err != nil {
		t.Fatalf("begin registration: %v", err)
	}
	// Expire the row without running any cleanup: rejection must come from the
	// expiry check, not from the row having been swept away.
	sessions.rows[key(hashWebAuthnHandle(handle))].ExpiresAt = time.Now().Add(-time.Second)

	if _, err := svc.FinishRegistration(ctx, user.ID, handle, nil, garbage); err != ErrWebAuthnSessionNotFound {
		t.Errorf("expected expired session to be rejected, got %v", err)
	}
	if sessions.count() != 1 {
		t.Error("expired row should still be present — rejection must not depend on cleanup")
	}
}

// Two ceremonies open at once for one user must both work, in either order.
func TestWebAuthnConcurrentCeremoniesForOneUser(t *testing.T) {
	for _, order := range []string{"first-then-second", "second-then-first"} {
		t.Run(order, func(t *testing.T) {
			sessions := newFakeSessionRepo()
			user := testUser("concurrent@example.com")
			svc := newTestWebAuthnService(t, sessions, 10, user)
			ctx := context.Background()

			first, _, err := svc.BeginRegistration(ctx, user.ID)
			if err != nil {
				t.Fatalf("begin first: %v", err)
			}
			second, _, err := svc.BeginRegistration(ctx, user.ID)
			if err != nil {
				t.Fatalf("begin second: %v", err)
			}
			if first == second {
				t.Fatal("second ceremony reused the first handle")
			}

			handles := []string{first, second}
			if order == "second-then-first" {
				handles = []string{second, first}
			}
			for i, h := range handles {
				if _, err := svc.FinishRegistration(ctx, user.ID, h, nil, garbage); !isInvalidResponse(err) {
					t.Errorf("ceremony %d should have been accepted, got %v", i, err)
				}
			}
		})
	}
}

func TestWebAuthnRegistrationAndLoginDoNotInterfere(t *testing.T) {
	sessions := newFakeSessionRepo()
	user := testUser("mixed@example.com")
	svc := newTestWebAuthnService(t, sessions, 10, user)
	ctx := context.Background()

	regHandle, _, err := svc.BeginRegistration(ctx, user.ID)
	if err != nil {
		t.Fatalf("begin registration: %v", err)
	}
	loginHandle, _, err := svc.BeginLogin(ctx, user.Email)
	if err != nil {
		t.Fatalf("begin login: %v", err)
	}
	if regHandle == loginHandle {
		t.Fatal("login ceremony reused the registration handle")
	}

	if _, _, err := svc.FinishLogin(ctx, loginHandle, garbage); !isInvalidResponse(err) {
		t.Errorf("login session should have been accepted, got %v", err)
	}
	if _, err := svc.FinishRegistration(ctx, user.ID, regHandle, nil, garbage); !isInvalidResponse(err) {
		t.Errorf("registration session should have been accepted, got %v", err)
	}
}

func TestWebAuthnOpenCeremoniesAreBoundedEvictingOldest(t *testing.T) {
	const maxOpen = 3
	sessions := newFakeSessionRepo()
	user := testUser("cap@example.com")
	svc := newTestWebAuthnService(t, sessions, maxOpen, user)
	ctx := context.Background()

	// Distinct creation times so "oldest" is well defined.
	base := time.Now().Add(-time.Hour)
	var issued []string
	for i := 0; i < maxOpen+1; i++ {
		sessions.now = func() time.Time { return base.Add(time.Duration(len(issued)) * time.Minute) }
		h, _, err := svc.BeginRegistration(ctx, user.ID)
		if err != nil {
			t.Fatalf("begin %d: %v", i, err)
		}
		issued = append(issued, h)
	}
	sessions.now = time.Now

	if got := sessions.count(); got != maxOpen {
		t.Errorf("expected open ceremonies capped at %d, got %d", maxOpen, got)
	}
	// The user's most recent attempt must always survive.
	newest := issued[len(issued)-1]
	if _, err := svc.FinishRegistration(ctx, user.ID, newest, nil, garbage); !isInvalidResponse(err) {
		t.Errorf("newest ceremony must survive the cap, got %v", err)
	}
	// The oldest was evicted rather than the newest rejected.
	if _, err := svc.FinishRegistration(ctx, user.ID, issued[0], nil, garbage); err != ErrWebAuthnSessionNotFound {
		t.Errorf("oldest ceremony should have been evicted, got %v", err)
	}
}

func TestWebAuthnEvictionIsScopedToOneUser(t *testing.T) {
	sessions := newFakeSessionRepo()
	victim := testUser("victim@example.com")
	noisy := testUser("noisy@example.com")
	svc := newTestWebAuthnService(t, sessions, 1, victim, noisy)
	ctx := context.Background()

	victimHandle, _, err := svc.BeginRegistration(ctx, victim.ID)
	if err != nil {
		t.Fatalf("begin victim: %v", err)
	}
	// A second user burning through ceremonies must not evict the first's.
	for i := 0; i < 5; i++ {
		if _, _, err := svc.BeginRegistration(ctx, noisy.ID); err != nil {
			t.Fatalf("begin noisy %d: %v", i, err)
		}
	}

	if _, err := svc.FinishRegistration(ctx, victim.ID, victimHandle, nil, garbage); !isInvalidResponse(err) {
		t.Errorf("another user's ceremonies evicted this one, got %v", err)
	}
}

func TestWebAuthnDiscoverableLoginSessionsAreNotUserScoped(t *testing.T) {
	sessions := newFakeSessionRepo()
	svc := newTestWebAuthnService(t, sessions, 10)
	ctx := context.Background()

	// No email → discoverable login, which has no user at begin time.
	handle, _, err := svc.BeginLogin(ctx, "")
	if err != nil {
		t.Fatalf("begin discoverable login: %v", err)
	}
	row, ok := sessions.rows[key(hashWebAuthnHandle(handle))]
	if !ok {
		t.Fatal("discoverable session was not stored")
	}
	if row.UserID != nil {
		t.Errorf("expected nil user id for discoverable login, got %v", *row.UserID)
	}
	if row.Ceremony != models.WebAuthnCeremonyLogin {
		t.Errorf("expected login ceremony, got %q", row.Ceremony)
	}
}

// isInvalidResponse reports whether the service got past session handling and
// failed on the credential payload instead.
func isInvalidResponse(err error) bool {
	return err != nil && strings.Contains(err.Error(), ErrWebAuthnInvalidResponse.Error())
}

// Unverified-account reaping is exercised in the reaper's own tests; this mock
// only needs the interface satisfied.
func (f *fakeUserRepoWA) ListUnverifiedForReminder(_ context.Context, _ time.Time, _ int) ([]*models.User, error) {
	return nil, nil
}

func (f *fakeUserRepoWA) MarkVerificationReminderSent(_ context.Context, _ uuid.UUID, _ time.Time) error {
	return nil
}

func (f *fakeUserRepoWA) DeleteUnverifiedRemindedBefore(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}
