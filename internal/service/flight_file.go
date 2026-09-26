package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/airports"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/pkg/igc"
	"github.com/fjaeckel/ninerlog-api/pkg/registration"
	"github.com/google/uuid"
)

// Flight file errors.
var (
	ErrFlightFileNotFound     = errors.New("flight file not found")
	ErrFlightFileEmpty        = errors.New("the file is empty")
	ErrFlightFileTooLarge     = errors.New("the file exceeds the maximum size")
	ErrFlightFileLimitReached = errors.New("maximum number of files for this flight reached")
	// ErrFlightFileDuplicate is returned when the flight already carries the
	// same file; see IGCDuplicateError for a file stored on another flight.
	ErrFlightFileDuplicate = errors.New("this file is already stored")
	// ErrInvalidIGC wraps every igc.ParseError; its message names the reason.
	ErrInvalidIGC = errors.New("not a valid IGC file")
	// ErrIGCNoFlight is returned when the file holds no take-off.
	ErrIGCNoFlight = errors.New("no take-off found in the IGC file")
	// ErrIGCNoRegistration is returned when a flight is to be created from a
	// file without an HFGIDGLIDERID registration.
	ErrIGCNoRegistration = errors.New("the IGC file names no glider registration (HFGIDGLIDERID); attach it to an existing flight instead")
)

// IGCParseError carries the parser's reason for rejecting a file.
type IGCParseError struct {
	Err error
}

func (e *IGCParseError) Error() string {
	var pe *igc.ParseError
	if errors.As(e.Err, &pe) && pe.Line > 0 {
		return fmt.Sprintf("not a valid IGC file: line %d: %v", pe.Line, pe.Err)
	}
	if errors.As(e.Err, &pe) {
		return "not a valid IGC file: " + pe.Err.Error()
	}
	return "not a valid IGC file"
}

func (e *IGCParseError) Unwrap() []error { return []error{ErrInvalidIGC, e.Err} }

// IGCDuplicateError reports that the user already stores the same file on
// another flight.
type IGCDuplicateError struct {
	FlightID uuid.UUID
}

func (e *IGCDuplicateError) Error() string {
	return "this IGC file is already stored on flight " + e.FlightID.String()
}

func (e *IGCDuplicateError) Unwrap() error { return ErrFlightFileDuplicate }

// OutlandingRadiusKm is the distance from an airport, or from the take-off
// point, within which a landing is not an outlanding.
const OutlandingRadiusKm = 3.0

// IGCPlace is a take-off or landing position, with the airport within
// OutlandingRadiusKm when there is one.
type IGCPlace struct {
	ICAO *string
	Name *string
	Lat  float64
	Lon  float64
}

// Label returns the ICAO code, or the coordinates as "50.49889N 9.95389E".
func (p IGCPlace) Label() string {
	if p.ICAO != nil {
		return *p.ICAO
	}
	ns, ew := "N", "E"
	lat, lon := p.Lat, p.Lon
	if lat < 0 {
		ns, lat = "S", -lat
	}
	if lon < 0 {
		ew, lon = "W", -lon
	}
	return fmt.Sprintf("%.5f%s %.5f%s", lat, ns, lon, ew)
}

// IGCPreview is the flight an IGC file describes.
type IGCPreview struct {
	// Date is the UTC date of the take-off.
	Date time.Time
	// TakeoffTime and LandingTime are UTC HH:MM:SS.
	TakeoffTime      string
	LandingTime      string
	LandingDetected  bool
	DurationMinutes  int
	LaunchMethod     string
	LaunchConfidence float64
	ReleaseHeightM   *int
	MaxAltitudeM     int
	FreeDistanceKm   float64
	OutAndReturnKm   float64
	Outlanding       bool
	Departure        IGCPlace
	Arrival          IGCPlace
	// GliderRegistration is canonical; GliderType and Pilot are as recorded.
	GliderRegistration *string
	GliderType         *string
	Pilot              *string
	MatchingFlightID   *uuid.UUID
	// SHA256 is the hex digest of the file.
	SHA256 string
}

// FlightFileService owns flight recorder files: upload validation, IGC
// analysis, ownership checks and the per-flight cap. Every method resolves
// the flight first; a file is only addressable through its flight.
type FlightFileService struct {
	repo       repository.FlightFileRepository
	flightRepo repository.FlightRepository
}

// NewFlightFileService returns a FlightFileService.
func NewFlightFileService(repo repository.FlightFileRepository, flightRepo repository.FlightRepository) *FlightFileService {
	return &FlightFileService{repo: repo, flightRepo: flightRepo}
}

