package postgres

import (
	"context"
	"database/sql"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

type FlightCrewRepository struct {
	db *sql.DB
}

func NewFlightCrewRepository(db *sql.DB) *FlightCrewRepository {
	return &FlightCrewRepository{db: db}
}

func (r *FlightCrewRepository) SetCrewMembers(ctx context.Context, flightID uuid.UUID, members []models.FlightCrewMember) error {
	// Delete existing crew members for the flight
	if err := r.DeleteByFlightID(ctx, flightID); err != nil {
		return err
	}

	if len(members) == 0 {
		return nil
	}

	query := `
		INSERT INTO flight_crew_members (id, flight_id, contact_id, name, role)
		VALUES ($1, $2, $3, $4, $5)
	`
	for _, m := range members {
		m.ID = uuid.New()
		m.FlightID = flightID
		_, err := r.db.ExecContext(ctx, query, m.ID, flightID, m.ContactID, m.Name, m.Role)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *FlightCrewRepository) GetByFlightID(ctx context.Context, flightID uuid.UUID) ([]models.FlightCrewMember, error) {
	query := `
		SELECT id, flight_id, contact_id, name, role
		FROM flight_crew_members
		WHERE flight_id = $1
		ORDER BY role, name
	`
	rows, err := r.db.QueryContext(ctx, query, flightID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []models.FlightCrewMember
	for rows.Next() {
		m := models.FlightCrewMember{}
		if err := rows.Scan(&m.ID, &m.FlightID, &m.ContactID, &m.Name, &m.Role); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

func (r *FlightCrewRepository) DeleteByFlightID(ctx context.Context, flightID uuid.UUID) error {
	query := `DELETE FROM flight_crew_members WHERE flight_id = $1`
	_, err := r.db.ExecContext(ctx, query, flightID)
	return err
}

// GetByFlightIDs batch-loads crew members for multiple flights and returns
// them grouped by flight ID. Used by exporters to avoid N+1 queries when
// rendering many flights.
func (r *FlightCrewRepository) GetByFlightIDs(ctx context.Context, flightIDs []uuid.UUID) (map[uuid.UUID][]models.FlightCrewMember, error) {
	out := make(map[uuid.UUID][]models.FlightCrewMember, len(flightIDs))
	if len(flightIDs) == 0 {
		return out, nil
	}
	query := `
		SELECT id, flight_id, contact_id, name, role
		FROM flight_crew_members
		WHERE flight_id = ANY($1)
		ORDER BY flight_id, role, name
	`
	rows, err := r.db.QueryContext(ctx, query, pq.Array(flightIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		m := models.FlightCrewMember{}
		if err := rows.Scan(&m.ID, &m.FlightID, &m.ContactID, &m.Name, &m.Role); err != nil {
			return nil, err
		}
		out[m.FlightID] = append(out[m.FlightID], m)
	}
	return out, rows.Err()
}
