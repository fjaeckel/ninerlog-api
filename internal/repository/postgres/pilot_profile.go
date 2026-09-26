package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

type pilotProfileRepository struct {
	db *sql.DB
}

// NewPilotProfileRepository creates a PostgreSQL-backed PilotProfileRepository.
func NewPilotProfileRepository(db *sql.DB) repository.PilotProfileRepository {
	return &pilotProfileRepository{db: db}
}

func (r *pilotProfileRepository) Get(ctx context.Context, userID uuid.UUID) (*models.PilotProfile, error) {
	p := &models.PilotProfile{UserID: userID}
	var mode string
	var raw []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT mode, disciplines, created_at, updated_at
		FROM pilot_profiles WHERE user_id = $1`, userID,
	).Scan(&mode, &raw, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get pilot profile: %w", err)
	}
	p.Mode = models.PilotProfileMode(mode)
	p.Disciplines = decodeDisciplineSettings(raw)
	return p, nil
}

// decodeDisciplineSettings parses the stored map, skipping unknown disciplines and
// malformed or unknown settings.
func decodeDisciplineSettings(raw []byte) map[models.Discipline]models.DisciplineSetting {
	out := map[models.Discipline]models.DisciplineSetting{}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return out
	}
	for key, val := range entries {
		d := models.Discipline(key)
		if !d.IsValid() {
			continue
		}
		var s models.DisciplineSetting
		if err := json.Unmarshal(val, &s); err != nil || !s.EffectiveIntent().IsValid() {
			continue
		}
		out[d] = s
	}
	return out
}

func (r *pilotProfileRepository) Upsert(ctx context.Context, p *models.PilotProfile) error {
	disciplines := p.Disciplines
	if disciplines == nil {
		disciplines = map[models.Discipline]models.DisciplineSetting{}
	}
	raw, err := json.Marshal(disciplines)
	if err != nil {
		return fmt.Errorf("encode pilot profile: %w", err)
	}
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO pilot_profiles (user_id, mode, disciplines)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET
			mode = EXCLUDED.mode,
			disciplines = EXCLUDED.disciplines
		RETURNING created_at, updated_at`,
		p.UserID, string(p.Mode), raw,
	).Scan(&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert pilot profile: %w", err)
	}
	return nil
}

type disciplineEvidenceSource struct {
	db *sql.DB
}

// NewDisciplineEvidenceSource creates a PostgreSQL-backed DisciplineEvidenceSource.
func NewDisciplineEvidenceSource(db *sql.DB) repository.DisciplineEvidenceSource {
	return &disciplineEvidenceSource{db: db}
}

// disciplineFlightGroupsQuery aggregates flights per normalised class and UL kind.
var disciplineFlightGroupsQuery = `
	WITH fl AS (
		SELECT
			f.*,
			COALESCE(upper(trim(a.aircraft_class)), '') AS class,
			CASE WHEN upper(trim(a.aircraft_class)) = 'ULTRALIGHT' THEN a.ul_kind END AS kind,
			(NOT f.is_simulator AND NOT f.is_passenger) AS crew,
			COALESCE(f.launch_method, '') IN ` + towedLaunches + ` AS towed
		FROM flights f
		LEFT JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
		WHERE f.user_id = $1
	)
	SELECT
		class,
		kind,
		COUNT(*) FILTER (WHERE crew AND NOT towed),
		MAX(date) FILTER (WHERE crew AND NOT towed),
		COUNT(*) FILTER (WHERE crew AND NOT towed AND dual_time > 0),
		COUNT(*) FILTER (WHERE crew AND towed),
		MAX(date) FILTER (WHERE crew AND towed),
		COUNT(*) FILTER (WHERE crew AND towed AND dual_time > 0),
		COALESCE(SUM(dual_time) FILTER (WHERE crew), 0),
		COALESCE(SUM(dual_given_time) FILTER (WHERE crew), 0),
		COALESCE(SUM(examiner_time) FILTER (WHERE crew), 0),
		COUNT(*) FILTER (WHERE crew AND (dual_given_time > 0 OR examiner_time > 0)),
		MAX(date) FILTER (WHERE crew AND (dual_given_time > 0 OR examiner_time > 0)),
		COALESCE(SUM(ifr_time) FILTER (WHERE crew), 0),
		COALESCE(SUM(approaches_count) FILTER (WHERE crew), 0),
		COUNT(*) FILTER (WHERE crew AND (ifr_time > 0 OR approaches_count > 0)),
		MAX(date) FILTER (WHERE crew AND (ifr_time > 0 OR approaches_count > 0)),
		COALESCE(SUM(multi_pilot_time) FILTER (WHERE crew), 0),
		COALESCE(SUM(sic_time) FILTER (WHERE crew), 0),
		COALESCE(SUM(relief_time) FILTER (WHERE crew), 0),
		COUNT(*) FILTER (WHERE crew AND (multi_pilot_time > 0 OR sic_time > 0 OR relief_time > 0)),
		MAX(date) FILTER (WHERE crew AND (multi_pilot_time > 0 OR sic_time > 0 OR relief_time > 0)),
		COUNT(*) FILTER (WHERE is_simulator AND NOT is_passenger),
		MAX(date) FILTER (WHERE is_simulator AND NOT is_passenger),
		COUNT(*) FILTER (WHERE is_passenger),
		MAX(date) FILTER (WHERE is_passenger)
	FROM fl
	GROUP BY class, kind
	ORDER BY class, kind`

func (s *disciplineEvidenceSource) GetDisciplineFlightGroups(ctx context.Context, userID uuid.UUID) ([]models.DisciplineFlightGroup, error) {
	rows, err := s.db.QueryContext(ctx, disciplineFlightGroupsQuery, userID)
	if err != nil {
		return nil, fmt.Errorf("discipline flight groups: %w", err)
	}
	defer rows.Close()

	var groups []models.DisciplineFlightGroup
	for rows.Next() {
		var g models.DisciplineFlightGroup
		var kind sql.NullString
		var lastFlight, lastTowed, lastInstructing, lastIFR, lastMultiCrew, lastSim, lastPax sql.NullTime
		if err := rows.Scan(
			&g.AircraftClass, &kind,
			&g.Flights, &lastFlight, &g.DualReceivedFlights,
			&g.TowedFlights, &lastTowed, &g.TowedDualReceivedFlights,
			&g.DualReceivedMinutes, &g.DualGivenMinutes, &g.ExaminerMinutes,
			&g.InstructingFlights, &lastInstructing,
			&g.IFRMinutes, &g.Approaches, &g.IFRFlights, &lastIFR,
			&g.MultiPilotMinutes, &g.SICMinutes, &g.ReliefMinutes, &g.MultiCrewFlights, &lastMultiCrew,
			&g.SimulatorSessions, &lastSim,
			&g.PassengerFlights, &lastPax,
		); err != nil {
			return nil, fmt.Errorf("scan discipline flight group: %w", err)
		}
		if kind.Valid {
			k := models.ULKind(kind.String)
			g.ULKind = &k
		}
		g.LastFlight = nullTimePtr(lastFlight)
		g.LastTowed = nullTimePtr(lastTowed)
		g.LastInstructing = nullTimePtr(lastInstructing)
		g.LastIFR = nullTimePtr(lastIFR)
		g.LastMultiCrew = nullTimePtr(lastMultiCrew)
		g.LastSimulator = nullTimePtr(lastSim)
		g.LastPassenger = nullTimePtr(lastPax)
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("discipline flight groups: %w", err)
	}
	return groups, nil
}

func nullTimePtr(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}