// parseIGC checks the size and parses data.
func parseIGC(data []byte) (*igc.File, error) {
	if len(data) == 0 {
		return nil, ErrFlightFileEmpty
	}
	if len(data) > models.MaxFlightFileBytes {
		return nil, ErrFlightFileTooLarge
	}
	f, err := igc.Parse(data)
	if err != nil {
		if errors.Is(err, igc.ErrTooLarge) {
			return nil, ErrFlightFileTooLarge
		}
		if errors.Is(err, igc.ErrEmpty) {
			return nil, ErrFlightFileEmpty
		}
		return nil, &IGCParseError{Err: err}
	}
	return f, nil
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Preview parses and analyses an IGC file and looks for a flight of the user
// on the same date and glider whose times overlap it.
func (s *FlightFileService) Preview(ctx context.Context, userID uuid.UUID, data []byte) (*IGCPreview, error) {
	f, err := parseIGC(data)
	if err != nil {
		return nil, err
	}
	a, err := igc.Analyze(f)
	if err != nil {
		if errors.Is(err, igc.ErrNoFlight) {
			return nil, ErrIGCNoFlight
		}
		return nil, err
	}

	p := &IGCPreview{
		Date:             time.Date(a.Takeoff.Time.Year(), a.Takeoff.Time.Month(), a.Takeoff.Time.Day(), 0, 0, 0, 0, time.UTC),
		TakeoffTime:      a.Takeoff.Time.Format("15:04:05"),
		LandingTime:      a.Landing.Time.Format("15:04:05"),
		LandingDetected:  a.LandingDetected,
		LaunchMethod:     a.LaunchMethod,
		LaunchConfidence: math.Round(a.LaunchConfidence*100) / 100,
		ReleaseHeightM:   a.ReleaseHeightM,
		MaxAltitudeM:     a.MaxAltitudeM,
		FreeDistanceKm:   a.FreeDistanceKm,
		OutAndReturnKm:   a.OutAndReturnKm,
		Departure:        resolvePlace(a.Takeoff.Lat, a.Takeoff.Lon),
		Arrival:          resolvePlace(a.Landing.Lat, a.Landing.Lon),
		SHA256:           digest(data),
	}
	if m, err := models.ClockSpanMinutes(p.TakeoffTime, p.LandingTime); err == nil {
		p.DurationMinutes = m
	}
	awayFromTakeoff := igc.DistanceKm(a.Takeoff.Lat, a.Takeoff.Lon, a.Landing.Lat, a.Landing.Lon) > OutlandingRadiusKm
	p.Outlanding = a.LandingDetected && awayFromTakeoff && p.Arrival.ICAO == nil && airports.Count() > 0

	if reg := registration.Canonical(f.Header.GliderID); reg != "" {
		p.GliderRegistration = &reg
	}
	if t := strings.TrimSpace(f.Header.GliderType); t != "" {
		p.GliderType = &t
	}
	if pilot := strings.TrimSpace(f.Header.Pilot); pilot != "" {
		p.Pilot = &pilot
	}
	if p.GliderRegistration != nil {
		id, err := s.findMatchingFlight(ctx, userID, p, a.Takeoff.Time, a.Landing.Time)
		if err != nil {
			return nil, err
		}
		p.MatchingFlightID = id
	}
	return p, nil
}

// resolvePlace returns the airport within OutlandingRadiusKm of lat/lon, or
// the bare coordinates.
func resolvePlace(lat, lon float64) IGCPlace {
	place := IGCPlace{Lat: math.Round(lat*1e5) / 1e5, Lon: math.Round(lon*1e5) / 1e5}
	ap := airports.Nearest(lat, lon)
	if ap == nil || igc.DistanceKm(lat, lon, ap.Latitude, ap.Longitude) > OutlandingRadiusKm {
		return place
	}
	icao := ap.ICAO
	place.ICAO = &icao
	if ap.Name != "" {
		name := ap.Name
		place.Name = &name
	}
	return place
}

// findMatchingFlight returns the first flight of the user on the preview's
// date with the same registration whose take-off/landing or block span
// overlaps takeoff..landing. A flight without a complete time pair matches on
// date and registration alone.
func (s *FlightFileService) findMatchingFlight(ctx context.Context, userID uuid.UUID, p *IGCPreview, takeoff, landing time.Time) (*uuid.UUID, error) {
	date := p.Date
	flights, err := s.flightRepo.GetByUserID(ctx, userID, &repository.FlightQueryOptions{StartDate: &date, EndDate: &date})
	if err != nil {
		return nil, fmt.Errorf("list flights: %w", err)
	}
	var loose *uuid.UUID
	for _, fl := range flights {
		if fl.Date.Format("2006-01-02") != date.Format("2006-01-02") ||
			registration.Canonical(fl.AircraftReg) != *p.GliderRegistration {
			continue
		}
		start, end, _, ok := models.ClocksOf(fl).Pair()
		if !ok {
			if loose == nil {
				id := fl.ID
				loose = &id
			}
			continue
		}
		fs, err1 := clockOn(date, start)
		fe, err2 := clockOn(date, end)
		if err1 != nil || err2 != nil {
			continue
		}
		if !fe.After(fs) {
			fe = fe.Add(24 * time.Hour)
		}
		if fs.Before(landing) && fe.After(takeoff) {
			id := fl.ID
			return &id, nil
		}
	}
	return loose, nil
}

func clockOn(date time.Time, clock string) (time.Time, error) {
	t, err := models.ParseClock(clock)
	if err != nil {
		return time.Time{}, err
	}
	return date.Add(time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute + time.Duration(t.Second())*time.Second), nil
}

// verifyFlight proves the caller owns the flight. Both misses collapse to
// ErrFlightNotFound.
func (s *FlightFileService) verifyFlight(ctx context.Context, userID, flightID uuid.UUID) error {
	fl, err := s.flightRepo.GetByID(ctx, flightID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrFlightNotFound
		}
		return err
	}
	if fl.UserID != userID {
		return ErrFlightNotFound
	}
	return nil
}

