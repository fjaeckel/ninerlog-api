package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

type flightSignatureRepository struct {
	db *sql.DB
}

// NewFlightSignatureRepository returns a postgres-backed FlightSignatureRepository.
func NewFlightSignatureRepository(db *sql.DB) repository.FlightSignatureRepository {
	return &flightSignatureRepository{db: db}
}

const flightSignatureColumns = `
	id, flight_id, user_id, method, status,
	token_hash, token_expires_at,
	instructor_email, email_sent_at, email_send_count, contact_id,
	instructor_name, instructor_credential_no, signature_image, signed_at,
	signer_ip, signer_user_agent,
	voided_at, voided_reason,
	created_at, updated_at
`

func scanFlightSignature(row interface{ Scan(...interface{}) error }) (*models.FlightSignature, error) {
	s := &models.FlightSignature{}
	if err := row.Scan(
		&s.ID,
		&s.FlightID,
		&s.UserID,
		&s.Method,
		&s.Status,
		&s.TokenHash,
		&s.TokenExpiresAt,
		&s.InstructorEmail,
		&s.EmailSentAt,
		&s.EmailSendCount,
		&s.ContactID,
		&s.InstructorName,
		&s.InstructorCredentialNo,
		&s.SignatureImage,
		&s.SignedAt,
		&s.SignerIP,
		&s.SignerUserAgent,
		&s.VoidedAt,
		&s.VoidedReason,
		&s.CreatedAt,
		&s.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return s, nil
}

func (r *flightSignatureRepository) Create(ctx context.Context, s *models.FlightSignature) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	if s.Status == "" {
		s.Status = models.SignatureStatusPending
	}
	query := `
		INSERT INTO flight_signatures (
			id, flight_id, user_id, method, status,
			token_hash, token_expires_at,
			instructor_email, email_sent_at, email_send_count, contact_id,
			instructor_name, instructor_credential_no, signature_image, signed_at,
			signer_ip, signer_user_agent,
			voided_at, voided_reason
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		RETURNING created_at, updated_at
	`
	err := r.db.QueryRowContext(ctx, query,
		s.ID,
		s.FlightID,
		s.UserID,
		s.Method,
		s.Status,
		s.TokenHash,
		s.TokenExpiresAt,
		s.InstructorEmail,
		s.EmailSentAt,
		s.EmailSendCount,
		s.ContactID,
		s.InstructorName,
		s.InstructorCredentialNo,
		s.SignatureImage,
		s.SignedAt,
		s.SignerIP,
		s.SignerUserAgent,
		s.VoidedAt,
		s.VoidedReason,
	).Scan(&s.CreatedAt, &s.UpdatedAt)
	if err != nil && strings.Contains(err.Error(), "flight_signatures_one_pending_per_flight") {
		return repository.ErrDuplicate
	}
	return err
}

func (r *flightSignatureRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.FlightSignature, error) {
	query := `SELECT ` + flightSignatureColumns + ` FROM flight_signatures WHERE id = $1`
	s, err := scanFlightSignature(r.db.QueryRowContext(ctx, query, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, err
	}
	return s, nil
}

func (r *flightSignatureRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*models.FlightSignature, error) {
	query := `SELECT ` + flightSignatureColumns + ` FROM flight_signatures WHERE token_hash = $1`
	s, err := scanFlightSignature(r.db.QueryRowContext(ctx, query, tokenHash))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, err
	}
	return s, nil
}

func (r *flightSignatureRepository) GetPendingByFlightID(ctx context.Context, flightID uuid.UUID) (*models.FlightSignature, error) {
	query := `SELECT ` + flightSignatureColumns + ` FROM flight_signatures WHERE flight_id = $1 AND status = 'pending'`
	s, err := scanFlightSignature(r.db.QueryRowContext(ctx, query, flightID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, err
	}
	return s, nil
}

func (r *flightSignatureRepository) ListByFlightID(ctx context.Context, flightID uuid.UUID) ([]*models.FlightSignature, error) {
	query := `SELECT ` + flightSignatureColumns + ` FROM flight_signatures WHERE flight_id = $1 ORDER BY created_at DESC`
	rows, err := r.db.QueryContext(ctx, query, flightID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*models.FlightSignature
	for rows.Next() {
		s, err := scanFlightSignature(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func (r *flightSignatureRepository) Update(ctx context.Context, s *models.FlightSignature) error {
	query := `
		UPDATE flight_signatures SET
			status = $2,
			token_hash = $3,
			token_expires_at = $4,
			instructor_email = $5,
			email_sent_at = $6,
			email_send_count = $7,
			contact_id = $8,
			instructor_name = $9,
			instructor_credential_no = $10,
			signature_image = $11,
			signed_at = $12,
			signer_ip = $13,
			signer_user_agent = $14,
			voided_at = $15,
			voided_reason = $16,
			updated_at = now()
		WHERE id = $1
		RETURNING updated_at
	`
	err := r.db.QueryRowContext(ctx, query,
		s.ID,
		s.Status,
		s.TokenHash,
		s.TokenExpiresAt,
		s.InstructorEmail,
		s.EmailSentAt,
		s.EmailSendCount,
		s.ContactID,
		s.InstructorName,
		s.InstructorCredentialNo,
		s.SignatureImage,
		s.SignedAt,
		s.SignerIP,
		s.SignerUserAgent,
		s.VoidedAt,
		s.VoidedReason,
	).Scan(&s.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return repository.ErrNotFound
	}
	return err
}

func (r *flightSignatureRepository) ExpirePendingPastDue(ctx context.Context) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		`UPDATE flight_signatures SET status = 'expired', updated_at = now()
		 WHERE status = 'pending' AND token_expires_at IS NOT NULL AND token_expires_at < now()`,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
