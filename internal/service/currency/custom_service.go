package currency

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

// ValidationError marks a user-fixable problem with a rule definition or its
// metadata. Handlers surface its message and map it to HTTP 400.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func newValidationError(msg string) *ValidationError { return &ValidationError{Msg: msg} }

// CustomService orchestrates persistence and evaluation of user-authored
// currency rules, including opt-in sharing and importing.
type CustomService struct {
	repo repository.CustomCurrencyRuleRepository
	eval *CustomEvaluator
}

// NewCustomService creates a custom currency service.
func NewCustomService(repo repository.CustomCurrencyRuleRepository, eval *CustomEvaluator) *CustomService {
	return &CustomService{repo: repo, eval: eval}
}

// CustomRuleInput carries the writable fields of a rule from the API layer.
type CustomRuleInput struct {
	Name        string
	Description *string
	Emoji       *string
	Definition  models.CustomCurrencyRuleBody
}

// CustomRuleWithStatus bundles a stored rule with its current evaluation. The
// rule carries the definition (for editing); the evaluation carries status and
// per-requirement progress (for display).
type CustomRuleWithStatus struct {
	Rule       *models.CustomCurrencyRule `json:"rule"`
	Evaluation CustomCurrencyResult       `json:"evaluation"`
}

// SharedRuleView is the read-only projection of a shared rule shown to a user
// following a share link. It deliberately omits owner identity.
type SharedRuleView struct {
	Name        string                        `json:"name"`
	Description *string                       `json:"description,omitempty"`
	Emoji       *string                       `json:"emoji,omitempty"`
	Definition  models.CustomCurrencyRuleBody `json:"definition"`
	ShareToken  string                        `json:"shareToken"`
}

const maxRuleNameLen = 120

// validateInput normalizes and validates rule metadata and definition.
func validateInput(in *CustomRuleInput) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return newValidationError("a rule needs a name")
	}
	if len(in.Name) > maxRuleNameLen {
		return newValidationError("name is too long")
	}
	if err := in.Definition.Validate(); err != nil {
		return newValidationError(err.Error())
	}
	return nil
}

// List returns all of a user's rules with their evaluations.
func (s *CustomService) List(ctx context.Context, userID uuid.UUID) ([]CustomRuleWithStatus, error) {
	rules, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]CustomRuleWithStatus, 0, len(rules))
	for _, rule := range rules {
		eval, err := s.eval.Evaluate(ctx, userID, &rule.Definition)
		if err != nil {
			return nil, err
		}
		out = append(out, CustomRuleWithStatus{Rule: rule, Evaluation: eval})
	}
	return out, nil
}

