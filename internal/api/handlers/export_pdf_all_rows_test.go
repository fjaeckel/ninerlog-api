package handlers

import (
	"bytes"
	"compress/zlib"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
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

// A printed logbook covers every row the holder logged. The EASA form has a
// CO-PILOT column (AMC1 FCL.050 col 16), and co-pilot time is part of total
// time of flight, so a flight flown purely as co-pilot belongs on the sheet
// and in the totals like any other.
func TestExportFlightsPDF_PrintsEveryLoggedFlight(t *testing.T) {
	for _, format := range []generated.ExportFlightsPDFParamsFormat{"easa", "faa", "summary"} {
		t.Run(string(format), func(t *testing.T) {
			f := format
			w := exportPDFForFlights(t, picAndSICFlights(), generated.ExportFlightsPDFParams{Format: &f})
			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}
			if format == "summary" {
				return
			}
			text := inflatePDFText(w.Body.Bytes())
			for _, reg := range []string{"DEPICX", "DEDUAL", "DESICX"} {
				if !bytes.Contains([]byte(text), []byte(reg)) {
					t.Errorf("%s flight missing from the PDF", reg)
				}
			}
		})
	}
}

// The totals summary covers the whole logbook: 90 + 60 + 120 minutes.
func TestExportFlightsPDF_SummaryTotalsEveryFlight(t *testing.T) {
	format := generated.ExportFlightsPDFParamsFormat("summary")
	w := exportPDFForFlights(t, picAndSICFlights(), generated.ExportFlightsPDFParams{Format: &format})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	text := inflatePDFText(w.Body.Bytes())
	if !bytes.Contains([]byte(text), []byte("4:30")) {
		t.Error("total block time should cover all three flights (4:30)")
	}
	if bytes.Contains([]byte(text), []byte("2:30")) {
		t.Error("total block time drops the co-pilot flight; co-pilot time is total time")
	}
}

// An FSTD session belongs on the sheet too: AMC1 FCL.050 gives it cols
// 20-22, and it must reach them without adding anything to TOTAL TIME.
func TestExportFlightsPDF_PrintsFSTDSessionWithoutFlightTime(t *testing.T) {
	fstd := "FNPT II"
	date := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	dep, arr := "EDDF", "EDDS"
	flights := []*models.Flight{
		{
			Date: date, AircraftReg: "DEPICX", AircraftType: "C172",
			DepartureICAO: &dep, ArrivalICAO: &arr,
			TotalTime: 90, IsPIC: true, PICTime: 90, LandingsDay: 1,
		},
		{
			Date: date.AddDate(0, 0, 1), AircraftType: "PA34",
			IsSimulator: true, FSTDType: &fstd, SimulatedFlightTime: 120,
		},
	}

	format := generated.ExportFlightsPDFParamsFormat("easa")
	layout := generated.ExportFlightsPDFParamsLayout("single")
	w := exportPDFForFlights(t, flights, generated.ExportFlightsPDFParams{Format: &format, Layout: &layout})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	text := inflatePDFText(w.Body.Bytes())

	if !bytes.Contains([]byte(text), []byte("FNPT II")) {
		t.Error("FSTD session missing from the printed logbook; AMC1 cols 20-22 never render")
	}
	page := strings.Join(pdfDrawnCells(text), " ")

	// The session must reach the FSTD columns (AMC1 cols 20-22).
	if !strings.Contains(page, "FNPT II") || !strings.Contains(page, "2:00") {
		t.Errorf("FSTD session missing from the printed logbook:\n%s", page)
	}
	// TOTAL TIME covers the 1:30 flight; the 2:00 session adds nothing.
	if !strings.Contains(page, "1:30") {
		t.Error("TOTAL TIME should cover the 90-minute flight")
	}
	if strings.Contains(page, "3:30") {
		t.Error("TOTAL TIME includes the FSTD session; session time is never summed with flight time")
	}
	// A device session has no pilot-in-command.
	if strings.Contains(page, "PA34 SELF") {
		t.Error("session row prints SELF in the NAME PIC column")
	}
}

// pdfDrawnCells returns the strings a rendered PDF actually draws, i.e. the
// operands of its Tj text operators. Raw stream bytes are full of
// coordinates and font sizes, so matching numbers against them is
// meaningless; the drawn cells are the page as a reader sees it.
func pdfDrawnCells(streamText string) []string {
	re := regexp.MustCompile(`\(((?:[^()\\]|\\.)*)\)Tj`)
	var out []string
	for _, m := range re.FindAllStringSubmatch(streamText, -1) {
		out = append(out, m[1])
	}
	return out
}
