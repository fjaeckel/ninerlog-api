package service

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/airports"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightcalc"
	"github.com/google/uuid"
)

var ErrInvalidSessionEvent = errors.New("invalid flight session event")

// maxEventClockSkew bounds how far in the future a client-supplied event
// timestamp may lie. Offline clients replay past taps; nothing legitimately
// reports an event from the future beyond ordinary clock drift.
const maxEventClockSkew = 5 * time.Minute

// FlightSessionService manages live tap-to-log flight sessions: opening a
// session on off-block, stamping takeoff/landing, and converting the session
// into a regular flight log entry when the pilot goes on blocks.
type FlightSessionService struct {
	sessionRepo   repository.FlightSessionRepository
	aircraftRepo  repository.AircraftRepository
	flightService *FlightService

	// Injection points for tests
	now            func() time.Time
	nearestAirport func(lat, lon float64) *airports.AirportInfo
}

func NewFlightSessionService(
	sessionRepo repository.FlightSessionRepository,
	aircraftRepo repository.AircraftRepository,
	flightService *FlightService,
) *FlightSessionService {
	return &FlightSessionService{
		sessionRepo:    sessionRepo,
		aircraftRepo:   aircraftRepo,
		flightService:  flightService,
		now:            time.Now,
		nearestAirport: airports.Nearest,
	}
}

// FlightSessionEventInput carries one tap-to-log event.
type FlightSessionEventInput struct {
	Type        string
	OccurredAt  *time.Time
	AircraftReg *string
	ICAO        *string
	Lat         *float64
	Lon         *float64

	// UserName is the authenticated user's display name, forwarded to
	// flight auto-calculations when the session is converted into a flight.
	UserName string
}

// GetCurrent returns the user's open session or models.ErrNoOpenFlightSession.
func (s *FlightSessionService) GetCurrent(ctx context.Context, userID uuid.UUID) (*models.FlightSession, error) {
	session, err := s.sessionRepo.GetOpenByUserID(ctx, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, models.ErrNoOpenFlightSession
	}
	return session, err
}

// Discard marks the user's open session as discarded without creating a
// flight. Returns models.ErrNoOpenFlightSession when there is none.
func (s *FlightSessionService) Discard(ctx context.Context, userID uuid.UUID) error {
	session, err := s.GetCurrent(ctx, userID)
	if err != nil {
		return err
	}
	session.Status = models.FlightSessionStatusDiscarded
	return s.sessionRepo.Update(ctx, session)
}

// RecordEvent applies one tap-to-log event. It returns the resulting session
// and whether a new session was opened (offblock on a user with no open
// session). Repeating an event type already recorded on the session is a
// no-op returning the unchanged session, which makes offline retries and
// double-taps safe. An onblock event completes the session and creates the
// flight log entry.
func (s *FlightSessionService) RecordEvent(ctx context.Context, userID uuid.UUID, input FlightSessionEventInput) (*models.FlightSession, bool, error) {
	occurredAt, err := s.resolveOccurredAt(input.OccurredAt)
	if err != nil {
		return nil, false, err
	}
	icao := s.resolveAirport(input)

	session, err := s.GetCurrent(ctx, userID)
	if err != nil && !errors.Is(err, models.ErrNoOpenFlightSession) {
		return nil, false, err
	}

	switch input.Type {
	case models.FlightSessionEventOffBlock:
		if session != nil {
			// Already off blocks — duplicate tap or offline retry.
			return session, false, nil
		}
		return s.openSession(ctx, userID, occurredAt, input.AircraftReg, icao)

	case models.FlightSessionEventTakeoff:
		if session == nil {
			return nil, false, models.ErrNoOpenFlightSession
		}
		if session.TakeoffAt != nil {
			return session, false, nil
		}
		session.TakeoffAt = &occurredAt
		fillIfEmpty(&session.DepartureICAO, icao)
		fillIfEmpty(&session.AircraftReg, normalizeReg(input.AircraftReg))
		return s.saveOrdered(ctx, session)

	case models.FlightSessionEventLanding:
		if session == nil {
			return nil, false, models.ErrNoOpenFlightSession
		}
		if session.LandingAt != nil {
			return session, false, nil
		}
		session.LandingAt = &occurredAt
		fillIfEmpty(&session.ArrivalICAO, icao)
		fillIfEmpty(&session.AircraftReg, normalizeReg(input.AircraftReg))
		return s.saveOrdered(ctx, session)

	case models.FlightSessionEventOnBlock:
		if session == nil {
			return nil, false, models.ErrNoOpenFlightSession
		}
		if session.OnBlockAt == nil {
			session.OnBlockAt = &occurredAt
		}
		fillIfEmpty(&session.ArrivalICAO, icao)
		fillIfEmpty(&session.AircraftReg, normalizeReg(input.AircraftReg))
		return s.completeSession(ctx, session, input.UserName)

	default:
		return nil, false, ErrInvalidSessionEvent
	}
}

func (s *FlightSessionService) resolveOccurredAt(clientTime *time.Time) (time.Time, error) {
	now := s.now().UTC()
	if clientTime == nil {
		return now, nil
	}
	t := clientTime.UTC()
	if t.After(now.Add(maxEventClockSkew)) {
		return time.Time{}, models.ErrInvalidFlightSessionData
	}
	return t, nil
}

