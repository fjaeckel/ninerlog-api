package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// contactColumns is the column list every contact read shares, so a new column
// cannot be added to one query and forgotten in the next.
const contactColumns = `id, user_id, name, email, phone, notes, created_at, updated_at`

type ContactRepository struct {
	db *sql.DB
}

func NewContactRepository(db *sql.DB) *ContactRepository {
	return &ContactRepository{db: db}
}

// scanContact reads one contact row in contactColumns order.
func scanContact(row interface{ Scan(...any) error }) (*models.Contact, error) {
	c := &models.Contact{}
	if err := row.Scan(&c.ID, &c.UserID, &c.Name, &c.Email, &c.Phone, &c.Notes, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	return c, nil
}

// isDuplicateName reports whether err is the unique violation raised by
// idx_contacts_user_lower_name (migration 000060).
func isDuplicateName(err error) bool {
	var pqErr *pq.Error
	// 23505 = unique_violation.
	return errors.As(err, &pqErr) && pqErr.Code == "23505" &&
		pqErr.Constraint == "idx_contacts_user_lower_name"
}

func (r *ContactRepository) Create(ctx context.Context, contact *models.Contact) error {
	query := `
		INSERT INTO contacts (id, user_id, name, email, phone, notes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	contact.ID = uuid.New()
	now := time.Now()
	contact.CreatedAt = now
	contact.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, query,
		contact.ID, contact.UserID, contact.Name, contact.Email, contact.Phone, contact.Notes,
		contact.CreatedAt, contact.UpdatedAt,
	)
	if isDuplicateName(err) {
		return repository.ErrDuplicate
	}
	return err
}

// FindOrCreateByName returns the user's contact with this name, creating it if
// there is none, and reports whether it created one.
//
// The read-then-insert is deliberately racy-tolerant rather than racy: two
// concurrent crew writes for the same new name both miss the SELECT, one
// INSERT wins, and the loser's ON CONFLICT DO NOTHING returns no row and falls
// through to the second lookup. Without idx_contacts_user_lower_name backing
// the conflict target this collapses back into a plain read-then-write and
// duplicates reappear.
//
// The caller is responsible for trimming; the name is matched and stored
// verbatim.
func (r *ContactRepository) FindOrCreateByName(ctx context.Context, userID uuid.UUID, name string) (*models.Contact, bool, error) {
	existing, err := r.GetByExactName(ctx, userID, name)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return nil, false, err
	}

	now := time.Now()
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO contacts (id, user_id, name, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $4)
		ON CONFLICT (user_id, LOWER(name)) DO NOTHING
		RETURNING `+contactColumns,
		uuid.New(), userID, name, now,
	)
	created, err := scanContact(row)
	if err == nil {
		return created, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}

	// Lost the race: the winner's row is what everyone should use.
	existing, err = r.GetByExactName(ctx, userID, name)
	if err != nil {
		return nil, false, err
	}
	return existing, false, nil
}

