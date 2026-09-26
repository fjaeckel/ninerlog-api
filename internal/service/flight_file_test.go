package service

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/airports"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

type mockFlightFileRepo struct {
	files     map[uuid.UUID]*models.FlightFile
	createErr error
}

func newMockFlightFileRepo() *mockFlightFileRepo {
	return &mockFlightFileRepo{files: map[uuid.UUID]*models.FlightFile{}}
}

func (m *mockFlightFileRepo) Create(_ context.Context, f *models.FlightFile, maxPerFlight int) error {
	if m.createErr != nil {
		return m.createErr
	}
	n := 0
	for _, s := range m.files {
		if s.FlightID == f.FlightID {
			n++
			if s.SHA256 == f.SHA256 {
				return repository.ErrDuplicate
			}
		}
	}
	if n >= maxPerFlight {
		return repository.ErrFlightFileLimit
	}
	f.ID = uuid.New()
	f.CreatedAt = time.Now()
	stored := *f
	m.files[f.ID] = &stored
	return nil
}

func (m *mockFlightFileRepo) ListByFlight(_ context.Context, userID, flightID uuid.UUID) ([]*models.FlightFile, error) {
	out := []*models.FlightFile{}
	for _, f := range m.files {
		if f.UserID == userID && f.FlightID == flightID {
			c := *f
			c.Content = nil
			out = append(out, &c)
		}
	}
	return out, nil
}

func (m *mockFlightFileRepo) GetWithContent(_ context.Context, userID, flightID, fileID uuid.UUID) (*models.FlightFile, error) {
	f, ok := m.files[fileID]
	if !ok || f.UserID != userID || f.FlightID != flightID {
		return nil, repository.ErrNotFound
	}
	return f, nil
}

func (m *mockFlightFileRepo) Delete(_ context.Context, userID, flightID, fileID uuid.UUID) error {
	f, ok := m.files[fileID]
	if !ok || f.UserID != userID || f.FlightID != flightID {
		return repository.ErrNotFound
	}
	delete(m.files, fileID)
	return nil
}

func (m *mockFlightFileRepo) ListBySHA256(_ context.Context, userID uuid.UUID, sha string) ([]*models.FlightFile, error) {
	out := []*models.FlightFile{}
	for _, f := range m.files {
		if f.UserID == userID && f.SHA256 == sha {
			out = append(out, f)
		}
	}
	return out, nil
}

func (m *mockFlightFileRepo) ListByUserWithContent(_ context.Context, userID uuid.UUID) ([]*models.FlightFile, error) {
	out := []*models.FlightFile{}
	for _, f := range m.files {
		if f.UserID == userID {
			out = append(out, f)
		}
	}
	return out, nil
}

func igcFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "pkg", "igc", "testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return b
}

// withAirports installs EDER (Wasserkuppe) and one far-away airport.
func withAirports(t *testing.T) {
	t.Helper()
	airports.SetTestDB(map[string]airports.AirportInfo{
		"EDER": {Name: "Wasserkuppe", Latitude: 50.49889, Longitude: 9.95389},
		"EDDF": {Name: "Frankfurt", Latitude: 50.03333, Longitude: 8.57056},
	})
	t.Cleanup(func() { airports.SetTestDB(nil) })
}

func newFlightFileTestService() (*FlightFileService, *mockFlightFileRepo, *mockFlightRepo) {
	files := newMockFlightFileRepo()
	flights := newMockFlightRepo()
	return NewFlightFileService(files, flights), files, flights
}

