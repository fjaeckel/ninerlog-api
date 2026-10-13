package postgres

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

func TestDecodeDisciplineSettings(t *testing.T) {
	got := decodeDisciplineSettings([]byte(`{
		"SAILPLANE": {"intent": "goal"},
		"IFR": {"intent": "off", "acknowledgedAt": "2026-09-01T10:00:00Z"},
		"TMG": {"acknowledgedAt": "2026-09-02T10:00:00Z"},
		"BALLOON": {"intent": "on"},
		"GYROPLANE": {"intent": "maybe"},
		"HELICOPTER": "garbage"
	}`))
	if len(got) != 3 {
		t.Fatalf("got %+v", got)
	}
	if got[models.DisciplineSailplane].Intent != models.IntentGoal {
		t.Errorf("SAILPLANE = %+v", got[models.DisciplineSailplane])
	}
	if s := got[models.DisciplineIFR]; s.Intent != models.IntentOff || s.AcknowledgedAt == nil {
		t.Errorf("IFR = %+v", s)
	}
	if s := got[models.DisciplineTMG]; s.EffectiveIntent() != models.IntentAuto || s.AcknowledgedAt == nil {
		t.Errorf("TMG = %+v", s)
	}
	if len(decodeDisciplineSettings([]byte(`not json`))) != 0 {
		t.Error("malformed JSON must decode to an empty map")
	}
}

func TestPilotProfileRepository_GetNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	userID := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta("FROM pilot_profiles WHERE user_id = $1")).
		WithArgs(userID).WillReturnError(sql.ErrNoRows)

	_, err = NewPilotProfileRepository(db).Get(context.Background(), userID)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

var disciplineGroupColumns = []string{
	"class", "kind",
	"flights", "last_flight", "dual_received_flights",
	"towed_flights", "last_towed", "towed_dual_received_flights",
	"dual_received_minutes", "dual_given_minutes", "examiner_minutes",
	"instructing_flights", "last_instructing",
	"ifr_minutes", "approaches", "ifr_flights", "last_ifr",
	"multi_pilot_minutes", "sic_minutes", "relief_minutes", "multi_crew_flights", "last_multi_crew",
	"simulator_sessions", "last_simulator",
	"passenger_flights", "last_passenger",
}

func TestGetDisciplineFlightGroups_Scans(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	userID := uuid.New()
	d := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("LEFT JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id")).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows(disciplineGroupColumns).
			AddRow("GLIDER", nil, 0, nil, 0, 14, d, 3, 60, 0, 0, 0, nil, 0, 0, 0, nil, 0, 0, 0, 0, nil, 0, nil, 0, nil).
			AddRow("ULTRALIGHT", "THREE_AXIS", 5, d, 1, 0, nil, 0, 30, 0, 0, 0, nil, 0, 0, 0, nil, 0, 0, 0, 0, nil, 0, nil, 1, d))

	groups, err := NewDisciplineEvidenceSource(db).GetDisciplineFlightGroups(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetDisciplineFlightGroups: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("groups = %+v", groups)
	}
	g := groups[0]
	if g.AircraftClass != "GLIDER" || g.ULKind != nil || g.TowedFlights != 14 || g.TowedDualReceivedFlights != 3 ||
		g.LastTowed == nil || !g.LastTowed.Equal(d) || g.LastFlight != nil {
		t.Errorf("glider group = %+v", g)
	}
	u := groups[1]
	if u.ULKind == nil || *u.ULKind != models.ULKindThreeAxis || u.Flights != 5 || u.PassengerFlights != 1 {
		t.Errorf("UL group = %+v", u)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}
