package handlers

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/pkg/jwt"
	"github.com/google/uuid"
)

func TestMapRowToFlight_VereinsfliegerLaunchMethod(t *testing.T) {
	tests := []struct {
		code string
		want string
	}{
		{"W", "winch"},
		{"F", "aerotow"},
		{"E", "self-launch"},
		{"A", "car"},
		{"G", "bungee"},
		{"w", "winch"},
		{"Winde", "winch"},
		{"X", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run("S.-Art "+tt.code, func(t *testing.T) {
			row := map[string]string{
				"Datum": "09.05.2026", "Lfz.": "D-1234", "Pilot": "Berger, Lena",
				"Begleiter/FI": "", "Start": "10:02", "Landung": "10:10", "Flugzeit": "8",
				"Startort": "Hartenholm EDHM", "Landeort": "Hartenholm EDHM", "Landungen": "1",
				"S.-Art": tt.code, "Flugart": "N", "Abr.": "K", "Verein": "LSV", "Bemerkung": "",
			}
			flight, errs := mapRowToFlight(row, mappingsFor("VEREINSFLIEGER_CSV", headersOf(row)), nil)
			if len(errs) > 0 {
				t.Fatalf("an unrecognised launch code must not fail the row: %+v", errs)
			}
			got := ""
			if flight.LaunchMethod != nil {
				got = string(*flight.LaunchMethod)
			}
			if got != tt.want {
				t.Errorf("launchMethod = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMapRowToFlight_LaunchRemarkMarker(t *testing.T) {
	tests := []struct {
		name        string
		remarks     string
		launchCol   string
		wantMethod  string
		wantRemarks *string
	}{
		{"marker alone", "[Launch: winch]", "", "winch", nil},
		{"marker after remarks", "Thermik [Launch: aerotow]", "", "aerotow", launchStrp("Thermik")},
		{"column wins over marker", "Thermik [Launch: aerotow]", "winch", "winch", launchStrp("Thermik")},
		{"no marker", "Thermik", "", "", launchStrp("Thermik")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := map[string]string{
				"Date": "2026-05-09", "Registration": "D-1234", "Type": "ASK21",
				"From": "EDHM", "To": "EDHM", "Total Time": "0:08",
				"Remarks": tt.remarks, "Launch Method": tt.launchCol,
			}
			flight, errs := mapRowToFlight(row, mappingsFor("CSV", headersOf(row)), nil)
			if len(errs) > 0 {
				t.Fatalf("errors: %+v", errs)
			}
			got := ""
			if flight.LaunchMethod != nil {
				got = string(*flight.LaunchMethod)
			}
			if got != tt.wantMethod {
				t.Errorf("launchMethod = %q, want %q", got, tt.wantMethod)
			}
			if safeStr(flight.Remarks) != safeStr(tt.wantRemarks) || (flight.Remarks == nil) != (tt.wantRemarks == nil) {
				t.Errorf("remarks = %v, want %v", flight.Remarks, tt.wantRemarks)
			}
		})
	}
}

func TestCollectTowedLaunchRegs(t *testing.T) {
	headers := []string{"Datum", "Lfz.", "Start", "Landung", "Flugzeit", "Startort", "Landeort", "Landungen", "S.-Art", "Abr.", "Verein", "Flugart", "Begleiter/FI"}
	mappings := mappingsFor("VEREINSFLIEGER_CSV", headers)
	row := func(reg, code string) map[string]string {
		return map[string]string{"Datum": "09.05.2026", "Lfz.": reg, "Flugzeit": "8", "Startort": "EDHM", "Landeort": "EDHM", "S.-Art": code}
	}
	rows := []map[string]string{
		row("D-1234", "W"),
		row("d-ktow", "F"),
		row("D-KLSG", "E"),
		row("D-5678", "X"),
		row("D-MXYZ", "G"),
	}

	got := collectTowedLaunchRegs(rows, mappings, nil)
	for reg, want := range map[string]bool{"D-1234": true, "D-KTOW": true, "D-KLSG": false, "D-5678": false, "D-MXYZ": true} {
		if got[reg] != want {
			t.Errorf("towed[%s] = %v, want %v", reg, got[reg], want)
		}
	}

	selected := collectTowedLaunchRegs(rows, mappings, func(rowIdx int) bool { return rowIdx != 1 })
	if selected["D-1234"] {
		t.Error("a deselected row must not mark its aircraft as towed")
	}
}

type stubFlightImportRepo struct {
	repository.FlightImportRepository
	created []*models.FlightImport
}

func (s *stubFlightImportRepo) Create(_ context.Context, rec *models.FlightImport) error {
	s.created = append(s.created, rec)
	return nil
}

// TestConfirmImport_L2VereinsfliegerGliders imports the Vereinsflieger glider
// sample through ConfirmImport: launch methods come from S.-Art and the
// auto-created aircraft are classed from the registration or the launch.
func TestConfirmImport_L2VereinsfliegerGliders(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(importSamplesDir, "vereinsflieger-glider.csv"))
	if err != nil {
		t.Fatal(err)
	}
	columns, rows, aircraft, err := parseCSV(data)
	if err != nil {
		t.Fatal(err)
	}
	_, format, _ := detectImportFormat(columns)
	if format != "VEREINSFLIEGER_CSV" {
		t.Fatalf("format = %s, want VEREINSFLIEGER_CSV", format)
	}

	aircraftRepo := newMockAircraftRepo()
	flightRepo := newMockFlightRepo()
	userRepo := newHandlerMockUserRepo()
	jwtMgr := jwt.NewManager("test-access", "test-refresh", 15*time.Minute, 7*24*time.Hour)
	twoFactorSvc := service.NewTwoFactorService(userRepo, jwtMgr, nil)
	h := &APIHandler{
		authService: service.NewAuthService(userRepo, newHandlerMockRefreshTokenRepo(), &mockPasswordResetRepo{},
			&mockEmailVerificationRepo{}, jwtMgr, twoFactorSvc, service.SessionPolicy{}),
		aircraftService:  service.NewAircraftService(aircraftRepo),
		contactService:   service.NewContactService(newMockContactRepo()),
		flightService:    service.NewFlightService(flightRepo, nil),
		flightImportRepo: &stubFlightImportRepo{},
	}

	userID := uuid.New()
	existingClass := "TMG"
	if err := aircraftRepo.Create(context.Background(), &models.Aircraft{
		UserID: userID, Registration: "D-5678", Type: "LS4", Make: "Rolladen-Schneider", Model: "LS4",
		AircraftClass: &existingClass,
	}); err != nil {
		t.Fatal(err)
	}

	token := newUploadToken()
	if err := storeSession(token, &uploadSession{
		userID: userID, fileName: "vereinsflieger-glider.csv", format: format,
		columns: columns, rows: rows, aircraft: aircraft, createdAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(generated.ConfirmImportJSONRequestBody{UploadToken: token})
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest(http.MethodPost, "/imports/confirm", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.ConfirmImport(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	var res generated.ImportResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.ImportedCount != 7 || res.ErrorCount != 0 {
		t.Fatalf("imported %d, errors %d (%v), want 7 and 0", res.ImportedCount, res.ErrorCount, res.Errors)
	}

	launches := map[string]int{}
	for _, f := range flightRepo.flights {
		m := ""
		if f.LaunchMethod != nil {
			m = *f.LaunchMethod
		}
		launches[f.AircraftReg+" "+m]++
	}
	wantLaunches := map[string]int{
		"D-1234 winch":       2,
		"D-5678 aerotow":     1,
		"D-KLSG self-launch": 1,
		"D-KTOW aerotow":     1,
		"D-MXYZ self-launch": 1,
		"D-1234 ":            1,
	}
	for k, n := range wantLaunches {
		if launches[k] != n {
			t.Errorf("flights %q = %d, want %d (all: %v)", k, launches[k], n, launches)
		}
	}

	classes := map[string]string{}
	for _, a := range aircraftRepo.aircraft {
		cl := "<unset>"
		if a.AircraftClass != nil {
			cl = *a.AircraftClass
		}
		classes[a.Registration] = cl
		if a.ULKind != nil {
			t.Errorf("%s: ul_kind = %s, want unset", a.Registration, *a.ULKind)
		}
	}
	wantClasses := map[string]string{
		"D-1234": "GLIDER",
		"D-5678": "TMG",
		"D-KLSG": "<unset>",
		"D-KTOW": "GLIDER",
		"D-MXYZ": "ULTRALIGHT",
	}
	for reg, want := range wantClasses {
		if classes[reg] != want {
			t.Errorf("aircraft %s class = %s, want %s", reg, classes[reg], want)
		}
	}
	if res.AircraftCreated == nil || *res.AircraftCreated != 4 {
		t.Errorf("aircraftCreated = %v, want 4", res.AircraftCreated)
	}
}

func TestWriteStandardCSV_LaunchMethodColumn(t *testing.T) {
	winch := "winch"
	flights := []*models.Flight{
		{Date: time.Date(2026, 5, 9, 0, 0, 0, 0, time.UTC), AircraftReg: "D-1234", AircraftType: "ASK21", LaunchMethod: &winch},
		{Date: time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC), AircraftReg: "D-EABC", AircraftType: "C172"},
	}
	data := exportCSV(t, "standard", flights, exportPrefs{DateFormat: "YYYY-MM-DD", DecimalSeparator: "dot"})
	records, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	header := records[0]
	col := -1
	for i, h := range header {
		if h == "LaunchMethod" {
			col = i
		}
	}
	if want := []string{"LaunchMethod", "Launches", "Outlanding", "TowFlight", "ReleaseHeightM", "AircraftClass", "ULKind"}; col < 0 || !slices.Equal(header[col:], want) {
		t.Fatalf("header ends %v, want %v", header[max(col, 0):], want)
	}
	if records[1][col] != "winch" || records[2][col] != "" {
		t.Errorf("launch cells = %q/%q, want winch/empty", records[1][col], records[2][col])
	}
	if got := len(standardCSVTotals(flights, exportPrefs{})); got != len(header) {
		t.Errorf("totals row has %d cells, header %d", got, len(header))
	}
}

func TestRemarksLayouts_CarryLaunchMethod(t *testing.T) {
	aerotow := "aerotow"
	remarks := "Thermik"
	f := &models.Flight{
		Date: time.Date(2026, 5, 9, 0, 0, 0, 0, time.UTC), AircraftReg: "D-5678", AircraftType: "LS4",
		Remarks: &remarks, LaunchMethod: &aerotow, TotalTime: 135,
	}
	for _, layout := range []string{"easa", "faa", "weblogbook"} {
		t.Run(layout+" CSV", func(t *testing.T) {
			data := exportCSV(t, layout, []*models.Flight{f}, exportPrefs{DateFormat: "YYYY-MM-DD", DecimalSeparator: "dot"})
			if !strings.Contains(string(data), "Thermik [Launch: aerotow]") {
				t.Errorf("remarks cell does not carry the launch method:\n%s", data)
			}
		})
	}
	t.Run("L5 EASA PDF remarks", func(t *testing.T) {
		rows := buildEASARows([]*models.Flight{f}, map[string]string{"D-5678": "GLIDER"}, "Lena Berger", 200)
		if rows[0].remarks != "Thermik [Launch: aerotow]" {
			t.Errorf("PDF remarks = %q, want %q", rows[0].remarks, "Thermik [Launch: aerotow]")
		}
	})
}

func TestExportImportRoundTrip_LaunchMethod(t *testing.T) {
	src := roundTripSourceFlight()
	winch := "winch"
	src.LaunchMethod = &winch
	prefs := exportPrefs{DateFormat: "YYYY-MM-DD", DecimalSeparator: "dot"}

	for _, layout := range []string{"standard", "easa", "faa", "weblogbook"} {
		t.Run(layout, func(t *testing.T) {
			data := exportCSV(t, layout, []*models.Flight{src}, prefs)
			columns, rows, _, err := parseCSV(data)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			_, _, mappings := detectImportFormat(columns)
			got, errs := mapRowToFlight(rows[0], toMappingLookup(mappings), nil)
			if len(errs) > 0 {
				t.Fatalf("errors: %+v", errs)
			}
			if got.LaunchMethod == nil || string(*got.LaunchMethod) != "winch" {
				t.Errorf("launchMethod = %v, want winch", got.LaunchMethod)
			}
			if got.Remarks == nil || strings.Contains(*got.Remarks, "[Launch:") {
				t.Errorf("remarks = %v, want the launch marker removed", got.Remarks)
			}
		})
	}
}

func launchStrp(s string) *string { return &s }

// TestConfirmImport_A2LaunchMethodOnlyOnSailplanes: Vereinsflieger "E" on a
// powered aircraft imports with no launch method; sailplane-type and
// unclassified aircraft keep theirs.
func TestConfirmImport_A2LaunchMethodOnlyOnSailplanes(t *testing.T) {
	data := []byte(`"Datum";"Lfz.";"Pilot";"Begleiter/FI";"Start";"Landung";"Flugzeit";"Startort";"Landeort";"Landungen";"S.-Art";"Flugart";"Abr.";"Verein";"Bemerkung"
"14.03.2026";"D-EABC";"Rivera, Alex";"";"09:12";"10:47";"95";"Uetersen EDHE";"Stade EDHS";"1";"E";"N";"K";"Aero-Club";""
"15.03.2026";"D-ESEP";"Rivera, Alex";"";"09:12";"10:47";"95";"Uetersen EDHE";"Stade EDHS";"1";"E";"N";"K";"Aero-Club";"Rundflug [Launch: self-launch]"
"16.03.2026";"D-MTRK";"Rivera, Alex";"";"09:12";"09:47";"35";"Uetersen EDHE";"Uetersen EDHE";"1";"E";"N";"K";"Aero-Club";""
"17.03.2026";"D-KLSG";"Rivera, Alex";"";"09:12";"10:12";"60";"Uetersen EDHE";"Uetersen EDHE";"1";"E";"N";"K";"Aero-Club";""
"18.03.2026";"D-MXYZ";"Rivera, Alex";"";"09:12";"09:52";"40";"Uetersen EDHE";"Uetersen EDHE";"1";"W";"N";"K";"Aero-Club";""
`)
	columns, rows, aircraft, err := parseCSV(data)
	if err != nil {
		t.Fatal(err)
	}
	_, format, _ := detectImportFormat(columns)

	aircraftRepo := newMockAircraftRepo()
	flightRepo := newMockFlightRepo()
	userRepo := newHandlerMockUserRepo()
	jwtMgr := jwt.NewManager("test-access", "test-refresh", 15*time.Minute, 7*24*time.Hour)
	twoFactorSvc := service.NewTwoFactorService(userRepo, jwtMgr, nil)
	h := &APIHandler{
		authService: service.NewAuthService(userRepo, newHandlerMockRefreshTokenRepo(), &mockPasswordResetRepo{},
			&mockEmailVerificationRepo{}, jwtMgr, twoFactorSvc, service.SessionPolicy{}),
		aircraftService:  service.NewAircraftService(aircraftRepo),
		contactService:   service.NewContactService(newMockContactRepo()),
		flightService:    service.NewFlightService(flightRepo, nil),
		flightImportRepo: &stubFlightImportRepo{},
	}
	userID := uuid.New()
	sep, ul := "SEP_LAND", "ULTRALIGHT"
	trike := models.ULKindWeightShift
	for _, a := range []*models.Aircraft{
		{UserID: userID, Registration: "D-EABC", Type: "C172", Make: "Cessna", Model: "172", AircraftClass: &sep},
		{UserID: userID, Registration: "D-ESEP", Type: "C172", Make: "Cessna", Model: "172", AircraftClass: &sep},
		{UserID: userID, Registration: "D-MTRK", Type: "TRIKE", Make: "Air Creation", Model: "Tanarg", AircraftClass: &ul, ULKind: &trike},
	} {
		if err := aircraftRepo.Create(context.Background(), a); err != nil {
			t.Fatal(err)
		}
	}

	token := newUploadToken()
	if err := storeSession(token, &uploadSession{
		userID: userID, fileName: "vf.csv", format: format,
		columns: columns, rows: rows, aircraft: aircraft, createdAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(generated.ConfirmImportJSONRequestBody{UploadToken: token})
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest(http.MethodPost, "/imports/confirm", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.ConfirmImport(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}

	want := map[string]string{
		"D-EABC": "",
		"D-ESEP": "",
		"D-MTRK": "",
		"D-KLSG": "self-launch",
		"D-MXYZ": "winch",
	}
	for _, f := range flightRepo.flights {
		got := ""
		if f.LaunchMethod != nil {
			got = *f.LaunchMethod
		}
		if got != want[f.AircraftReg] {
			t.Errorf("%s launchMethod = %q, want %q", f.AircraftReg, got, want[f.AircraftReg])
		}
		if f.Remarks != nil && strings.Contains(*f.Remarks, "[Launch:") {
			t.Errorf("%s remarks = %q, want the marker removed", f.AircraftReg, *f.Remarks)
		}
	}
}

func TestMapRowToFlight_GliderFacts(t *testing.T) {
	tests := []struct {
		name           string
		cells          map[string]string
		wantLaunches   *int
		wantOutlanding bool
		wantTow        bool
		wantHeight     *int
		wantErrField   string
	}{
		{
			name:         "L1 series of six launches",
			cells:        map[string]string{"Launches": "6"},
			wantLaunches: intPtr(6),
		},
		{
			name:           "P1 outlanding and release height",
			cells:          map[string]string{"Outlanding": "ja", "ReleaseHeightM": "450"},
			wantOutlanding: true,
			wantHeight:     intPtr(450),
		},
		{
			name:    "tow flight",
			cells:   map[string]string{"TowFlight": "true"},
			wantTow: true,
		},
		{
			name:         "release height above bound is a row error",
			cells:        map[string]string{"ReleaseHeightM": "20001"},
			wantErrField: "releaseHeightM",
		},
		{
			name:         "negative launches is a row error",
			cells:        map[string]string{"Launches": "-1"},
			wantErrField: "launches",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := map[string]string{
				"Date": "2026-05-09", "AircraftID": "D-1234", "AircraftType": "ASK21",
				"From": "EDHM", "To": "EDHM", "TimeOff": "10:02", "TimeOn": "10:10", "TotalTime": "0.13",
				"AllLandings": "1", "LaunchMethod": "winch",
			}
			for k, v := range tt.cells {
				row[k] = v
			}
			flight, errs := mapRowToFlight(row, mappingsFor("NINERLOG_CSV", headersOf(row)), nil)
			if tt.wantErrField != "" {
				found := false
				for _, e := range errs {
					if e.field == tt.wantErrField {
						found = true
					}
				}
				if !found {
					t.Errorf("errors = %+v, want one on %s", errs, tt.wantErrField)
				}
				return
			}
			if len(errs) > 0 {
				t.Fatalf("errors = %+v", errs)
			}
			if (flight.Launches == nil) != (tt.wantLaunches == nil) || (flight.Launches != nil && *flight.Launches != *tt.wantLaunches) {
				t.Errorf("launches = %v, want %v", flight.Launches, tt.wantLaunches)
			}
			if (flight.IsOutlanding != nil && *flight.IsOutlanding) != tt.wantOutlanding {
				t.Errorf("isOutlanding = %v, want %t", flight.IsOutlanding, tt.wantOutlanding)
			}
			if (flight.IsTowFlight != nil && *flight.IsTowFlight) != tt.wantTow {
				t.Errorf("isTowFlight = %v, want %t", flight.IsTowFlight, tt.wantTow)
			}
			if (flight.ReleaseHeightM == nil) != (tt.wantHeight == nil) || (flight.ReleaseHeightM != nil && *flight.ReleaseHeightM != *tt.wantHeight) {
				t.Errorf("releaseHeightM = %v, want %v", flight.ReleaseHeightM, tt.wantHeight)
			}
		})
	}
}
