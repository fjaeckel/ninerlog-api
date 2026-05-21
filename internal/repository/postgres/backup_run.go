package postgres

import (
	"context"
	"database/sql"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

type backupRunRepository struct {
	db *sql.DB
}

// NewBackupRunRepository returns a Postgres-backed repository for backup runs.
func NewBackupRunRepository(db *sql.DB) repository.BackupRunRepository {
	return &backupRunRepository{db: db}
}

const backupRunColumns = `
	id, destination_id, user_id, status, trigger,
	started_at, completed_at, duration_ms,
	size_bytes, sha256,
	flight_count, aircraft_count, license_count, credential_count,
	remote_path, error_message, created_at
`

func (r *backupRunRepository) Create(ctx context.Context, run *models.BackupRun) error {
	query := `
		INSERT INTO backup_runs (
			destination_id, user_id, status, trigger,
			started_at, completed_at, duration_ms,
			size_bytes, sha256,
			flight_count, aircraft_count, license_count, credential_count,
			remote_path, error_message
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING id, created_at
	`
	return r.db.QueryRowContext(ctx, query,
		run.DestinationID,
		run.UserID,
		string(run.Status),
		string(run.Trigger),
		run.StartedAt,
		run.CompletedAt,
		run.DurationMs,
		run.SizeBytes,
		run.SHA256,
		run.FlightCount,
		run.AircraftCount,
		run.LicenseCount,
		run.CredentialCount,
		run.RemotePath,
		run.ErrorMessage,
	).Scan(&run.ID, &run.CreatedAt)
}

func (r *backupRunRepository) Update(ctx context.Context, run *models.BackupRun) error {
	query := `
		UPDATE backup_runs SET
			status = $1,
			completed_at = $2,
			duration_ms = $3,
			size_bytes = $4,
			sha256 = $5,
			flight_count = $6,
			aircraft_count = $7,
			license_count = $8,
			credential_count = $9,
			remote_path = $10,
			error_message = $11
		WHERE id = $12
	`
	result, err := r.db.ExecContext(ctx, query,
		string(run.Status),
		run.CompletedAt,
		run.DurationMs,
		run.SizeBytes,
		run.SHA256,
		run.FlightCount,
		run.AircraftCount,
		run.LicenseCount,
		run.CredentialCount,
		run.RemotePath,
		run.ErrorMessage,
		run.ID,
	)
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

func (r *backupRunRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.BackupRun, error) {
	query := `SELECT ` + backupRunColumns + ` FROM backup_runs WHERE id = $1`
	row := r.db.QueryRowContext(ctx, query, id)
	run, err := scanBackupRun(row)
	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound
	}
	return run, err
}

func (r *backupRunRepository) GetByDestinationID(ctx context.Context, destinationID uuid.UUID, limit, offset int) ([]*models.BackupRun, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM backup_runs WHERE destination_id = $1",
		destinationID,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `SELECT ` + backupRunColumns + `
		FROM backup_runs
		WHERE destination_id = $1
		ORDER BY started_at DESC
		LIMIT $2 OFFSET $3`
	rows, err := r.db.QueryContext(ctx, query, destinationID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []*models.BackupRun
	for rows.Next() {
		run, err := scanBackupRun(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, run)
	}
	return out, total, rows.Err()
}

func (r *backupRunRepository) DeleteByDestinationID(ctx context.Context, destinationID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM backup_runs WHERE destination_id = $1", destinationID)
	return err
}

func scanBackupRun(s rowScanner) (*models.BackupRun, error) {
	run := &models.BackupRun{}
	var status string
	var trigger string
	var completedAt sql.NullTime
	var durationMs sql.NullInt32
	var sizeBytes sql.NullInt64
	var flightCount sql.NullInt32
	var aircraftCount sql.NullInt32
	var licenseCount sql.NullInt32
	var credentialCount sql.NullInt32

	err := s.Scan(
		&run.ID,
		&run.DestinationID,
		&run.UserID,
		&status,
		&trigger,
		&run.StartedAt,
		&completedAt,
		&durationMs,
		&sizeBytes,
		&run.SHA256,
		&flightCount,
		&aircraftCount,
		&licenseCount,
		&credentialCount,
		&run.RemotePath,
		&run.ErrorMessage,
		&run.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	run.Status = models.BackupRunStatus(status)
	run.Trigger = models.BackupRunTrigger(trigger)
	if completedAt.Valid {
		t := completedAt.Time
		run.CompletedAt = &t
	}
	if durationMs.Valid {
		v := int(durationMs.Int32)
		run.DurationMs = &v
	}
	if sizeBytes.Valid {
		v := sizeBytes.Int64
		run.SizeBytes = &v
	}
	if flightCount.Valid {
		v := int(flightCount.Int32)
		run.FlightCount = &v
	}
	if aircraftCount.Valid {
		v := int(aircraftCount.Int32)
		run.AircraftCount = &v
	}
	if licenseCount.Valid {
		v := int(licenseCount.Int32)
		run.LicenseCount = &v
	}
	if credentialCount.Valid {
		v := int(credentialCount.Int32)
		run.CredentialCount = &v
	}
	return run, nil
}
