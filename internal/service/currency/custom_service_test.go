package currency

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

// mockCustomRepo is an in-memory CustomCurrencyRuleRepository for exercising the
// service's authorization, quota, and sharing logic without a database. Paths
// under test short-circuit before any flight evaluation, so the evaluator's DB
// is never touched.
type mockCustomRepo struct {
	rules map[uuid.UUID]*models.CustomCurrencyRule
}

func newMockRepo() *mockCustomRepo {
	return &mockCustomRepo{rules: map[uuid.UUID]*models.CustomCurrencyRule{}}
}

func (m *mockCustomRepo) Create(_ context.Context, r *models.CustomCurrencyRule) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	cp := *r
	m.rules[r.ID] = &cp
	return nil
}
func (m *mockCustomRepo) GetByID(_ context.Context, id uuid.UUID) (*models.CustomCurrencyRule, error) {
	r, ok := m.rules[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	cp := *r
	return &cp, nil
}
func (m *mockCustomRepo) GetByUserID(_ context.Context, userID uuid.UUID) ([]*models.CustomCurrencyRule, error) {
	var out []*models.CustomCurrencyRule
	for _, r := range m.rules {
		if r.UserID == userID {
			cp := *r
			out = append(out, &cp)
		}
	}
	return out, nil
}
func (m *mockCustomRepo) GetByShareToken(_ context.Context, token string) (*models.CustomCurrencyRule, error) {
	for _, r := range m.rules {
		if r.IsShared && r.ShareToken != nil && *r.ShareToken == token {
			cp := *r
			return &cp, nil
		}
	}
	return nil, repository.ErrNotFound
}
func (m *mockCustomRepo) Update(_ context.Context, r *models.CustomCurrencyRule) error {
	if _, ok := m.rules[r.ID]; !ok {
		return repository.ErrNotFound
	}
	cp := *r
	m.rules[r.ID] = &cp
	return nil
}
func (m *mockCustomRepo) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := m.rules[id]; !ok {
		return repository.ErrNotFound
	}
	delete(m.rules, id)
	return nil
}

func newServiceWithMock(repo *mockCustomRepo) *CustomService {
	// nil DB is safe: the tests here never reach flight evaluation.
	return NewCustomService(repo, NewCustomEvaluator(nil))
}

func seedRule(repo *mockCustomRepo, owner uuid.UUID, shared bool, token string) *models.CustomCurrencyRule {
	r := &models.CustomCurrencyRule{
		ID:     uuid.New(),
		UserID: owner,
		Name:   "Rule",
		Definition: models.CustomCurrencyRuleBody{
			Window:       models.CurrencyWindow{Amount: 90, Unit: "days"},
			Requirements: []models.CurrencyRequirement{{Metric: "landings", Min: 3}},
		},
		IsShared: shared,
	}
	if token != "" {
		r.ShareToken = &token
	}
	repo.rules[r.ID] = r
	return r
}

func validInput() CustomRuleInput {
	return CustomRuleInput{
		Name: "My rule",
		Definition: models.CustomCurrencyRuleBody{
			Window:       models.CurrencyWindow{Amount: 90, Unit: "days"},
			Requirements: []models.CurrencyRequirement{{Metric: "landings", Min: 3}},
		},
	}
}

func TestService_IDOR_NonOwnerCannotAccess(t *testing.T) {
	repo := newMockRepo()
	svc := newServiceWithMock(repo)
	owner, attacker := uuid.New(), uuid.New()
	rule := seedRule(repo, owner, false, "")

	ctx := context.Background()
	if _, err := svc.Get(ctx, attacker, rule.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Get by non-owner: got %v, want ErrNotFound", err)
	}
	if _, err := svc.Update(ctx, attacker, rule.ID, validInput()); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Update by non-owner: got %v, want ErrNotFound", err)
	}
	if err := svc.Delete(ctx, attacker, rule.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Delete by non-owner: got %v, want ErrNotFound", err)
	}
	if _, err := svc.SetShared(ctx, attacker, rule.ID, true); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("SetShared by non-owner: got %v, want ErrNotFound", err)
	}
	// The rule must be untouched by the attacker's share attempt.
	if repo.rules[rule.ID].IsShared {
		t.Error("attacker's SetShared must not have modified the rule")
	}
}