// StoredFlightFor returns the flight on which the user already stores a file
// with this content, or nil.
func (s *FlightFileService) StoredFlightFor(ctx context.Context, userID uuid.UUID, data []byte) (*uuid.UUID, error) {
	files, err := s.repo.ListBySHA256(ctx, userID, digest(data))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, nil
	}
	id := files[0].FlightID
	return &id, nil
}

// Attach validates an IGC file and stores it on one of the user's flights.
// A file the user already stores is refused with ErrFlightFileDuplicate, or
// an *IGCDuplicateError when it is on another flight.
func (s *FlightFileService) Attach(ctx context.Context, userID, flightID uuid.UUID, filename string, data []byte) (*models.FlightFile, error) {
	if _, err := parseIGC(data); err != nil {
		return nil, err
	}
	if err := s.verifyFlight(ctx, userID, flightID); err != nil {
		return nil, err
	}
	stored, err := s.StoredFlightFor(ctx, userID, data)
	if err != nil {
		return nil, err
	}
	if stored != nil {
		if *stored == flightID {
			return nil, ErrFlightFileDuplicate
		}
		return nil, &IGCDuplicateError{FlightID: *stored}
	}
	name := sanitizeFilename(filename)
	if name == "" {
		name = "flight.igc"
	}
	file := &models.FlightFile{
		UserID:    userID,
		FlightID:  flightID,
		Kind:      models.FlightFileKindIGC,
		Filename:  truncateRunes(name, models.MaxFlightFileFilenameLen),
		SizeBytes: len(data),
		SHA256:    digest(data),
		Content:   data,
	}
	if err := s.repo.Create(ctx, file, models.MaxFlightFilesPerFlight); err != nil {
		switch {
		case errors.Is(err, repository.ErrNotFound):
			return nil, ErrFlightNotFound
		case errors.Is(err, repository.ErrFlightFileLimit):
			return nil, ErrFlightFileLimitReached
		case errors.Is(err, repository.ErrDuplicate):
			return nil, ErrFlightFileDuplicate
		}
		return nil, err
	}
	file.Content = nil
	return file, nil
}

// List returns a flight's files without content, oldest first.
func (s *FlightFileService) List(ctx context.Context, userID, flightID uuid.UUID) ([]*models.FlightFile, error) {
	if err := s.verifyFlight(ctx, userID, flightID); err != nil {
		return nil, err
	}
	return s.repo.ListByFlight(ctx, userID, flightID)
}

// Get returns one file including its content.
func (s *FlightFileService) Get(ctx context.Context, userID, flightID, fileID uuid.UUID) (*models.FlightFile, error) {
	if err := s.verifyFlight(ctx, userID, flightID); err != nil {
		return nil, err
	}
	f, err := s.repo.GetWithContent(ctx, userID, flightID, fileID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrFlightFileNotFound
		}
		return nil, err
	}
	return f, nil
}

// Delete removes one file from a flight.
func (s *FlightFileService) Delete(ctx context.Context, userID, flightID, fileID uuid.UUID) error {
	if err := s.verifyFlight(ctx, userID, flightID); err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, userID, flightID, fileID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrFlightFileNotFound
		}
		return err
	}
	return nil
}

// ListAllWithContent returns every file of the user including its content.
func (s *FlightFileService) ListAllWithContent(ctx context.Context, userID uuid.UUID) ([]*models.FlightFile, error) {
	return s.repo.ListByUserWithContent(ctx, userID)
}
