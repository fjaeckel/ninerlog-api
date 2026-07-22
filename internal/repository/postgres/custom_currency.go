package postgres

import (
	"context"
	"database/sql"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

type customCurrencyRuleRepository struct {
	db *sql.DB
}

// NewCustomCurrencyRuleRepository creates a Postgres-backed repository for
// user-authored currency rules.
func NewCustomCurrencyRuleRepository(db *sql.DB) repository.CustomCurrencyRuleRepository {
	return &customCurrencyRuleRepository{db: db}
}

func (r *customCurrencyRuleRepository) Create(ctx context.Context, rule *models.CustomCurrencyRule) error {
	query := `
		INSERT INTO custom_currency_rules
			(user_id, name, description, emoji, definition, enabled, notify, is_shared, share_token, imported_from)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at, updated_at
	`
	return r.db.QueryRowContext(ctx, query,
		rule.UserID,
		rule.Name,
		rule.Description,
		rule.Emoji,
		rule.Definition,
		rule.Enabled,
		rule.Notify,
		rule.IsShared,
		rule.ShareToken,
		rule.ImportedFrom,
	).Scan(&rule.ID, &rule.CreatedAt, &rule.UpdatedAt)
}

const customCurrencyColumns = `
	id, user_id, name, description, emoji, definition,
	enabled, notify, is_shared, share_token, imported_from, created_at, updated_at
`

func scanCustomCurrencyRule(s interface {
	Scan(dest ...interface{}) error
}) (*models.CustomCurrencyRule, error) {
	rule := &models.CustomCurrencyRule{}
	err := s.Scan(
		&rule.ID, &rule.UserID, &rule.Name, &rule.Description, &rule.Emoji,
		&rule.Definition, &rule.Enabled, &rule.Notify, &rule.IsShared, &rule.ShareToken, &rule.ImportedFrom,
		&rule.CreatedAt, &rule.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return rule, nil
}

func (r *customCurrencyRuleRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.CustomCurrencyRule, error) {
	query := `SELECT ` + customCurrencyColumns + ` FROM custom_currency_rules WHERE id = $1`
	rule, err := scanCustomCurrencyRule(r.db.QueryRowContext(ctx, query, id))
	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound
	}
	return rule, err
}

func (r *customCurrencyRuleRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*models.CustomCurrencyRule, error) {
	query := `SELECT ` + customCurrencyColumns + ` FROM custom_currency_rules WHERE user_id = $1 ORDER BY created_at ASC`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*models.CustomCurrencyRule
	for rows.Next() {
		rule, err := scanCustomCurrencyRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (r *customCurrencyRuleRepository) GetByShareToken(ctx context.Context, token string) (*models.CustomCurrencyRule, error) {
	query := `SELECT ` + customCurrencyColumns + ` FROM custom_currency_rules WHERE share_token = $1 AND is_shared = true`
	rule, err := scanCustomCurrencyRule(r.db.QueryRowContext(ctx, query, token))
	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound
	}
	return rule, err
}

func (r *customCurrencyRuleRepository) Update(ctx context.Context, rule *models.CustomCurrencyRule) error {
	query := `
		UPDATE custom_currency_rules
		SET name = $1, description = $2, emoji = $3, definition = $4,
		    enabled = $5, notify = $6, is_shared = $7, share_token = $8, updated_at = NOW()
		WHERE id = $9
	`
	res, err := r.db.ExecContext(ctx, query,
		rule.Name, rule.Description, rule.Emoji, rule.Definition,
		rule.Enabled, rule.Notify, rule.IsShared, rule.ShareToken, rule.ID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *customCurrencyRuleRepository) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM custom_currency_rules WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return repository.ErrNotFound
	}
	return nil
}
