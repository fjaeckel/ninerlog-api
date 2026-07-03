package handlers

import (
	"bytes"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

func TestLaunchAbbrev(t *testing.T) {
	tests := []struct {
		method *string
		want   string
	}{
		{nil, ""},
		{strPtr("winch"), "W"},
		{strPtr("aerotow"), "A"},
		{strPtr("self-launch"), "S"},
		{strPtr("bungee"), "B"},
		{strPtr("auto-tow"), "T"},
		{strPtr("mystery"), "MYSTERY"},
	}
	for _, tt := range tests {
		got := launchAbbrev(tt.method)
		if got != tt.want {
			label := "nil"
			if tt.method != nil {
				label = *tt.method
			}
			t.Errorf("launchAbbrev(%s) = %q, want %q", label, got, tt.want)
		}
	}
}

func TestFmtReleaseAltitude(t *testing.T) {
	tests := []struct {
		alt  *int
		ref  *string
		want string
	}{
		{nil, nil, ""},
		{intPtr(450), strPtr("AGL"), "450 AGL"},
		{intPtr(900), strPtr("AMSL"), "900 AMSL"},
		{intPtr(300), nil, "300"},
		{intPtr(300), strPtr(""), "300"},
	}
	for _, tt := range tests {
		if got := fmtReleaseAltitude(tt.alt, tt.ref); got != tt.want {
			t.Errorf("fmtReleaseAltitude = %q, want %q", got, tt.want)
		}
	}
}

func TestRenderGliderPDF(t *testing.T) {
	date := time.Date(2026, 5, 26, 0, 0, 0, 0, time.UTC)
	flights := []*models.Flight{
		{
			ID:                 uuid.New(),
			Date:               date,
			AircraftReg:        "D-1234",
			AircraftType:       "ASK 21",
			DepartureICAO:      strPtr("EDKA"),
			ArrivalICAO:        strPtr("EDKA"),
			DepartureTime:      strPtr("10:15:00"),
			ArrivalTime:        strPtr("10:55:00"),
			TotalTime:          40,
			IsPIC:              true,
			LaunchMethod:       strPtr("winch"),
			Launches:           3,
			ReleaseAltitude:    intPtr(420),
			ReleaseAltitudeRef: strPtr("AGL"),
		},
		{
			ID:            uuid.New(),
			Date:          date,
			AircraftReg:   "D-5678",
			AircraftType:  "Duo Discus",
			DepartureICAO: strPtr("EDKA"),
			ArrivalICAO:   strPtr("EDKA"),
			TotalTime:     185,
			IsPIC:         true,
			LaunchMethod:  strPtr("aerotow"),
			Launches:      1,
		},
	}

	geom := geometryFor("A4")
	pdf := renderGlider(flights, geom, "A Pilot")
	if pdf == nil {
		t.Fatal("renderGlider returned nil")
	}
	if pdf.Err() {
		t.Fatalf("pdf error: %v", pdf.Error())
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		t.Fatalf("pdf output: %v", err)
	}
	if buf.Len() < 1000 {
		t.Errorf("expected a non-trivial PDF, got %d bytes", buf.Len())
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF")) {
		t.Error("output is not a PDF (missing %PDF header)")
	}
}

func TestRenderGliderPDF_Empty(t *testing.T) {
	geom := geometryFor("A4")
	pdf := renderGlider(nil, geom, "A Pilot")
	if pdf == nil {
		t.Fatal("renderGlider returned nil for empty input")
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		t.Fatalf("pdf output: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected a PDF even with no flights (summary page)")
	}
}
