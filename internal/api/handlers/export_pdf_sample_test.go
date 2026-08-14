package handlers

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/go-pdf/fpdf"
)

// TestGenerateSamplePDFs is a manual sample-generator. Skipped unless
// GENERATE_SAMPLE_PDFS=1 is set.
func TestGenerateSamplePDFs(t *testing.T) {
	if os.Getenv("GENERATE_SAMPLE_PDFS") != "1" {
		t.Skip("set GENERATE_SAMPLE_PDFS=1 to produce sample PDFs")
	}

	flights := buildSamplePDFFlights(45)

	// Samples carry prior experience so the carried-forward opening balance,
	// the footer disclosure and the summary note are all visible for review.
	sampleBaseline := &models.FlightBaseline{
		BaselineDate:        time.Date(2019, 3, 12, 0, 0, 0, 0, time.UTC),
		TotalFlights:        412,
		TotalMinutes:        73_920,
		PICMinutes:          51_300,
		SICMinutes:          4_140,
		DualMinutes:         18_480,
		DualGivenMinutes:    2_760,
		MultiPilotMinutes:   9_600,
		NightMinutes:        6_840,
		IFRMinutes:          11_100,
		SoloMinutes:         7_260,
		CrossCountryMinutes: 29_400,
		LandingsDay:         690,
		LandingsNight:       84,
	}

	outDir := os.Getenv("SAMPLE_PDF_DIR")
	if outDir == "" {
		outDir = "/tmp/ninerlog-pdf-samples"
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		fmtName  string
		layout   string
		pageSize string
	}{
		{"easa", "spread", "a4"},
		{"easa", "spread", "a5"},
		{"easa", "spread", "letter"},
		{"easa", "single", "a4"},
		{"easa", "single", "a5"},
		{"easa", "single", "letter"},
		{"faa", "spread", "a4"},
		{"faa", "spread", "a5"},
		{"faa", "spread", "letter"},
		{"faa", "single", "a4"},
		{"faa", "single", "a5"},
		{"faa", "single", "letter"},
		{"summary", "", "a4"},
	}

	for _, c := range cases {
		geom := geometryFor(c.pageSize)
		path := fmt.Sprintf("%s/sample_%s_%s_%s.pdf", outDir, c.fmtName, c.layout, c.pageSize)
		if c.fmtName == "summary" {
			path = fmt.Sprintf("%s/sample_summary_%s.pdf", outDir, c.pageSize)
		}
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		var doc *fpdf.Fpdf
		switch c.fmtName {
		case "faa":
			doc = generateFAAPDF(flights, geom, "Sample Pilot", c.layout, sampleBaseline)
		case "summary":
			doc = generateSummaryPDF(flights, geom, "Sample Pilot", sampleBaseline)
		case "easa":
			doc = renderEASA(flights, geom, map[string]string{}, "Sample Pilot", c.layout, sampleBaseline)
		}
		if err := doc.Output(f); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", path)
	}
}

func buildSamplePDFFlights(n int) []*models.Flight {
	out := make([]*models.Flight, 0, n)
	start := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	regs := []string{"D-EABC", "D-EFGH", "N123AB", "G-XYZW", "F-GTUV"}
	types := []string{"C172", "PA28", "DA40", "C152", "SR22"}
	deps := []string{"EDDF", "EDDM", "EDDB", "LFPG", "EHAM"}
	arrs := []string{"EDDS", "EDDH", "LFPO", "EBBR", "EDDK"}
	pics := []string{"SELF", "John Doe", "Maria Schmidt", "SELF", "Pierre Dupont"}
	for i := 0; i < n; i++ {
		date := start.AddDate(0, 0, i)
		off := "10:30:00"
		on := "12:15:00"
		dep := deps[i%len(deps)]
		arr := arrs[i%len(arrs)]
		pic := pics[i%len(pics)]
		pp := pic
		picPtr := &pp
		rem := "Training flight \u2014 pattern work"
		var endo *string
		if i%7 == 0 {
			e := "BFR/IPC"
			endo = &e
		}
		var fstd *string
		var simT int
		if i%9 == 0 {
			s := "FNPT II"
			fstd = &s
			simT = 60
		}
		f := &models.Flight{
			Date:                    date,
			AircraftReg:             regs[i%len(regs)],
			AircraftType:            types[i%len(types)],
			DepartureICAO:           &dep,
			ArrivalICAO:             &arr,
			OffBlockTime:            &off,
			OnBlockTime:             &on,
			TotalTime:               60 + (i%5)*15,
			PICTime:                 60 + (i%5)*15,
			NightTime:               (i % 4) * 10,
			IFRTime:                 (i % 3) * 20,
			LandingsDay:             1 + (i % 3),
			LandingsNight:           i % 2,
			SoloTime:                (i % 2) * 30,
			CrossCountryTime:        60,
			SimulatedFlightTime:     simT,
			FSTDType:                fstd,
			ActualInstrumentTime:    (i % 3) * 5,
			SimulatedInstrumentTime: (i % 4) * 10,
			Holds:                   i % 2,
			ApproachesCount:         i % 3,
			IsIPC:                   i%11 == 0,
			IsFlightReview:          i%13 == 0,
			Endorsements:            endo,
			PICName:                 picPtr,
			Remarks:                 &rem,
		}
		out = append(out, f)
	}
	return out
}
