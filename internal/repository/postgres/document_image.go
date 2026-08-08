package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

type documentImageRepository struct {
	db *sql.DB
}

func NewDocumentImageRepository(db *sql.DB) repository.DocumentImageRepository {
	return &documentImageRepository{db: db}
}

// subjectColumn maps a subject type to the FK column that carries it. Callers
// come from a closed set of two constants; an unknown value is a programming
// error and is refused rather than defaulted, so a typo can never widen a
// query to "all rows".
func subjectColumn(subject models.DocumentSubjectType) (string, error) {
	switch subject {
	case models.DocumentSubjectLicense:
		return "license_id", nil
	case models.DocumentSubjectCredential:
		return "credential_id", nil
	default:
		return "", fmt.Errorf("unknown document subject type %q", subject)
	}
}

// subjectTable names the parent table a subject lives in. Only used to take a
// row lock on the owning document while its image count is being enforced.
func subjectTable(subject models.DocumentSubjectType) (string, error) {
	switch subject {
	case models.DocumentSubjectLicense:
		return "licenses", nil
	case models.DocumentSubjectCredential:
		return "credentials", nil
	default:
		return "", fmt.Errorf("unknown document subject type %q", subject)
	}
}

// documentImageColumns is the metadata projection — deliberately without
// `data`, so listings never pull the payload out of TOAST storage.
const documentImageColumns = `id, user_id, license_id, credential_id, content_type,
		byte_size, width, height, filename, caption, created_at, updated_at`

func scanDocumentImage(scan func(dest ...any) error, withData bool) (*models.DocumentImage, error) {
	img := &models.DocumentImage{}
	dest := []any{
		&img.ID, &img.UserID, &img.LicenseID, &img.CredentialID, &img.ContentType,
		&img.ByteSize, &img.Width, &img.Height, &img.Filename, &img.Caption,
		&img.CreatedAt, &img.UpdatedAt,
	}
	if withData {
		dest = append(dest, &img.Data)
	}
	if err := scan(dest...); err != nil {
		return nil, err
	}
	return img, nil
}

func (r *documentImageRepository) Create(ctx context.Context, image *models.DocumentImage, maxPerSubject int) error {
	col, err := subjectColumn(image.SubjectType())
	if err != nil {
		return err
	}
	subjectID := image.SubjectID()
	if subjectID == uuid.Nil {
		return fmt.Errorf("document image has no subject")
	}

	table, err := subjectTable(image.SubjectType())
	if err != nil {
		return err
	}

	// Enforcing the cap needs a lock, not just a single statement.
	//
	// Putting the count in the INSERT's WHERE clause is NOT enough on its own:
	// under READ COMMITTED the subquery reads the statement's snapshot and
	// takes no lock, so two uploads arriving together both see "4 images" and
	// both write a fifth. That is a genuine race, and it reproduces under the
	// concurrent-upload integration test.
	//
	// So the count and the insert run inside one transaction that first takes
	// a row lock on the owning document. Competing uploads for the same
	// licence/credential serialize on that row, which makes the count stable
	// for the rest of the transaction. Uploads to *different* documents never
	// contend. The lock is taken on the parent before touching the child, the
	// same order a cascading DELETE of the parent takes, so the two cannot
	// deadlock against each other.
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // no-op once committed

	var lockedID uuid.UUID
	lockQuery := fmt.Sprintf(`SELECT id FROM %s WHERE id = $1 FOR UPDATE`, table)
	if err := tx.QueryRowContext(ctx, lockQuery, subjectID).Scan(&lockedID); err != nil {
		if err == sql.ErrNoRows {
			// The document was deleted between the service's ownership check
			// and this write.
			return repository.ErrNotFound
		}
		return err
	}

	var count int
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM document_images WHERE %s = $1`, col)
	if err := tx.QueryRowContext(ctx, countQuery, subjectID).Scan(&count); err != nil {
		return err
	}
	if count >= maxPerSubject {
		return repository.ErrDocumentImageLimit
	}

	insertQuery := fmt.Sprintf(`
		INSERT INTO document_images (user_id, %s, content_type, byte_size, width, height, filename, caption, data)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at
	`, col)
	if err := tx.QueryRowContext(ctx, insertQuery,
		image.UserID,
		subjectID,
		image.ContentType,
		image.ByteSize,
		image.Width,
		image.Height,
		image.Filename,
		image.Caption,
		image.Data,
	).Scan(&image.ID, &image.CreatedAt, &image.UpdatedAt); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *documentImageRepository) ListBySubject(ctx context.Context, userID uuid.UUID, subject models.DocumentSubjectType, subjectID uuid.UUID) ([]*models.DocumentImage, error) {
	col, err := subjectColumn(subject)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`
		SELECT %s FROM document_images
		WHERE user_id = $1 AND %s = $2
		ORDER BY created_at ASC, id ASC
	`, documentImageColumns, col)

	rows, err := r.db.QueryContext(ctx, query, userID, subjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	images := make([]*models.DocumentImage, 0)
	for rows.Next() {
		img, err := scanDocumentImage(rows.Scan, false)
		if err != nil {
			return nil, err
		}
		images = append(images, img)
	}
	return images, rows.Err()
}

func (r *documentImageRepository) GetWithData(ctx context.Context, userID uuid.UUID, subject models.DocumentSubjectType, subjectID, imageID uuid.UUID) (*models.DocumentImage, error) {
	col, err := subjectColumn(subject)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`
		SELECT %s, data FROM document_images
		WHERE id = $1 AND user_id = $2 AND %s = $3
	`, documentImageColumns, col)

	img, err := scanDocumentImage(r.db.QueryRowContext(ctx, query, imageID, userID, subjectID).Scan, true)
	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return img, nil
}

func (r *documentImageRepository) Delete(ctx context.Context, userID uuid.UUID, subject models.DocumentSubjectType, subjectID, imageID uuid.UUID) error {
	col, err := subjectColumn(subject)
	if err != nil {
		return err
	}
	query := fmt.Sprintf(`DELETE FROM document_images WHERE id = $1 AND user_id = $2 AND %s = $3`, col)

	result, err := r.db.ExecContext(ctx, query, imageID, userID, subjectID)
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

func (r *documentImageRepository) CountBySubject(ctx context.Context, userID uuid.UUID, subject models.DocumentSubjectType, subjectID uuid.UUID) (int, error) {
	col, err := subjectColumn(subject)
	if err != nil {
		return 0, err
	}
	query := fmt.Sprintf(`SELECT COUNT(*) FROM document_images WHERE user_id = $1 AND %s = $2`, col)

	var count int
	if err := r.db.QueryRowContext(ctx, query, userID, subjectID).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}