func (r *ContactRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Contact, error) {
	query := `SELECT ` + contactColumns + ` FROM contacts WHERE id = $1`
	c, err := scanContact(r.db.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *ContactRepository) GetByUserID(ctx context.Context, userID uuid.UUID, updatedSince *time.Time) ([]*models.Contact, error) {
	query := `SELECT ` + contactColumns + ` FROM contacts WHERE user_id = $1`
	args := []any{userID}
	clause, clauseArgs := updatedSinceClause(updatedSince, 2)
	query += clause + " ORDER BY name"
	args = append(args, clauseArgs...)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []*models.Contact
	for rows.Next() {
		c := &models.Contact{}
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.Email, &c.Phone, &c.Notes, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		contacts = append(contacts, c)
	}
	return contacts, rows.Err()
}

func (r *ContactRepository) GetByExactName(ctx context.Context, userID uuid.UUID, name string) (*models.Contact, error) {
	query := `SELECT ` + contactColumns + ` FROM contacts WHERE user_id = $1 AND LOWER(name) = LOWER($2) LIMIT 1`
	c, err := scanContact(r.db.QueryRowContext(ctx, query, userID, name))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *ContactRepository) Search(ctx context.Context, userID uuid.UUID, query string, limit int) ([]*models.Contact, error) {
	sqlQuery := `
		SELECT ` + contactColumns + `
		FROM contacts
		WHERE user_id = $1 AND LOWER(name) LIKE LOWER($2)
		ORDER BY name
		LIMIT $3
	`
	pattern := fmt.Sprintf("%%%s%%", query)
	rows, err := r.db.QueryContext(ctx, sqlQuery, userID, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []*models.Contact
	for rows.Next() {
		c := &models.Contact{}
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.Email, &c.Phone, &c.Notes, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		contacts = append(contacts, c)
	}
	return contacts, rows.Err()
}

const contactUpdateQuery = `
	UPDATE contacts SET name = $1, email = $2, phone = $3, notes = $4, updated_at = $5
	WHERE id = $6
`

func (r *ContactRepository) Update(ctx context.Context, contact *models.Contact) error {
	contact.UpdatedAt = time.Now()
	result, err := r.db.ExecContext(ctx, contactUpdateQuery,
		contact.Name, contact.Email, contact.Phone, contact.Notes, contact.UpdatedAt, contact.ID,
	)
	if isDuplicateName(err) {
		return repository.ErrDuplicate
	}
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// UpdateWithCrewRename updates the contact and carries a name change into the
// crew rows that reference it, returning how many crew rows were rewritten.
// contact.UserID must be set by the caller.
//
// flight_crew_members.name is denormalised: it is the name as logged, and it is
// what exports and PIC-of-record resolution read. Leaving it behind on a rename
// means correcting a typo in the address book fixes nothing in the logbook,
// while rewriting it everywhere would mutate entries an instructor has already
// attested. So the rename stops at the signature boundary, exactly as the
// aircraft-registration rename does.
func (r *ContactRepository) UpdateWithCrewRename(ctx context.Context, contact *models.Contact) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after a successful Commit

	now := time.Now()
	result, err := tx.ExecContext(ctx, contactUpdateQuery,
		contact.Name, contact.Email, contact.Phone, contact.Notes, now, contact.ID,
	)
	if isDuplicateName(err) {
		return 0, repository.ErrDuplicate
	}
	if err != nil {
		return 0, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if rows == 0 {
		return 0, repository.ErrNotFound
	}

	// signature_id IS NULL is REQUIRED — see UpdateWithFlightRename in
	// aircraft.go for what dropping the equivalent guard did there. A signed
	// flight keeps the crew name it was signed with.
	crewResult, err := tx.ExecContext(ctx, `
		UPDATE flight_crew_members fcm
		SET name = $1
		FROM flights f
		WHERE fcm.flight_id = f.id
		  AND fcm.contact_id = $2
		  AND f.user_id = $3
		  AND f.signature_id IS NULL
		  AND fcm.name <> $1`,
		contact.Name, contact.ID, contact.UserID,
	)
	if err != nil {
		return 0, err
	}
	crewUpdated, err := crewResult.RowsAffected()
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	contact.UpdatedAt = now
	return int(crewUpdated), nil
}

// RolesByContact returns the distinct crew roles each of the user's contacts
// has been logged in, so an export can say that someone is an instructor
// without the caller walking every flight.
func (r *ContactRepository) RolesByContact(ctx context.Context, userID uuid.UUID) (map[uuid.UUID][]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT fcm.contact_id, fcm.role::text, COUNT(*)
		FROM flight_crew_members fcm
		JOIN flights f ON f.id = fcm.flight_id
		WHERE f.user_id = $1 AND fcm.contact_id IS NOT NULL
		GROUP BY fcm.contact_id, fcm.role
		ORDER BY fcm.contact_id, COUNT(*) DESC, fcm.role`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[uuid.UUID][]string)
	for rows.Next() {
		var id uuid.UUID
		var role string
		var count int
		if err := rows.Scan(&id, &role, &count); err != nil {
			return nil, err
		}
		out[id] = append(out[id], role)
	}
	return out, rows.Err()
}

func (r *ContactRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM contacts WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return repository.ErrNotFound
	}
	return nil
}