// resolveAirport returns the event's airport: an explicit ICAO code wins,
// otherwise GPS coordinates are resolved to the nearest known airport.
func (s *FlightSessionService) resolveAirport(input FlightSessionEventInput) *string {
	if input.ICAO != nil {
		code := strings.ToUpper(strings.TrimSpace(*input.ICAO))
		if len(code) == 4 {
			return &code
		}
	}
	if input.Lat != nil && input.Lon != nil {
		if ap := s.nearestAirport(*input.Lat, *input.Lon); ap != nil {
			code := ap.ICAO
			return &code
		}
	}
	return nil
}

func (s *FlightSessionService) openSession(ctx context.Context, userID uuid.UUID, offBlockAt time.Time, reg *string, departureICAO *string) (*models.FlightSession, bool, error) {
	session := &models.FlightSession{
		UserID:        userID,
		Status:        models.FlightSessionStatusOpen,
		OffBlockAt:    &offBlockAt,
		AircraftReg:   normalizeReg(reg),
		DepartureICAO: departureICAO,
	}
	err := s.sessionRepo.Create(ctx, session)
	if errors.Is(err, repository.ErrDuplicate) {
		// Lost a race against a concurrent offblock — return the winner.
		existing, getErr := s.GetCurrent(ctx, userID)
		if getErr != nil {
			return nil, false, getErr
		}
		return existing, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return session, true, nil
}

func (s *FlightSessionService) saveOrdered(ctx context.Context, session *models.FlightSession) (*models.FlightSession, bool, error) {
	if err := session.ValidateEventOrder(); err != nil {
		return nil, false, err
	}
	if err := s.sessionRepo.Update(ctx, session); err != nil {
		return nil, false, err
	}
	return session, false, nil
}

// completeSession validates the finished session, converts it into a flight
// log entry, and closes it. The session stays open when validation fails so
// the client can retry (e.g. resend onblock with the missing registration).
func (s *FlightSessionService) completeSession(ctx context.Context, session *models.FlightSession, userName string) (*models.FlightSession, bool, error) {
	if err := session.ValidateEventOrder(); err != nil {
		return nil, false, err
	}
	duration := session.BlockDuration()
	if duration <= 0 {
		return nil, false, models.ErrFlightSessionTimeOrder
	}
	if duration > models.MaxFlightSessionDuration {
		return nil, false, models.ErrFlightSessionTooLong
	}
	if session.AircraftReg == nil || *session.AircraftReg == "" {
		return nil, false, models.ErrFlightSessionMissingReg
	}

	flight := s.buildFlight(ctx, session, userName)
	if err := s.flightService.CreateFlight(ctx, flight); err != nil {
		return nil, false, err
	}

	session.Status = models.FlightSessionStatusCompleted
	session.FlightID = &flight.ID
	if err := s.sessionRepo.Update(ctx, session); err != nil {
		return nil, false, err
	}
	return session, false, nil
}

// buildFlight converts a finished session into a flight log entry. The
// flight date is the UTC date of off-block; event instants become HH:MM:SS
// strings as stored on flights. Total time is the block duration, and the
// standard auto-calculations fill night time, landings split, distance, etc.
// The pilot completes remaining details (crew, instrument time, remarks)
// later in the normal flight edit flow.
func (s *FlightSessionService) buildFlight(ctx context.Context, session *models.FlightSession, userName string) *models.Flight {
	offBlock := session.OffBlockAt.UTC()
	onBlock := session.OnBlockAt.UTC()

	totalMinutes := int(math.Round(session.BlockDuration().Minutes()))
	if totalMinutes < 1 {
		totalMinutes = 1
	}

	flight := &models.Flight{
		UserID:        session.UserID,
		Date:          time.Date(offBlock.Year(), offBlock.Month(), offBlock.Day(), 0, 0, 0, 0, time.UTC),
		AircraftReg:   *session.AircraftReg,
		AircraftType:  s.lookupAircraftType(ctx, session.UserID, *session.AircraftReg),
		DepartureICAO: session.DepartureICAO,
		ArrivalICAO:   session.ArrivalICAO,
		OffBlockTime:  timeOfDay(offBlock),
		OnBlockTime:   timeOfDay(onBlock),
		DepartureTime: timeOfDayPtr(session.TakeoffAt),
		ArrivalTime:   timeOfDayPtr(session.LandingAt),
		TotalTime:     totalMinutes,
		IsPIC:         true,
		AllLandings:   1, // a completed flight has one landing; day/night split is auto-calculated
	}

	flightcalc.ApplyAutoCalculations(flight, userName)
	return flight
}

// lookupAircraftType resolves the aircraft type from the user's aircraft
// list by registration. Falls back to "UNKNOWN" so the quick-logged flight
// can still be created; the pilot corrects it during review.
func (s *FlightSessionService) lookupAircraftType(ctx context.Context, userID uuid.UUID, reg string) string {
	aircraft, err := s.aircraftRepo.GetByUserID(ctx, userID)
	if err == nil {
		for _, a := range aircraft {
			if strings.EqualFold(a.Registration, reg) {
				return a.Type
			}
		}
	}
	return "UNKNOWN"
}

func normalizeReg(reg *string) *string {
	if reg == nil {
		return nil
	}
	r := strings.ToUpper(strings.TrimSpace(*reg))
	if r == "" {
		return nil
	}
	return &r
}

// fillIfEmpty sets *dst to val when dst is currently unset and val is present.
func fillIfEmpty(dst **string, val *string) {
	if *dst == nil && val != nil {
		*dst = val
	}
}

func timeOfDay(t time.Time) *string {
	s := t.Format("15:04:05")
	return &s
}

func timeOfDayPtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	return timeOfDay(t.UTC())
}