func addFlight(t *testing.T, repo *mockFlightRepo, userID uuid.UUID, date, reg, from, to string) *models.Flight {
	t.Helper()
	d, _ := time.Parse("2006-01-02", date)
	f := &models.Flight{UserID: userID, Date: d, AircraftReg: reg, AircraftType: "ASK 21", TotalTime: 10}
	if from != "" {
		f.DepartureTime, f.ArrivalTime = &from, &to
	}
	if err := repo.Create(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestFlightFilePreview_Fixtures(t *testing.T) {
	withAirports(t)
	tests := []struct {
		name       string
		file       string
		launch     string
		date       string
		takeoff    string
		landing    string
		minutes    int
		reg        string
		depICAO    string
		arrICAO    string
		outlanding bool
		releaseMin int
		freeKmMin  float64
	}{
		{name: "P2 self-launched out-and-return with an outlanding", file: "selflaunch.igc", launch: "self-launch",
			date: "2026-08-10", takeoff: "09:31:46", landing: "10:22:46", minutes: 51, reg: "D-KXYZ",
			depICAO: "EDER", outlanding: true, releaseMin: 700, freeKmMin: 39},
		{name: "L winch circuit lands back at EDER", file: "winch.igc", launch: "winch",
			date: "2026-06-15", takeoff: "10:01:33", landing: "10:08:09", minutes: 7, reg: "D-1234",
			depICAO: "EDER", arrICAO: "EDER", releaseMin: 380, freeKmMin: 1.5},
		{name: "winch launch then outlanding 15 km away", file: "outlanding.igc", launch: "winch",
			date: "2026-06-20", reg: "D-1234", takeoff: "13:01:03", landing: "13:22:51", minutes: 22,
			depICAO: "EDER", outlanding: true, releaseMin: 380, freeKmMin: 14},
		{name: "aerotow", file: "aerotow.igc", launch: "aerotow", date: "2026-07-02", takeoff: "12:01:06",
			landing: "12:22:38", minutes: 22, reg: "D-5678", depICAO: "EDER", arrICAO: "EDER", releaseMin: 500, freeKmMin: 5},
		{name: "flight across midnight UTC is dated by its take-off", file: "midnight.igc", launch: "aerotow",
			date: "2025-01-15", takeoff: "23:25:06", landing: "00:36:46", minutes: 72, reg: "ZK-GXX", releaseMin: 700, freeKmMin: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, _ := newFlightFileTestService()
			p, err := svc.Preview(context.Background(), uuid.New(), igcFixture(t, tt.file))
			if err != nil {
				t.Fatalf("Preview: %v", err)
			}
			if p.LaunchMethod != tt.launch {
				t.Errorf("launch = %s", p.LaunchMethod)
			}
			if got := p.Date.Format("2006-01-02"); got != tt.date {
				t.Errorf("date = %s", got)
			}
			if p.TakeoffTime != tt.takeoff || p.LandingTime != tt.landing || p.DurationMinutes != tt.minutes {
				t.Errorf("times = %s-%s (%d min)", p.TakeoffTime, p.LandingTime, p.DurationMinutes)
			}
			if p.GliderRegistration == nil || *p.GliderRegistration != tt.reg {
				t.Errorf("registration = %v", p.GliderRegistration)
			}
			if got := icao(p.Departure); got != tt.depICAO {
				t.Errorf("departure = %q", got)
			}
			if got := icao(p.Arrival); got != tt.arrICAO {
				t.Errorf("arrival = %q", got)
			}
			if p.Outlanding != tt.outlanding {
				t.Errorf("outlanding = %v", p.Outlanding)
			}
			if p.ReleaseHeightM == nil || *p.ReleaseHeightM < tt.releaseMin {
				t.Errorf("release height = %v", p.ReleaseHeightM)
			}
			if p.FreeDistanceKm < tt.freeKmMin || p.OutAndReturnKm < 2*tt.freeKmMin {
				t.Errorf("distances = %.1f / %.1f", p.FreeDistanceKm, p.OutAndReturnKm)
			}
			if len(p.SHA256) != 64 {
				t.Errorf("sha256 = %q", p.SHA256)
			}
		})
	}
}

func icao(p IGCPlace) string {
	if p.ICAO == nil {
		return ""
	}
	return *p.ICAO
}

func TestFlightFilePreview_Outlanding(t *testing.T) {
	t.Run("no airport database: never an outlanding", func(t *testing.T) {
		airports.SetTestDB(nil)
		svc, _, _ := newFlightFileTestService()
		p, err := svc.Preview(context.Background(), uuid.New(), igcFixture(t, "selflaunch.igc"))
		if err != nil {
			t.Fatal(err)
		}
		if p.Outlanding || p.Arrival.ICAO != nil {
			t.Errorf("outlanding = %v, arrival = %+v", p.Outlanding, p.Arrival)
		}
		if !strings.HasSuffix(p.Arrival.Label(), "E") || !strings.Contains(p.Arrival.Label(), "N ") {
			t.Errorf("label = %q", p.Arrival.Label())
		}
	})
	t.Run("landing near the take-off point of an unlisted site is not an outlanding", func(t *testing.T) {
		airports.SetTestDB(map[string]airports.AirportInfo{"EDDF": {Latitude: 50.03333, Longitude: 8.57056}})
		t.Cleanup(func() { airports.SetTestDB(nil) })
		svc, _, _ := newFlightFileTestService()
		p, err := svc.Preview(context.Background(), uuid.New(), igcFixture(t, "winch.igc"))
		if err != nil {
			t.Fatal(err)
		}
		if p.Outlanding || p.Departure.ICAO != nil {
			t.Errorf("outlanding = %v, departure = %+v", p.Outlanding, p.Departure)
		}
	})
}

func TestFlightFilePreview_MatchingFlight(t *testing.T) {
	withAirports(t)
	userID, otherID := uuid.New(), uuid.New()
	tests := []struct {
		name  string
		setup func(t *testing.T, repo *mockFlightRepo) *uuid.UUID
	}{
		{"overlapping times on the same date and glider", func(t *testing.T, repo *mockFlightRepo) *uuid.UUID {
			f := addFlight(t, repo, userID, "2026-06-15", "D-1234", "10:00", "10:10")
			return &f.ID
		}},
		{"registration spelled without hyphen still matches", func(t *testing.T, repo *mockFlightRepo) *uuid.UUID {
			f := addFlight(t, repo, userID, "2026-06-15", "D1234", "10:05", "10:30")
			return &f.ID
		}},
		{"flight without times matches on date and glider", func(t *testing.T, repo *mockFlightRepo) *uuid.UUID {
			f := addFlight(t, repo, userID, "2026-06-15", "D-1234", "", "")
			return &f.ID
		}},
		{"non-overlapping circuit does not match", func(t *testing.T, repo *mockFlightRepo) *uuid.UUID {
			addFlight(t, repo, userID, "2026-06-15", "D-1234", "11:00", "11:08")
			return nil
		}},
		{"other glider does not match", func(t *testing.T, repo *mockFlightRepo) *uuid.UUID {
			addFlight(t, repo, userID, "2026-06-15", "D-5678", "10:00", "10:10")
			return nil
		}},
		{"other date does not match", func(t *testing.T, repo *mockFlightRepo) *uuid.UUID {
			addFlight(t, repo, userID, "2026-06-16", "D-1234", "10:00", "10:10")
			return nil
		}},
		{"another user's flight does not match", func(t *testing.T, repo *mockFlightRepo) *uuid.UUID {
			addFlight(t, repo, otherID, "2026-06-15", "D-1234", "10:00", "10:10")
			return nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, flights := newFlightFileTestService()
			want := tt.setup(t, flights)
			p, err := svc.Preview(context.Background(), userID, igcFixture(t, "winch.igc"))
			if err != nil {
				t.Fatal(err)
			}
			switch {
			case want == nil && p.MatchingFlightID != nil:
				t.Errorf("matched %s", p.MatchingFlightID)
			case want != nil && (p.MatchingFlightID == nil || *p.MatchingFlightID != *want):
				t.Errorf("matched %v, want %s", p.MatchingFlightID, want)
			}
		})
	}
}

func TestFlightFilePreview_Rejects(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    error
		message string
	}{
		{"empty", nil, ErrFlightFileEmpty, ""},
		{"too large", bytes.Repeat([]byte("A"), models.MaxFlightFileBytes+1), ErrFlightFileTooLarge, ""},
		{"CSV", []byte("date,reg\n2026-01-01,D-1234\n"), ErrInvalidIGC, "does not start with an IGC A record"},
		{"binary", []byte("AXXX\n\x00\x01\x02"), ErrInvalidIGC, "line 2"},
		{"no date", []byte("AXXX\nB1000005000000N01000000EA0010000100\n"), ErrInvalidIGC, "HFDTE"},
		{"ground only", []byte("AXXX\nHFDTE010726\nB1000005000000N01000000EA0010000100\nB1000105000000N01000000EA0010000100\n"), ErrIGCNoFlight, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, _ := newFlightFileTestService()
			_, err := svc.Preview(context.Background(), uuid.New(), tt.data)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if tt.message != "" && !strings.Contains(err.Error(), tt.message) {
				t.Errorf("message %q lacks %q", err.Error(), tt.message)
			}
		})
	}
}

