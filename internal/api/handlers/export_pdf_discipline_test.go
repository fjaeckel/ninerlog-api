package handlers

import (
	"bytes"
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightrules"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// lenaWinchCircuits returns n winch circuits of 8 minutes on D-1234, the last
// ending in an outlanding with a 400 m release.
func lenaWinchCircuits(n int) []*models.Flight {
	winch := "winch"
	site := "EDST"
	out := make([]*models.Flight, n)
	for i := range out {
		to := time.Date(2026, 5, 2, 10, 10*i, 0, 0, time.UTC)
		dep, arr := to.Format("15:04"), to.Add(8*time.Minute).Format("15:04")
		out[i] = &models.Flight{
			ID: uuid.New(), Date: time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC),
			AircraftReg: "D-1234", AircraftType: "ASK21",
			DepartureICAO: &site, ArrivalICAO: &site,
			DepartureTime: &dep, ArrivalTime: &arr,
			TotalTime: 8, PICTime: 8, LandingsDay: 1, TakeoffsDay: 1, Launches: 1,
			LaunchMethod: &winch,
		}
	}
	h := 400
	last := out[n-1]
	last.IsOutlanding = true
	last.ReleaseHeightM = &h
	return out
}

func mehmetFlights() ([]*models.Flight, map[string]*models.Aircraft) {
	ul, sep, three := "ULTRALIGHT", "SEP_LAND", models.ULKindThreeAxis
	strip := "UL-Platz Musterstadt"
	dep, arr := "09:00", "10:15"
	remark := flightrules.CreditedLabel + " Rundflug"
	flights := []*models.Flight{
		{ID: uuid.New(), Date: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), AircraftReg: "D-MXYZ", AircraftType: "C42",
			DepartureICAO: &strip, ArrivalICAO: &strip, OffBlockTime: &dep, OnBlockTime: &arr,
			TotalTime: 75, PICTime: 75, LandingsDay: 3},
		{ID: uuid.New(), Date: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), AircraftReg: "D-EABC", AircraftType: "C172",
			TotalTime: 60, PICTime: 60, LandingsDay: 1, Remarks: &remark},
		{ID: uuid.New(), Date: time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC), AircraftReg: "D-MTRK", AircraftType: "TRIKE",
			TotalTime: 30, PICTime: 30, LandingsDay: 1},
	}
	fleet := fleetByRegistration([]*models.Aircraft{
		{Registration: "D-MXYZ", AircraftClass: &ul, ULKind: &three},
		{Registration: "D-EABC", AircraftClass: &sep},
		{Registration: "D-MTRK", AircraftClass: &ul},
	})
	return flights, fleet
}