// Get returns one rule owned by the user, or repository.ErrNotFound.
func (s *CustomService) Get(ctx context.Context, userID, id uuid.UUID) (*CustomRuleWithStatus, error) {
	rule, err := s.ownedRule(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	eval, err := s.eval.Evaluate(ctx, userID, &rule.Definition)
	if err != nil {
		return nil, err
	}
	return &CustomRuleWithStatus{Rule: rule, Evaluation: eval}, nil
}

// Create validates and persists a new rule, returning it with its evaluation.
func (s *CustomService) Create(ctx context.Context, userID uuid.UUID, in CustomRuleInput) (*CustomRuleWithStatus, error) {
	if err := validateInput(&in); err != nil {
		return nil, err
	}
	rule := &models.CustomCurrencyRule{
		UserID:      userID,
		Name:        in.Name,
		Description: in.Description,
		Emoji:       in.Emoji,
		Definition:  in.Definition,
	}
	if err := s.repo.Create(ctx, rule); err != nil {
		return nil, err
	}
	eval, err := s.eval.Evaluate(ctx, userID, &rule.Definition)
	if err != nil {
		return nil, err
	}
	return &CustomRuleWithStatus{Rule: rule, Evaluation: eval}, nil
}

// Update validates and saves changes to an owned rule.
func (s *CustomService) Update(ctx context.Context, userID, id uuid.UUID, in CustomRuleInput) (*CustomRuleWithStatus, error) {
	if err := validateInput(&in); err != nil {
		return nil, err
	}
	rule, err := s.ownedRule(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	rule.Name = in.Name
	rule.Description = in.Description
	rule.Emoji = in.Emoji
	rule.Definition = in.Definition
	if err := s.repo.Update(ctx, rule); err != nil {
		return nil, err
	}
	eval, err := s.eval.Evaluate(ctx, userID, &rule.Definition)
	if err != nil {
		return nil, err
	}
	return &CustomRuleWithStatus{Rule: rule, Evaluation: eval}, nil
}

// Delete removes an owned rule.
func (s *CustomService) Delete(ctx context.Context, userID, id uuid.UUID) error {
	if _, err := s.ownedRule(ctx, userID, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

// Preview validates and evaluates an unsaved definition without persisting it,
// powering the live preview in the builder.
func (s *CustomService) Preview(ctx context.Context, userID uuid.UUID, body models.CustomCurrencyRuleBody) (CustomCurrencyResult, error) {
	if err := body.Validate(); err != nil {
		return CustomCurrencyResult{}, newValidationError(err.Error())
	}
	return s.eval.Evaluate(ctx, userID, &body)
}

// SetShared toggles sharing for an owned rule. Enabling generates a share token
// if one does not already exist; disabling clears it.
func (s *CustomService) SetShared(ctx context.Context, userID, id uuid.UUID, shared bool) (*models.CustomCurrencyRule, error) {
	rule, err := s.ownedRule(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if shared {
		rule.IsShared = true
		if rule.ShareToken == nil || *rule.ShareToken == "" {
			token, err := generateShareToken()
			if err != nil {
				return nil, err
			}
			rule.ShareToken = &token
		}
	} else {
		rule.IsShared = false
		rule.ShareToken = nil
	}
	if err := s.repo.Update(ctx, rule); err != nil {
		return nil, err
	}
	return rule, nil
}

// GetShared returns the read-only view of a shared rule by token.
func (s *CustomService) GetShared(ctx context.Context, token string) (*SharedRuleView, error) {
	rule, err := s.repo.GetByShareToken(ctx, token)
	if err != nil {
		return nil, err
	}
	return &SharedRuleView{
		Name:        rule.Name,
		Description: rule.Description,
		Emoji:       rule.Emoji,
		Definition:  rule.Definition,
		ShareToken:  token,
	}, nil
}

// Import copies a shared rule into the caller's account, recording provenance.
// A user cannot import their own rule.
func (s *CustomService) Import(ctx context.Context, userID uuid.UUID, token string) (*CustomRuleWithStatus, error) {
	source, err := s.repo.GetByShareToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if source.UserID == userID {
		return nil, newValidationError("you already own this rule")
	}
	copyRule := &models.CustomCurrencyRule{
		UserID:       userID,
		Name:         source.Name,
		Description:  source.Description,
		Emoji:        source.Emoji,
		Definition:   source.Definition,
		ImportedFrom: &source.ID,
	}
	if err := s.repo.Create(ctx, copyRule); err != nil {
		return nil, err
	}
	eval, err := s.eval.Evaluate(ctx, userID, &copyRule.Definition)
	if err != nil {
		return nil, err
	}
	return &CustomRuleWithStatus{Rule: copyRule, Evaluation: eval}, nil
}

// ownedRule fetches a rule and enforces ownership. To avoid leaking the
// existence of other users' rules, a non-owner sees repository.ErrNotFound.
func (s *CustomService) ownedRule(ctx context.Context, userID, id uuid.UUID) (*models.CustomCurrencyRule, error) {
	rule, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if rule.UserID != userID {
		return nil, repository.ErrNotFound
	}
	return rule, nil
}

// generateShareToken returns a URL-safe, unguessable share token.
func generateShareToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// IsValidationError reports whether err is (or wraps) a ValidationError.
func IsValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}