func TestService_ShareTokenLifecycle(t *testing.T) {
	repo := newMockRepo()
	svc := newServiceWithMock(repo)
	owner := uuid.New()
	rule := seedRule(repo, owner, false, "")
	ctx := context.Background()

	shared, err := svc.SetShared(ctx, owner, rule.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if !shared.IsShared || shared.ShareToken == nil || *shared.ShareToken == "" {
		t.Fatalf("enabling share should set a token, got %+v", shared)
	}
	token := *shared.ShareToken

	// Shared rule is resolvable by token, and exposes no owner identity.
	view, err := svc.GetShared(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if view.ShareToken != token {
		t.Errorf("shared view token mismatch")
	}

	// Disabling share must clear the token so the old link stops resolving.
	unshared, err := svc.SetShared(ctx, owner, rule.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if unshared.IsShared || unshared.ShareToken != nil {
		t.Errorf("disabling share should clear the token, got %+v", unshared)
	}
	if _, err := svc.GetShared(ctx, token); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("revoked token should no longer resolve, got %v", err)
	}
}

func TestService_SetEnabledTogglesAndSkipsEvaluation(t *testing.T) {
	repo := newMockRepo()
	// Evaluator with a nil DB: if a disabled rule were evaluated it would panic,
	// proving disabled rules are not evaluated.
	svc := newServiceWithMock(repo)
	owner := uuid.New()
	rule := seedRule(repo, owner, false, "")
	rule.Enabled = true
	repo.rules[rule.ID] = rule
	ctx := context.Background()

	res, err := svc.SetEnabled(ctx, owner, rule.ID, false)
	if err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if res.Rule.Enabled {
		t.Error("rule should be disabled")
	}
	if res.Evaluation.Status != StatusUnknown {
		t.Errorf("disabled rule status = %v, want unknown", res.Evaluation.Status)
	}
	if len(res.Evaluation.Requirements) != 0 {
		t.Error("disabled rule should not carry requirement progress")
	}
	if res.Evaluation.WindowLabel == "" {
		t.Error("disabled rule should still carry its window label")
	}

	// List must also skip evaluation for the now-disabled rule (nil DB, no panic).
	list, err := svc.List(ctx, owner)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Evaluation.Status != StatusUnknown {
		t.Errorf("List did not treat disabled rule as paused: %+v", list)
	}
}

func TestService_SetEnabledNonOwnerDenied(t *testing.T) {
	repo := newMockRepo()
	svc := newServiceWithMock(repo)
	owner, attacker := uuid.New(), uuid.New()
	rule := seedRule(repo, owner, false, "")
	if _, err := svc.SetEnabled(context.Background(), attacker, rule.ID, false); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("non-owner SetEnabled: got %v, want ErrNotFound", err)
	}
}

func TestService_CannotImportOwnRule(t *testing.T) {
	repo := newMockRepo()
	svc := newServiceWithMock(repo)
	owner := uuid.New()
	seedRule(repo, owner, true, "tok-abc")

	if _, err := svc.Import(context.Background(), owner, "tok-abc"); err == nil || !IsValidationError(err) {
		t.Errorf("importing own rule should be a validation error, got %v", err)
	}
}

func TestService_MetadataLengthLimits(t *testing.T) {
	repo := newMockRepo()
	svc := newServiceWithMock(repo)
	ctx := context.Background()
	owner := uuid.New()

	longDesc := strings.Repeat("x", maxRuleDescLen+1)
	in := validInput()
	in.Description = &longDesc
	if _, err := svc.Create(ctx, owner, in); !IsValidationError(err) {
		t.Errorf("over-long description should be rejected, got %v", err)
	}

	longEmoji := strings.Repeat("🙂", maxRuleEmojiLen+1)
	in2 := validInput()
	in2.Emoji = &longEmoji
	if _, err := svc.Create(ctx, owner, in2); !IsValidationError(err) {
		t.Errorf("over-long emoji should be rejected, got %v", err)
	}
}

func TestService_QuotaEnforced(t *testing.T) {
	repo := newMockRepo()
	svc := newServiceWithMock(repo)
	owner := uuid.New()
	for i := 0; i < maxRulesPerUser; i++ {
		seedRule(repo, owner, false, "")
	}
	if _, err := svc.Create(context.Background(), owner, validInput()); !IsValidationError(err) {
		t.Errorf("creating past the quota should be rejected, got %v", err)
	}
}