func TestSailplanePDF(t *testing.T) {
	g := geometryFor("a4")
	flights := lenaWinchCircuits(6)
	text := renderedText(t, renderSailplane(flights, g, "Lena Berger", nil, nil))

	t.Run("L5 sailplane PDF has launch-method and launches columns and no multi-pilot column", func(t *testing.T) {
		for _, want := range []string{"(METHOD)", "(LAUNCHES)", "(LAUNCH)", "(TAKE-OFF)", "(LANDING)", "(FLIGHT TIME)", "Part-SFCL"} {
			if !strings.Contains(text, want) {
				t.Errorf("sailplane PDF lacks %s", want)
			}
		}
		for _, absent := range []string{"MULTI-PILOT", "SINGLE-PILOT", "IFR", "NIGHT", "Multi-Pilot", "Night", "CO-PILOT"} {
			if strings.Contains(text, absent) {
				t.Errorf("sailplane PDF contains %q", absent)
			}
		}
	})

	t.Run("L5 row cells", func(t *testing.T) {
		for _, want := range []string{"(02.05.26)", "(ASK21)", "(D-1234)", "(EDST)", "(10:00)", "(10:08)", "(0:08)", "(Winch)"} {
			if !strings.Contains(text, want) {
				t.Errorf("sailplane PDF lacks row cell %s", want)
			}
		}
	})

	t.Run("L5 remarks carry outlanding and release but no launch marker", func(t *testing.T) {
		if !strings.Contains(text, "[Outlanding] [Release 400 m]") {
			t.Error("sailplane PDF lacks the outlanding and release markers")
		}
		if strings.Contains(text, "[Launch:") {
			t.Error("sailplane PDF carries the [Launch: ...] remarks marker")
		}
	})

	t.Run("L1 totals count launches and flight time", func(t *testing.T) {
		// Page total, grand total and summary total.
		if got := strings.Count(text, "(0:48)"); got < 3 {
			t.Errorf("0:48 appears %d times, want >= 3", got)
		}
		if !strings.Contains(text, "(Launches ") || !strings.Contains(text, "Winch)") {
			t.Error("summary lacks the launches-by-method row")
		}
		if !strings.Contains(text, "(Outlandings)") {
			t.Error("summary lacks the outlandings row")
		}
	})

	t.Run("A2 EASA layout keeps the launch marker", func(t *testing.T) {
		easa := renderedText(t, renderEASA(flights, g, nil, "Lena Berger", layoutSingle, nil, nil))
		if !strings.Contains(easa, "[Launch: winch]") {
			t.Error("EASA PDF lost the [Launch: winch] marker")
		}
		faa := renderedText(t, generateFAAPDF(flights, g, "Lena Berger", layoutSingle, nil, nil))
		if !strings.Contains(faa, "[Launch: winch]") {
			t.Error("FAA PDF lost the [Launch: winch] marker")
		}
	})

	t.Run("baseline opens the balance", func(t *testing.T) {
		b := &models.FlightBaseline{BaselineDate: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), TotalMinutes: 3_671, TotalFlights: 100}
		got := renderedText(t, renderSailplane(flights, g, "Lena Berger", b, nil))
		if !strings.Contains(got, "(61:59)") {
			t.Error("sailplane grand total does not include the 61:11 baseline")
		}
		if !strings.Contains(got, "Launch counts cover logged flights only.") {
			t.Error("summary lacks the launch-count disclosure")
		}
	})
}

func TestUltralightPDF(t *testing.T) {
	flights, fleet := mehmetFlights()
	text := renderedText(t, renderUltralight(flights, geometryFor("a4"), fleet, "Mehmet Yilmaz", nil, nil))

	t.Run("M UL layout shows kind", func(t *testing.T) {
		for _, want := range []string{"(UL KIND)", "(Three-axis)", "(D-MXYZ)", "(C42)", "(UL-Platz Musterstadt)", "(09:00)", "(10:15)", "(1:15)"} {
			if !strings.Contains(text, want) {
				t.Errorf("UL PDF lacks %s", want)
			}
		}
	})

	t.Run("M credited SEP flight shows its class and the credited marker", func(t *testing.T) {
		for _, want := range []string{"(SEP LAND)", flightrules.CreditedLabel + " Rundflug"} {
			if !strings.Contains(text, want) {
				t.Errorf("UL PDF lacks %s", want)
			}
		}
	})

	t.Run("S1 kindless ultralight prints UL", func(t *testing.T) {
		if !strings.Contains(text, "(UL)") {
			t.Error("UL PDF lacks the UL placeholder for D-MTRK")
		}
	})

	t.Run("S3 no IFR, multi-pilot, night or launch columns", func(t *testing.T) {
		for _, absent := range []string{"IFR", "MULTI-PILOT", "NIGHT", "Night", "LAUNCH", "SIC", "BLOCK"} {
			if strings.Contains(text, absent) {
				t.Errorf("UL PDF contains %q", absent)
			}
		}
	})

	t.Run("M totals and per-kind summary", func(t *testing.T) {
		if !strings.Contains(text, "(2:45)") {
			t.Error("UL PDF lacks the 2:45 grand total")
		}
		if !strings.Contains(text, "Three-axis)") || !strings.Contains(text, "(Total Landings)") {
			t.Error("UL summary lacks the per-kind or landings rows")
		}
	})
}

