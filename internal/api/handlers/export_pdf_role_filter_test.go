package handlers

import (
	"bytes"
	"compress/zlib"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/google/uuid"
)

// inflatePDFText concatenates every inflated content stream of a rendered
// PDF, which is where the cell text of the logbook rows lives.
func inflatePDFText(raw []byte) string {
	var out bytes.Buffer
	const open, close = "\nstream\n", "\nendstream"
	for pos := 0; ; {
		i := bytes.Index(raw[pos:], []byte(open))
		if i < 0 {
			break
		}
		start := pos + i + len(open)
		j := bytes.Index(raw[start:], []byte(close))
		if j < 0 {
			break
		}
		body := raw[start : start+j]
		pos = start + j + len(close)
		r, err := zlib.NewReader(bytes.NewReader(body))
		if err != nil {
			out.Write(body)
			continue
		}
		data, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			continue
		}
		out.Write(data)
	}
	return out.String()
}

// exportPDFForFlights runs the export handler over exactly `flights` and
// returns the recorder.
func exportPDFForFlights(t *testing.T, flights []*models.Flight, params generated.ExportFlightsPDFParams) *httptest.ResponseRecorder {
	t.Helper()
	h, userRepo := setupTestHandler()

	userID := uuid.New()
	userRepo.users[userID] = &models.User{ID: userID, Email: "pdf-role-filter@example.com"}

	repo := newMockFlightRepo()
	for _, f := range flights {
		f.ID = uuid.New()
		f.UserID = userID
		repo.flights[f.ID] = f
	}
	h.flightService = service.NewFlightService(repo, nil)

	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("GET", "/exports/pdf", nil)
	h.ExportFlightsPDF(c, params)
	return w
}

// picAndSICFlights returns one PIC, one dual-received and one co-pilot-only
// flight, each on its own registration.
func picAndSICFlights() []*models.Flight {
	date := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	dep, arr := "EDDF", "EDDS"
	return []*models.Flight{
		{
			Date: date, AircraftReg: "DEPICX", AircraftType: "C172",
			DepartureICAO: &dep, ArrivalICAO: &arr,
			TotalTime: 90, IsPIC: true, PICTime: 90, LandingsDay: 1,
		},
		{
			Date: date.AddDate(0, 0, 1), AircraftReg: "DEDUAL", AircraftType: "PA28",
			DepartureICAO: &dep, ArrivalICAO: &arr,
			TotalTime: 60, IsDual: true, DualTime: 60, LandingsDay: 1,
		},
		{
			Date: date.AddDate(0, 0, 2), AircraftReg: "DESICX", AircraftType: "DA42",
			DepartureICAO: &dep, ArrivalICAO: &arr,
			TotalTime: 120, SICTime: 120, MultiPilotTime: 120, LandingsDay: 1,
		},
	}
}

// TestExportFlightsPDF_OmitsCoPilotOnlyFlights asserts a flight logged
// neither as PIC nor as dual never reaches the rendered logbook, in any
// format.
func TestExportFlightsPDF_OmitsCoPilotOnlyFlights(t *testing.T) {
	for _, format := range []generated.ExportFlightsPDFParamsFormat{"easa", "faa", "summary"} {
		t.Run(string(format), func(t *testing.T) {
			f := format
			w := exportPDFForFlights(t, picAndSICFlights(), generated.ExportFlightsPDFParams{Format: &f})
			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}
			text := inflatePDFText(w.Body.Bytes())
			if bytes.Contains([]byte(text), []byte("DESICX")) {
				t.Error("co-pilot-only flight was rendered into the PDF")
			}
			if format == "summary" {
				return
			}
			for _, reg := range []string{"DEPICX", "DEDUAL"} {
				if !bytes.Contains([]byte(text), []byte(reg)) {
					t.Errorf("%s flight missing from the PDF", reg)
				}
			}
		})
	}
}

// TestExportFlightsPDF_SummaryDropsCoPilotOnlyTime asserts the totals
// summary counts only the flights the logbook pages show.
func TestExportFlightsPDF_SummaryDropsCoPilotOnlyTime(t *testing.T) {
	format := generated.ExportFlightsPDFParamsFormat("summary")
	w := exportPDFForFlights(t, picAndSICFlights(), generated.ExportFlightsPDFParams{Format: &format})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	text := inflatePDFText(w.Body.Bytes())
	// 90 + 60 logged, the 120-minute co-pilot flight excluded.
	if !bytes.Contains([]byte(text), []byte("2:30")) {
		t.Error("total block time should cover the PIC and dual flights only")
	}
	if bytes.Contains([]byte(text), []byte("4:30")) {
		t.Error("total block time still includes the co-pilot-only flight")
	}
}