func TestFlightFileAttach(t *testing.T) {
	winch := igcFixture(t, "winch.igc")
	ctx := context.Background()
	userID := uuid.New()

	t.Run("stores the file with a sanitised name", func(t *testing.T) {
		svc, files, flights := newFlightFileTestService()
		f := addFlight(t, flights, userID, "2026-06-15", "D-1234", "", "")
		got, err := svc.Attach(ctx, userID, f.ID, `C:\logs\..\2026-06-15.igc`, winch)
		if err != nil {
			t.Fatalf("Attach: %v", err)
		}
		if got.Filename != "2026-06-15.igc" || got.Kind != models.FlightFileKindIGC || got.SizeBytes != len(winch) || got.Content != nil {
			t.Errorf("file = %+v", got)
		}
		stored := files.files[got.ID]
		if !bytes.Equal(stored.Content, winch) {
			t.Error("content not stored verbatim")
		}
	})
	t.Run("blank filename gets a default", func(t *testing.T) {
		svc, _, flights := newFlightFileTestService()
		f := addFlight(t, flights, userID, "2026-06-15", "D-1234", "", "")
		got, err := svc.Attach(ctx, userID, f.ID, "  ", winch)
		if err != nil || got.Filename != "flight.igc" {
			t.Fatalf("file = %+v, err = %v", got, err)
		}
	})

	tests := []struct {
		name  string
		setup func(svc *FlightFileService, files *mockFlightFileRepo, flights *mockFlightRepo) uuid.UUID
		data  []byte
		want  error
	}{
		{"another user's flight is not found", func(_ *FlightFileService, _ *mockFlightFileRepo, flights *mockFlightRepo) uuid.UUID {
			return addFlight(t, flights, uuid.New(), "2026-06-15", "D-1234", "", "").ID
		}, winch, ErrFlightNotFound},
		{"missing flight is not found", func(*FlightFileService, *mockFlightFileRepo, *mockFlightRepo) uuid.UUID {
			return uuid.New()
		}, winch, ErrFlightNotFound},
		{"not an IGC file", func(_ *FlightFileService, _ *mockFlightFileRepo, flights *mockFlightRepo) uuid.UUID {
			return addFlight(t, flights, userID, "2026-06-15", "D-1234", "", "").ID
		}, []byte("hello"), ErrInvalidIGC},
		{"same file twice on the flight", func(svc *FlightFileService, _ *mockFlightFileRepo, flights *mockFlightRepo) uuid.UUID {
			id := addFlight(t, flights, userID, "2026-06-15", "D-1234", "", "").ID
			if _, err := svc.Attach(ctx, userID, id, "a.igc", winch); err != nil {
				t.Fatal(err)
			}
			return id
		}, winch, ErrFlightFileDuplicate},
		{"same file already on another flight", func(svc *FlightFileService, _ *mockFlightFileRepo, flights *mockFlightRepo) uuid.UUID {
			first := addFlight(t, flights, userID, "2026-06-15", "D-1234", "", "").ID
			if _, err := svc.Attach(ctx, userID, first, "a.igc", winch); err != nil {
				t.Fatal(err)
			}
			return addFlight(t, flights, userID, "2026-06-15", "D-1234", "", "").ID
		}, winch, ErrFlightFileDuplicate},
		{"flight at the file cap", func(_ *FlightFileService, files *mockFlightFileRepo, flights *mockFlightRepo) uuid.UUID {
			files.createErr = repository.ErrFlightFileLimit
			return addFlight(t, flights, userID, "2026-06-15", "D-1234", "", "").ID
		}, winch, ErrFlightFileLimitReached},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, files, flights := newFlightFileTestService()
			flightID := tt.setup(svc, files, flights)
			_, err := svc.Attach(ctx, userID, flightID, "x.igc", tt.data)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("duplicate on another flight names that flight", func(t *testing.T) {
		svc, _, flights := newFlightFileTestService()
		first := addFlight(t, flights, userID, "2026-06-15", "D-1234", "", "").ID
		if _, err := svc.Attach(ctx, userID, first, "a.igc", winch); err != nil {
			t.Fatal(err)
		}
		second := addFlight(t, flights, userID, "2026-06-15", "D-1234", "", "").ID
		_, err := svc.Attach(ctx, userID, second, "a.igc", winch)
		var dup *IGCDuplicateError
		if !errors.As(err, &dup) || dup.FlightID != first {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestFlightFileListGetDelete_Ownership(t *testing.T) {
	ctx := context.Background()
	owner, stranger := uuid.New(), uuid.New()
	svc, _, flights := newFlightFileTestService()
	f := addFlight(t, flights, owner, "2026-06-15", "D-1234", "", "")
	file, err := svc.Attach(ctx, owner, f.ID, "w.igc", igcFixture(t, "winch.igc"))
	if err != nil {
		t.Fatal(err)
	}

	if list, err := svc.List(ctx, owner, f.ID); err != nil || len(list) != 1 || list[0].Content != nil {
		t.Fatalf("owner list = %v, %v", list, err)
	}
	if got, err := svc.Get(ctx, owner, f.ID, file.ID); err != nil || len(got.Content) == 0 {
		t.Fatalf("owner get = %v", err)
	}
	if _, err := svc.List(ctx, stranger, f.ID); !errors.Is(err, ErrFlightNotFound) {
		t.Errorf("stranger list err = %v", err)
	}
	if _, err := svc.Get(ctx, stranger, f.ID, file.ID); !errors.Is(err, ErrFlightNotFound) {
		t.Errorf("stranger get err = %v", err)
	}
	if err := svc.Delete(ctx, stranger, f.ID, file.ID); !errors.Is(err, ErrFlightNotFound) {
		t.Errorf("stranger delete err = %v", err)
	}
	if _, err := svc.Get(ctx, owner, f.ID, uuid.New()); !errors.Is(err, ErrFlightFileNotFound) {
		t.Errorf("missing file err = %v", err)
	}
	if err := svc.Delete(ctx, owner, f.ID, file.ID); err != nil {
		t.Fatalf("owner delete: %v", err)
	}
	if err := svc.Delete(ctx, owner, f.ID, file.ID); !errors.Is(err, ErrFlightFileNotFound) {
		t.Errorf("second delete err = %v", err)
	}
}
