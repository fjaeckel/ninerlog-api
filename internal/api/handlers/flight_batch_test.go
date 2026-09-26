package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func postFlightBatch(t *testing.T, h *APIHandler, userID *uuid.UUID, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	var c *gin.Context
	if userID != nil {
		c = authenticatedContext(w, *userID)
	} else {
		c, _ = gin.CreateTestContext(w)
	}
	c.Request = httptest.NewRequest("POST", "/flights/batch", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.CreateFlightBatch(c)
	return w
}

// winchLegs returns n 8-minute legs starting at 10:00 UTC.
func winchLegs(n int) string {
	legs := make([]string, n)
	for i := range legs {
		start := 10*60 + i*10
		legs[i] = fmt.Sprintf(`{"departureTime":"%02d:%02d:00","arrivalTime":"%02d:%02d:00"}`,
			start/60, start%60, (start+8)/60, (start+8)%60)
	}
	return "[" + strings.Join(legs, ",") + "]"
}

const lenaTemplate = `{"date":"2026-05-09","aircraftReg":"D-1234","aircraftType":"ASK21",
	"departureIcao":"Hausen am Albis","arrivalIcao":"Hausen am Albis","launchMethod":"winch"}`

func TestCreateFlightBatch_Handler(t *testing.T) {
	userID := uuid.New()
	tests := []struct {
		name       string
		userID     *uuid.UUID
		body       string
		wantStatus int
		wantMsg    string
		wantLegs   int
	}{
		{
			name:       "L1 six winch circuits",
			userID:     &userID,
			body:       `{"template":` + lenaTemplate + `,"legs":` + winchLegs(6) + `}`,
			wantStatus: http.StatusCreated,
			wantLegs:   6,
		},
		{
			name:       "unauthenticated",
			body:       `{"template":` + lenaTemplate + `,"legs":` + winchLegs(1) + `}`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "malformed body",
			userID:     &userID,
			body:       `{bad`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "no legs",
			userID:     &userID,
			body:       `{"template":` + lenaTemplate + `,"legs":[]}`,
			wantStatus: http.StatusBadRequest,
			wantMsg:    "legs must hold between 1 and 50 entries",
		},
		{
			name:       "51 legs",
			userID:     &userID,
			body:       `{"template":` + lenaTemplate + `,"legs":` + winchLegs(51) + `}`,
			wantStatus: http.StatusBadRequest,
			wantMsg:    "legs must hold between 1 and 50 entries",
		},
		{
			name:   "leg without landing time names its index",
			userID: &userID,
			body: `{"template":` + lenaTemplate + `,"legs":[{"departureTime":"10:00:00","arrivalTime":"10:08:00"},` +
				`{"departureTime":"10:10:00"}]}`,
			wantStatus: http.StatusBadRequest,
			wantMsg:    "Leg 1:",
		},
		{
			name:   "invalid release height on a leg's template names leg 0",
			userID: &userID,
			body: `{"template":{"date":"2026-05-09","aircraftReg":"D-1234","aircraftType":"ASK21",
				"departureIcao":"EDxx","arrivalIcao":"EDxx","releaseHeightM":20001},"legs":` + winchLegs(2) + `}`,
			wantStatus: http.StatusBadRequest,
			wantMsg:    "Leg 0: release height must be between 0 and 20000 m",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := setupTestHandler()
			w := postFlightBatch(t, h, tt.userID, tt.body)
			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", w.Code, tt.wantStatus, w.Body.String())
			}
			if tt.wantMsg != "" && !strings.Contains(w.Body.String(), tt.wantMsg) {
				t.Errorf("body = %s, want it to contain %q", w.Body.String(), tt.wantMsg)
			}
			if tt.wantStatus != http.StatusCreated {
				if tt.userID != nil {
					n, _ := h.flightService.CountFlights(t.Context(), *tt.userID, nil)
					if n != 0 {
						t.Errorf("%d flights stored after a rejected batch, want 0", n)
					}
				}
				return
			}
			var res generated.FlightBatchResult
			if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
				t.Fatal(err)
			}
			if len(res.Flights) != tt.wantLegs {
				t.Fatalf("got %d flights, want %d", len(res.Flights), tt.wantLegs)
			}
			total, launches := 0, 0
			for _, f := range res.Flights {
				total += f.TotalTime
				launches += f.Launches
				if f.LaunchMethod == nil || *f.LaunchMethod != "winch" || f.AllLandings != 1 || f.LaunchesOverride {
					t.Errorf("leg = %+v, want a winch launch with one landing and derived launches", f)
				}
			}
			if total != 8*tt.wantLegs || launches != tt.wantLegs {
				t.Errorf("total %d min, %d launches; want %d min, %d launches", total, launches, 8*tt.wantLegs, tt.wantLegs)
			}
		})
	}
}

func TestBatchLegCreate(t *testing.T) {
	str := func(s string) *string { return &s }
	num := func(n int) *int { return &n }
	tests := []struct {
		name         string
		template     generated.FlightCreate
		leg          generated.FlightBatchLeg
		wantLandings *int
		wantLaunches *int
		wantRemarks  *string
		wantDep      *string
	}{
		{
			name:         "leg times replace the template's and landings default to 1",
			template:     generated.FlightCreate{DepartureTime: str("09:00:00"), Remarks: str("Thermik")},
			leg:          generated.FlightBatchLeg{DepartureTime: str("10:00:00"), ArrivalTime: str("10:08:00")},
			wantLandings: num(1),
			wantRemarks:  str("Thermik"),
			wantDep:      str("10:00:00"),
		},
		{
			name:         "template landings apply to every leg",
			template:     generated.FlightCreate{Landings: num(2)},
			leg:          generated.FlightBatchLeg{},
			wantLandings: num(2),
		},
		{
			name:         "leg landings, launches and remarks win",
			template:     generated.FlightCreate{Landings: num(2), Remarks: str("Thermik")},
			leg:          generated.FlightBatchLeg{Landings: num(3), Launches: num(4), Remarks: str("Seilriss")},
			wantLandings: num(3),
			wantLaunches: num(4),
			wantRemarks:  str("Seilriss"),
		},
	}
	eqInt := func(a, b *int) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }
	eqStr := func(a, b *string) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := batchLegCreate(tt.template, tt.leg)
			if !eqInt(got.Landings, tt.wantLandings) || !eqInt(got.Launches, tt.wantLaunches) ||
				!eqStr(got.Remarks, tt.wantRemarks) || !eqStr(got.DepartureTime, tt.wantDep) {
				t.Errorf("merged = %+v", got)
			}
		})
	}
}