func TestDisciplinePDFPageCounts(t *testing.T) {
	const n = 45
	flights := buildSamplePDFFlights(n)
	for _, size := range []string{"a4", "a5", "letter"} {
		g := geometryFor(size)
		want := (n+g.logRowsPerPage()-1)/g.logRowsPerPage() + 1
		if got := renderSailplane(flights, g, "P", nil, nil).PageCount(); got != want {
			t.Errorf("%s sailplane: %d pages, want %d", size, got, want)
		}
		if got := renderUltralight(flights, g, nil, "P", nil, nil).PageCount(); got != want {
			t.Errorf("%s ultralight: %d pages, want %d", size, got, want)
		}
	}
	if got := renderSailplane(nil, geometryFor("a4"), "", nil, nil).PageCount(); got != 1 {
		t.Errorf("empty sailplane: %d pages, want 1", got)
	}
	if got := renderUltralight(nil, geometryFor("a4"), nil, "", nil, nil).PageCount(); got != 1 {
		t.Errorf("empty ultralight: %d pages, want 1", got)
	}
}

// exportWith runs GET /exports/pdf for the fixture with the given params.
func (fx *logbookPDFFixture) exportWith(t *testing.T, params generated.ExportFlightsPDFParams) (string, string) {
	t.Helper()
	w := httptest.NewRecorder()
	c := authenticatedContext(w, fx.userID)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/exports/pdf", nil)
	fx.h.ExportFlightsPDF(c, params)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	return pdfPlainText(t, w.Body.Bytes()), w.Header().Get("Content-Disposition")
}

func TestExportFlightsPDF_FormatByLicence(t *testing.T) {
	kind := func(k models.ULKind) *models.ULKind { return &k }
	format := func(f generated.ExportFlightsPDFParamsFormat) *generated.ExportFlightsPDFParamsFormat { return &f }

	type setup func(fx *logbookPDFFixture) uuid.UUID
	lena := func(fx *logbookPDFFixture) uuid.UUID {
		fx.addAircraft("D-1234", "GLIDER", nil)
		fx.addFlight("D-1234", "ASK21", "Thermik", "winch")
		return fx.license("EASA", "SPL", &models.ClassRating{ClassType: models.ClassTypeGlider})
	}
	mehmet := func(fx *logbookPDFFixture) uuid.UUID {
		fx.addAircraft("D-MXYZ", "ULTRALIGHT", kind(models.ULKindThreeAxis))
		fx.addFlight("D-MXYZ", "C42", "Platzrunden", "")
		return fx.license("DULV", "UL", &models.ClassRating{ClassType: models.ClassTypeUL, ULKind: kind(models.ULKindThreeAxis)})
	}
	anna := func(fx *logbookPDFFixture) uuid.UUID {
		fx.addAircraft("D-EABC", "SEP_LAND", nil)
		fx.addFlight("D-EABC", "C172", "", "")
		return fx.license("EASA", "PPL(A)", &models.ClassRating{ClassType: models.ClassTypeSEPLand})
	}

	tests := []struct {
		name     string
		setup    setup
		format   *generated.ExportFlightsPDFParamsFormat
		want     []string
		absent   []string
		filename string
	}{
		{"L5 SPL licence picks the sailplane layout", lena, nil,
			[]string{"Part-SFCL", "(LAUNCHES)", "(Winch)"}, []string{"MULTI-PILOT", "[Launch: winch]"}, "ninerlog_sailplane_a4_"},
		{"M DULV licence picks the ultralight layout", mehmet, nil,
			[]string{"(UL KIND)", "(Three-axis)"}, []string{"MULTI-PILOT", "IFR"}, "ninerlog_ultralight_a4_"},
		{"N PPL(A) licence keeps the EASA layout", anna, nil,
			[]string{"MULTI-PILOT", "FCL.050"}, []string{"Part-SFCL", "UL KIND"}, "ninerlog_easa_single_a4_"},
		{"L5 explicit easa wins over the SPL licence", lena, format(generated.ExportFlightsPDFParamsFormatEasa),
			[]string{"MULTI-PILOT", "[Launch: winch]"}, []string{"Part-SFCL"}, "ninerlog_easa_single_a4_"},
		{"explicit ultralight on an SPL licence", lena, format(generated.ExportFlightsPDFParamsFormatUltralight),
			[]string{"(UL KIND)"}, []string{"Part-SFCL"}, "ninerlog_ultralight_a4_"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newLogbookPDFFixture()
			id := openapi_types.UUID(tt.setup(fx))
			layout := generated.ExportFlightsPDFParamsLayout(layoutSingle)
			text, disposition := fx.exportWith(t, generated.ExportFlightsPDFParams{LogbookLicenseId: &id, Layout: &layout, Format: tt.format})
			for _, w := range tt.want {
				if !strings.Contains(text, w) {
					t.Errorf("PDF lacks %q", w)
				}
			}
			for _, a := range tt.absent {
				if strings.Contains(text, a) {
					t.Errorf("PDF contains %q", a)
				}
			}
			if !strings.Contains(disposition, tt.filename) {
				t.Errorf("Content-Disposition = %q, want %q", disposition, tt.filename)
			}
		})
	}

	t.Run("A2 no licence and no format stays EASA", func(t *testing.T) {
		fx := newLogbookPDFFixture()
		lena(fx)
		text, _ := fx.exportWith(t, generated.ExportFlightsPDFParams{})
		if !strings.Contains(text, "FCL.050") || strings.Contains(text, "Part-SFCL") {
			t.Error("export without a licence is not the EASA layout")
		}
	})
}

