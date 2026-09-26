package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/airports"
	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type mockFlightFileRepo struct {
	files map[uuid.UUID]*models.FlightFile
}

func (m *mockFlightFileRepo) Create(_ context.Context, f *models.FlightFile, maxPerFlight int) error {
	n := 0
	for _, s := range m.files {
		if s.FlightID == f.FlightID {
			n++
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
			out = append(out, f)
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

func setupIgcHandler(t *testing.T) (*APIHandler, *mockFlightRepo, *mockAircraftRepo) {
	t.Helper()
	h, _ := setupTestHandler()
	flights := newMockFlightRepo()
	aircraft := newMockAircraftRepo()
	h.flightService = service.NewFlightService(flights, nil)
	h.aircraftService = service.NewAircraftService(aircraft)
	h.SetFlightFileService(service.NewFlightFileService(&mockFlightFileRepo{files: map[uuid.UUID]*models.FlightFile{}}, flights))
	airports.SetTestDB(map[string]airports.AirportInfo{
		"EDER": {Name: "Wasserkuppe", Latitude: 50.49889, Longitude: 9.95389},
	})
	t.Cleanup(func() { airports.SetTestDB(nil) })
	return h, flights, aircraft
}

func igcFixtureBytes(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "pkg", "igc", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func multipartRequest(t *testing.T, path string, file []byte, fields map[string]string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if file != nil {
		part, err := w.CreateFormFile("file", "flight.igc")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write(file)
	}
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func TestPreviewIgcFlight_Statuses(t *testing.T) {
	tests := []struct {
		name    string
		file    []byte
		noFile  bool
		status  int
		message string
	}{
		{name: "winch fixture", file: igcFixtureBytes(t, "winch.igc"), status: http.StatusOK},
		{name: "no file field", noFile: true, status: http.StatusBadRequest, message: "A file field is required"},
		{name: "not IGC", file: []byte("date,reg\n"), status: http.StatusBadRequest, message: "does not start with an IGC A record"},
		{name: "binary", file: []byte("AXXX\n\x00\x00"), status: http.StatusBadRequest, message: "line 2"},
		{name: "no take-off", file: []byte("AXXX\nHFDTE010726\nB1000005000000N01000000EA0010000100\n"), status: http.StatusBadRequest, message: "no take-off"},
		{name: "too large", file: bytes.Repeat([]byte("A"), models.MaxFlightFileBytes+1), status: http.StatusRequestEntityTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _, _ := setupIgcHandler(t)
			w := httptest.NewRecorder()
			c := authenticatedContext(w, uuid.New())
			file := tt.file
			if tt.noFile {
				file = nil
			}
			c.Request = multipartRequest(t, "/flights/igc/preview", file, nil)
			h.PreviewIgcFlight(c)
			if w.Code != tt.status {
				t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
			}
			if tt.message != "" && !strings.Contains(w.Body.String(), tt.message) {
				t.Errorf("body %s lacks %q", w.Body.String(), tt.message)
			}
			if tt.status == http.StatusOK {
				var p generated.IgcFlightPreview
				_ = json.Unmarshal(w.Body.Bytes(), &p)
				if p.LaunchMethod != "winch" || p.ReleaseHeightM == nil || p.Arrival.Icao == nil || *p.Arrival.Icao != "EDER" {
					t.Errorf("preview = %+v", p)
				}
			}
		})
	}
}

func importIgc(t *testing.T, h *APIHandler, userID uuid.UUID, file []byte, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = multipartRequest(t, "/flights/igc", file, fields)
	h.ImportIgcFlight(c)
	return w
}

func TestImportIgcFlight_CreatesFlight(t *testing.T) {
	h, _, aircraft := setupIgcHandler(t)
	userID := uuid.New()
	w := importIgc(t, h, userID, igcFixtureBytes(t, "selflaunch.igc"), nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	var res generated.IgcImportResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	f := res.Flight
	if f.AircraftReg != "D-KXYZ" || f.AircraftType != "ASG 29E" {
		t.Errorf("aircraft = %s %s", f.AircraftReg, f.AircraftType)
	}
	if f.LaunchMethod == nil || string(*f.LaunchMethod) != "self-launch" {
		t.Errorf("launch method = %v", f.LaunchMethod)
	}
	if !f.IsOutlanding {
		t.Errorf("outlanding = %v", f.IsOutlanding)
	}
	if f.ReleaseHeightM == nil || *f.ReleaseHeightM < 700 {
		t.Errorf("release height = %v", f.ReleaseHeightM)
	}
	if f.TotalTime != 51 || f.DepartureIcao == nil || *f.DepartureIcao != "EDER" {
		t.Errorf("total %d, departure %v", f.TotalTime, f.DepartureIcao)
	}
	if f.ArrivalIcao == nil || !strings.HasSuffix(*f.ArrivalIcao, "E") {
		t.Errorf("arrival = %v", f.ArrivalIcao)
	}
	if res.Summary.FreeDistanceKm < 39 {
		t.Errorf("free distance = %.1f", res.Summary.FreeDistanceKm)
	}
	var created *models.Aircraft
	for _, a := range aircraft.aircraft {
		if a.Registration == "D-KXYZ" {
			created = a
		}
	}
	if created == nil || created.AircraftClass == nil || *created.AircraftClass != string(models.ClassTypeGlider) {
		t.Fatalf("aircraft = %+v", created)
	}

	again := importIgc(t, h, userID, igcFixtureBytes(t, "selflaunch.igc"), nil)
	if again.Code != http.StatusConflict {
		t.Errorf("re-import status = %d", again.Code)
	}
}

func TestImportIgcFlight_Statuses(t *testing.T) {
	winch := igcFixtureBytes(t, "winch.igc")
	noReg := bytes.Replace(winch, []byte("HFGIDGLIDERID:D-1234"), []byte("HFGIDGLIDERID:"), 1)
	tests := []struct {
		name   string
		file   []byte
		fields func(h *APIHandler, flights *mockFlightRepo, userID uuid.UUID) map[string]string
		status int
	}{
		{"attach to own flight", winch, func(_ *APIHandler, flights *mockFlightRepo, userID uuid.UUID) map[string]string {
			f := &models.Flight{UserID: userID, Date: time.Now(), AircraftReg: "D-1234", AircraftType: "ASK 21", TotalTime: 7}
			_ = flights.Create(context.Background(), f)
			return map[string]string{"flightId": f.ID.String()}
		}, http.StatusCreated},
		{"attach to another user's flight", winch, func(_ *APIHandler, flights *mockFlightRepo, _ uuid.UUID) map[string]string {
			f := &models.Flight{UserID: uuid.New(), Date: time.Now(), AircraftReg: "D-1234", AircraftType: "ASK 21", TotalTime: 7}
			_ = flights.Create(context.Background(), f)
			return map[string]string{"flightId": f.ID.String()}
		}, http.StatusNotFound},
		{"attach to a missing flight", winch, func(*APIHandler, *mockFlightRepo, uuid.UUID) map[string]string {
			return map[string]string{"flightId": uuid.NewString()}
		}, http.StatusNotFound},
		{"malformed flightId", winch, func(*APIHandler, *mockFlightRepo, uuid.UUID) map[string]string {
			return map[string]string{"flightId": "nope"}
		}, http.StatusBadRequest},
		{"no registration to create from", noReg, nil, http.StatusBadRequest},
		{"not IGC", []byte("hello"), nil, http.StatusBadRequest},
		{"too large", bytes.Repeat([]byte("A"), models.MaxFlightFileBytes+1), nil, http.StatusRequestEntityTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, flights, _ := setupIgcHandler(t)
			userID := uuid.New()
			var fields map[string]string
			if tt.fields != nil {
				fields = tt.fields(h, flights, userID)
			}
			w := importIgc(t, h, userID, tt.file, fields)
			if w.Code != tt.status {
				t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestFlightFileDownloadAndDelete(t *testing.T) {
	h, _, _ := setupIgcHandler(t)
	owner, stranger := uuid.New(), uuid.New()
	data := igcFixtureBytes(t, "winch.igc")
	w := importIgc(t, h, owner, data, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("import status = %d %s", w.Code, w.Body.String())
	}
	var res generated.IgcImportResult
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	flightID, fileID := res.Flight.Id, res.FileId

	get := func(user uuid.UUID) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c := authenticatedContext(w, user)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		h.GetFlightFile(c, flightID, fileID)
		return w
	}
	dl := get(owner)
	if dl.Code != http.StatusOK || !bytes.Equal(dl.Body.Bytes(), data) {
		t.Fatalf("download status = %d, %d bytes", dl.Code, dl.Body.Len())
	}
	if ct := dl.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("content type = %q", ct)
	}
	if cd := dl.Header().Get("Content-Disposition"); cd != `attachment; filename="flight.igc"` {
		t.Errorf("disposition = %q", cd)
	}
	if got := get(stranger); got.Code != http.StatusNotFound {
		t.Errorf("stranger download status = %d", got.Code)
	}

	lw := httptest.NewRecorder()
	h.ListFlightFiles(authenticatedContextWithRequest(lw, stranger), flightID)
	if lw.Code != http.StatusNotFound {
		t.Errorf("stranger list status = %d", lw.Code)
	}

	del := func(user uuid.UUID) int {
		w := httptest.NewRecorder()
		c := authenticatedContextWithRequest(w, user)
		h.DeleteFlightFile(c, flightID, fileID)
		return c.Writer.Status()
	}
	if code := del(stranger); code != http.StatusNotFound {
		t.Errorf("stranger delete status = %d", code)
	}
	if code := del(owner); code != http.StatusNoContent {
		t.Errorf("owner delete status = %d", code)
	}
	if code := del(owner); code != http.StatusNotFound {
		t.Errorf("second delete status = %d", code)
	}
}

func authenticatedContextWithRequest(w *httptest.ResponseRecorder, userID uuid.UUID) *gin.Context {
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	return c
}
