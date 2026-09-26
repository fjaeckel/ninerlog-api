package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Rows per multi-row INSERT; each chunk stays far below PostgreSQL's limit of
// 65535 bind parameters per statement.
const (
	flightInsertChunk   = 500
	contactInsertChunk  = 1000
	crewInsertChunk     = 1000
	revisionInsertChunk = 1000
	uuidChunk           = 20000
)

// queryExecer is the part of *sql.DB and *sql.Tx that single-row inserts use.
type queryExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

type restoreStore struct {
	db               *sql.DB
	statementTimeout time.Duration
}

// NewRestoreStore creates the transactional restore store. statementTimeout
// replaces the connection's statement timeout inside each restore transaction.
func NewRestoreStore(db *sql.DB, statementTimeout time.Duration) repository.RestoreStore {
	return &restoreStore{db: db, statementTimeout: statementTimeout}
}

func (s *restoreStore) InRestoreTx(ctx context.Context, userID uuid.UUID, fn func(tx repository.RestoreTx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // no-op once committed

	if s.statementTimeout > 0 {
		stmt := fmt.Sprintf("SET LOCAL statement_timeout = %d", s.statementTimeout.Milliseconds())
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended('user-data-restore:' || $1::text, 0))`, userID); err != nil {
		return err
	}
	if err := applyChangeContext(ctx, tx); err != nil {
		return err
	}

	if err := fn(&restoreTx{tx: tx}); err != nil {
		return err
	}
	return tx.Commit()
}

type restoreTx struct {
	tx *sql.Tx
}

func (t *restoreTx) CountContent(ctx context.Context, userID uuid.UUID) (repository.RestoreContent, error) {
	var c repository.RestoreContent
	err := t.tx.QueryRowContext(ctx, `
		SELECT
			EXISTS (SELECT 1 FROM flights WHERE user_id = $1),
			EXISTS (SELECT 1 FROM aircraft WHERE user_id = $1),
			EXISTS (SELECT 1 FROM licenses WHERE user_id = $1),
			EXISTS (SELECT 1 FROM credentials WHERE user_id = $1),
			EXISTS (SELECT 1 FROM contacts WHERE user_id = $1),
			EXISTS (SELECT 1 FROM custom_currency_rules WHERE user_id = $1),
			EXISTS (SELECT 1 FROM custom_reports WHERE user_id = $1)
	`, userID).Scan(&c.Flights, &c.Aircraft, &c.Licenses, &c.Credentials,
		&c.Contacts, &c.CustomCurrencyRules, &c.CustomReports)
	return c, err
}

func (t *restoreTx) CountAttachments(ctx context.Context, userID uuid.UUID) (repository.RestoreAttachments, error) {
	var a repository.RestoreAttachments
	err := t.tx.QueryRowContext(ctx, `
		SELECT
			EXISTS (SELECT 1 FROM document_files WHERE user_id = $1),
			EXISTS (SELECT 1 FROM flights WHERE user_id = $1 AND signature_id IS NOT NULL)
				OR EXISTS (SELECT 1 FROM flight_signatures WHERE user_id = $1)
	`, userID).Scan(&a.DocumentFiles, &a.SignedFlights)
	return a, err
}

func (t *restoreTx) WipeContent(ctx context.Context, userID uuid.UUID, correctionFlightIDs []uuid.UUID) error {
	if len(correctionFlightIDs) > 0 {
		if err := t.deleteFlights(ctx, userID, correctionFlightIDs); err != nil {
			return err
		}
	}
	queries := append(append([]string{}, logbookWipeQueries...),
		`DELETE FROM custom_currency_rules WHERE user_id = $1`,
		`DELETE FROM custom_reports WHERE user_id = $1`,
	)
	for _, q := range queries {
		if _, err := t.tx.ExecContext(ctx, q, userID); err != nil {
			return err
		}
	}
	return nil
}

// deleteFlights deletes the flights and their crew with the correction flag
// set, then puts the flag of the change context back.
func (t *restoreTx) deleteFlights(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) error {
	if err := t.setCorrection(ctx, true); err != nil {
		return err
	}
	for start := 0; start < len(ids); start += uuidChunk {
		chunk := pq.Array(ids[start:min(start+uuidChunk, len(ids))])
		if _, err := t.tx.ExecContext(ctx, `DELETE FROM flight_crew_members WHERE flight_id = ANY($1::uuid[])`, chunk); err != nil {
			return err
		}
		if _, err := t.tx.ExecContext(ctx, `DELETE FROM flights WHERE user_id = $1 AND id = ANY($2::uuid[])`, userID, chunk); err != nil {
			return err
		}
	}
	cc, _ := repository.ChangeContextFrom(ctx)
	return t.setCorrection(ctx, cc.IsCorrection)
}

func (t *restoreTx) setCorrection(ctx context.Context, correction bool) error {
	_, err := t.tx.ExecContext(ctx, `SELECT set_config('ninerlog.change_correction', $1, true)`, strconv.FormatBool(correction))
	return err
}

func (t *restoreTx) Repos() repository.TxRepos {
	return &txRepos{tx: t.tx}
}

// deleteClassifiedCorrections deletes the flights the classifier carried by
// ctx names, with the correction flag set, and returns how many it deleted.
// It does nothing without one.
func (t *restoreTx) deleteClassifiedCorrections(ctx context.Context, userID uuid.UUID) (int64, error) {
	classify, ok := repository.DeleteCorrectionsFrom(ctx)
	if !ok {
		return 0, nil
	}
	ids, err := classify(ctx, t.Repos(), userID, func() ([]*models.Flight, error) {
		return t.ListFlightTimings(ctx, userID)
	})
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	return int64(len(ids)), t.deleteFlights(ctx, userID, ids)
}

func (t *restoreTx) ListFlightTimings(ctx context.Context, userID uuid.UUID) ([]*models.Flight, error) {
	rows, err := t.tx.QueryContext(ctx,
		`SELECT f.id, f.date, f.off_block_time, f.on_block_time, f.departure_icao,
		        (SELECT s.id FROM flight_signatures s
		          WHERE s.flight_id = f.id AND s.status IN ('completed', 'voided') LIMIT 1)
		   FROM flights f WHERE f.user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*models.Flight
	for rows.Next() {
		f := &models.Flight{UserID: userID}
		var off, on *time.Time
		if err := rows.Scan(&f.ID, &f.Date, &off, &on, &f.DepartureICAO, &f.SignatureID); err != nil {
			return nil, err
		}
		f.OffBlockTime, f.OnBlockTime = timeToString(off), timeToString(on)
		out = append(out, f)
	}
	return out, rows.Err()
}

func (t *restoreTx) FlightIDUsage(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (repository.FlightIDUsage, error) {
	usage := repository.FlightIDUsage{Taken: map[uuid.UUID]bool{}, Voided: map[uuid.UUID]bool{}}
	for start := 0; start < len(ids); start += uuidChunk {
		if err := t.collectFlightIDUsage(ctx, userID, ids[start:min(start+uuidChunk, len(ids))], &usage); err != nil {
			return usage, err
		}
	}
	return usage, nil
}

func (t *restoreTx) collectFlightIDUsage(ctx context.Context, userID uuid.UUID, ids []uuid.UUID, usage *repository.FlightIDUsage) error {
	rows, err := t.tx.QueryContext(ctx, `
		SELECT id, true FROM flights WHERE id = ANY($1::uuid[])
		UNION
		SELECT flight_id, user_id <> $2 FROM flight_revisions
		 WHERE flight_id = ANY($1::uuid[]) AND (user_id <> $2 OR kind = 'delete')`, pq.Array(ids), userID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id uuid.UUID
		var taken bool
		if err := rows.Scan(&id, &taken); err != nil {
			return err
		}
		if taken {
			usage.Taken[id] = true
		} else {
			usage.Voided[id] = true
		}
	}
	return rows.Err()
}

func (t *restoreTx) InsertAircraft(ctx context.Context, aircraft *models.Aircraft) error {
	return (&aircraftRepository{tx: t.tx}).Create(ctx, aircraft)
}

func (t *restoreTx) InsertLicense(ctx context.Context, license *models.License) error {
	return (&licenseRepository{tx: t.tx}).Create(ctx, license)
}

func (t *restoreTx) InsertClassRating(ctx context.Context, rating *models.ClassRating) error {
	return (&ClassRatingRepository{tx: t.tx}).Create(ctx, rating)
}

func (t *restoreTx) InsertLicencePrivilege(ctx context.Context, privilege *models.LicencePrivilege) error {
	return (&LicencePrivilegeRepository{db: t.tx}).Create(ctx, privilege)
}

func (t *restoreTx) InsertFlightFile(ctx context.Context, file *models.FlightFile) error {
	return t.tx.QueryRowContext(ctx, `
		INSERT INTO flight_files (user_id, flight_id, kind, filename, content, size_bytes, sha256)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at`,
		file.UserID, file.FlightID, string(file.Kind), file.Filename, file.Content, file.SizeBytes, file.SHA256,
	).Scan(&file.ID, &file.CreatedAt)
}

func (t *restoreTx) InsertCredential(ctx context.Context, credential *models.Credential) error {
	return (&credentialRepository{tx: t.tx}).Create(ctx, credential)
}

func (t *restoreTx) InsertCustomCurrencyRule(ctx context.Context, rule *models.CustomCurrencyRule) error {
	return (&customCurrencyRuleRepository{tx: t.tx}).Create(ctx, rule)
}

func (t *restoreTx) InsertCustomReport(ctx context.Context, report *models.CustomReport) error {
	return (&customReportRepository{tx: t.tx}).Create(ctx, report)
}

func (t *restoreTx) UpsertNotificationPreferences(ctx context.Context, prefs *models.NotificationPreferences) error {
	return (&notificationRepository{tx: t.tx}).UpsertPreferences(ctx, prefs)
}

func (t *restoreTx) UpsertPilotProfile(ctx context.Context, profile *models.PilotProfile) error {
	return (&pilotProfileRepository{db: t.tx}).Upsert(ctx, profile)
}

func (t *restoreTx) UpsertFlightBaseline(ctx context.Context, baseline *models.FlightBaseline) error {
	return (&flightBaselineRepository{tx: t.tx}).Upsert(ctx, baseline)
}

func (t *restoreTx) InsertContacts(ctx context.Context, contacts []*models.Contact) error {
	const perRow = 6
	for start := 0; start < len(contacts); start += contactInsertChunk {
		chunk := contacts[start:min(start+contactInsertChunk, len(contacts))]
		args := make([]any, 0, len(chunk)*perRow)
		for _, c := range chunk {
			args = append(args, c.ID, c.UserID, c.Name, c.Email, c.Phone, c.Notes)
		}
		query := `INSERT INTO contacts (id, user_id, name, email, phone, notes) VALUES ` +
			valueRows(len(chunk), perRow)
		if _, err := t.tx.ExecContext(ctx, query, args...); err != nil {
			return err
		}
	}
	return nil
}

func (t *restoreTx) InsertFlights(ctx context.Context, flights []*models.Flight) error {
	for start := 0; start < len(flights); start += flightInsertChunk {
		chunk := flights[start:min(start+flightInsertChunk, len(flights))]
		var args []any
		var rows strings.Builder
		for i, f := range chunk {
			first := len(args) + 1
			args = append(args, f.ID)
			args = append(args, flightInsertArgs(f)...)
			args = append(args, nullableTime(f.CreatedAt))
			if i > 0 {
				rows.WriteString(", ")
			}
			fmt.Fprintf(&rows, "(%s, COALESCE($%d::timestamptz, now()))",
				placeholders(first, len(args)-first), len(args))
		}
		query := `INSERT INTO flights (id, ` + flightInsertColumns + `, created_at) VALUES ` + rows.String()
		if _, err := t.tx.ExecContext(ctx, query, args...); err != nil {
			return err
		}
	}
	return nil
}

func (t *restoreTx) InsertCrewMembers(ctx context.Context, members []models.FlightCrewMember) error {
	const perRow = 5
	for start := 0; start < len(members); start += crewInsertChunk {
		chunk := members[start:min(start+crewInsertChunk, len(members))]
		args := make([]any, 0, len(chunk)*perRow)
		for _, m := range chunk {
			args = append(args, m.ID, m.FlightID, m.ContactID, m.Name, m.Role)
		}
		query := `INSERT INTO flight_crew_members (id, flight_id, contact_id, name, role) VALUES ` +
			valueRows(len(chunk), perRow)
		if _, err := t.tx.ExecContext(ctx, query, args...); err != nil {
			return err
		}
	}
	return nil
}

func (t *restoreTx) logbook() *logbookRepository {
	return &logbookRepository{tx: t.tx}
}

func (t *restoreTx) SetStrict(ctx context.Context, userID uuid.UUID, strict bool, trigger string, licenseID *uuid.UUID) (bool, error) {
	return t.logbook().SetStrict(ctx, userID, strict, trigger, licenseID)
}

func (t *restoreTx) RestoreStrictSince(ctx context.Context, userID uuid.UUID, since time.Time) error {
	return t.logbook().RestoreStrictSince(ctx, userID, since)
}

func (t *restoreTx) ImportModeEvent(ctx context.Context, event *models.LogbookModeEvent) (bool, error) {
	return t.logbook().ImportModeEvent(ctx, event)
}

func (t *restoreTx) ImportCertification(ctx context.Context, c *models.LogbookCertification) (bool, error) {
	return t.logbook().ImportCertification(ctx, c)
}

func (t *restoreTx) ImportRevisions(ctx context.Context, revs []models.FlightRevision) (int, error) {
	return t.logbook().ImportRevisions(ctx, revs)
}

// valueRows renders "($1, $2), ($3, $4), ..." for rows of perRow parameters.
func valueRows(rows, perRow int) string {
	var b strings.Builder
	for r := 0; r < rows; r++ {
		if r > 0 {
			b.WriteString(", ")
		}
		b.WriteString("(")
		b.WriteString(placeholders(r*perRow+1, perRow))
		b.WriteString(")")
	}
	return b.String()
}