func TestWriteStandardCSV_AircraftClassColumns(t *testing.T) {
	flights, fleet := mehmetFlights()
	glider := "GLIDER"
	fleet["D-1234"] = &models.Aircraft{Registration: "D-1234", AircraftClass: &glider}
	flights = append(flights,
		&models.Flight{Date: time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC), AircraftReg: "D-1234", AircraftType: "ASK21"},
		&models.Flight{Date: time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC), AircraftReg: "D-XXXX", AircraftType: "DR40"},
	)
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	writeStandardCSV(w, flights, exportPrefs{}, fleet)
	csvWrite(w, standardCSVTotals(flights, exportPrefs{}))
	w.Flush()
	records, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	header := records[0]
	n := len(header)
	if header[n-2] != "AircraftClass" || header[n-1] != "ULKind" {
		t.Fatalf("header ends %v, want AircraftClass, ULKind", header[n-2:])
	}
	tests := []struct {
		name, reg, class, kind string
	}{
		{"M three-axis ultralight", "D-MXYZ", "ULTRALIGHT", "THREE_AXIS"},
		{"SEP aircraft has no kind", "D-EABC", "SEP_LAND", ""},
		{"S1 kindless ultralight", "D-MTRK", "ULTRALIGHT", ""},
		{"L glider", "D-1234", "GLIDER", ""},
		{"not in fleet", "D-XXXX", "", ""},
	}
	byReg := map[string][]string{}
	for _, r := range records[1 : len(records)-1] {
		byReg[r[1]] = r
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := byReg[tt.reg]
			if r == nil {
				t.Fatalf("no row for %s", tt.reg)
			}
			if r[n-2] != tt.class || r[n-1] != tt.kind {
				t.Errorf("class/kind = %q/%q, want %q/%q", r[n-2], r[n-1], tt.class, tt.kind)
			}
		})
	}
	if got := len(records[len(records)-1]); got != n {
		t.Errorf("totals row has %d cells, header %d", got, n)
	}
}
