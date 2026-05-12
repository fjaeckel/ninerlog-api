package handlers

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

// TestGenerateExportDemoPDF is a local-only helper that generates a PDF export
// with many PIC-name variations so the fix can be visually verified. It is
// skipped unless the NINERLOG_EXPORT_DEMO=1 env var is set.
//
// Run with:
//
//	NINERLOG_EXPORT_DEMO=1 go test ./internal/api/handlers/ -run TestGenerateExportDemoPDF -v
//
// PDFs are written to /tmp/ninerlog-export-demo/ (or $NINERLOG_EXPORT_DIR).
func TestGenerateExportDemoPDF(t *testing.T) {
	if os.Getenv("NINERLOG_EXPORT_DEMO") != "1" {
		t.Skip("set NINERLOG_EXPORT_DEMO=1 to run this local demo helper")
	}

	outDir := os.Getenv("NINERLOG_EXPORT_DIR")
	if outDir == "" {
		outDir = "/tmp/ninerlog-export-demo"
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	userName := "Amelia Earhart"
	flights := buildPICNameVariations(userName)
	t.Logf("generated %d demo flights", len(flights))

	regToClass := map[string]string{
		"D-EABC": "SEP",
		"D-EXYZ": "SEP",
		"N12345": "SEP",
		"D-IMEP": "MEP",
		"D-ESET": "SET",
	}

	geom := geometryFor("A4")

	// EASA two-page spread (the format that showed the SELF bug).
	easaPDF := renderEASA(flights, geom, regToClass, userName)
	easaPath := filepath.Join(outDir, "easa-pic-name-variations.pdf")
	if err := easaPDF.OutputFileAndClose(easaPath); err != nil {
		t.Fatalf("write EASA PDF: %v", err)
	}
	t.Logf("EASA PDF:    %s", easaPath)

	// FAA layout.
	faaPDF := generateFAAPDF(flights, geom)
	faaPath := filepath.Join(outDir, "faa-pic-name-variations.pdf")
	if err := faaPDF.OutputFileAndClose(faaPath); err != nil {
		t.Fatalf("write FAA PDF: %v", err)
	}
	t.Logf("FAA PDF:     %s", faaPath)

	// Summary layout.
	sumPDF := generateSummaryPDF(flights, geom)
	sumPath := filepath.Join(outDir, "summary-pic-name-variations.pdf")
	if err := sumPDF.OutputFileAndClose(sumPath); err != nil {
		t.Fatalf("write summary PDF: %v", err)
	}
	t.Logf("Summary PDF: %s", sumPath)
}

// buildPICNameVariations returns a deterministic list of flights covering all
// the data shapes the PIC-name resolver has to handle. The Remarks column
// describes the scenario so it can be cross-checked against the rendered "PIC
// Name" column in the PDF.
func buildPICNameVariations(userName string) []*models.Flight {
	s := func(v string) *string { return &v }
	dep := s("EDDF")
	arr := s("EDDM")
	off := s("10:00:00")
	on := s("11:30:00")

	day := func(n int) time.Time {
		return time.Date(2025, time.January, n, 0, 0, 0, 0, time.UTC)
	}

	base := func(n int, reg, ty, scenario string) *models.Flight {
		return &models.Flight{
			Date:          day(n),
			AircraftReg:   reg,
			AircraftType:  ty,
			DepartureICAO: dep,
			ArrivalICAO:   arr,
			OffBlockTime:  off,
			OnBlockTime:   on,
			TotalTime:     90,
			LandingsDay:   1,
			Remarks:       s(scenario),
		}
	}

	crewInstr := func(name string) []models.FlightCrewMember {
		return []models.FlightCrewMember{{Name: name, Role: models.CrewRoleInstructor}}
	}
	crewSIC := func(name string) []models.FlightCrewMember {
		return []models.FlightCrewMember{{Name: name, Role: models.CrewRoleSIC}}
	}

	var fs []*models.Flight

	// 1. Solo PIC: no instructor anywhere → SELF
	f := base(1, "D-EABC", "C172", "1) solo PIC → SELF")
	f.IsPIC = true
	f.PICTime = 90
	fs = append(fs, f)

	// 2. Solo PIC with persisted "Self" → Self
	f = base(2, "D-EABC", "C172", "2) solo PIC + stored Self → Self")
	f.IsPIC = true
	f.PICTime = 90
	f.PICName = s("Self")
	fs = append(fs, f)

	// 3. Dual receiving with crew instructor (modern shape) → instructor
	f = base(3, "D-EABC", "C172", "3) Dual + crew instructor → CFI Mueller")
	f.IsDual = true
	f.DualTime = 90
	f.CrewMembers = crewInstr("CFI Mueller")
	fs = append(fs, f)

	// 4. Dual receiving with legacy InstructorName (pre-crew-table) → instructor
	f = base(4, "D-EABC", "C172", "4) Dual + legacy InstructorName → Legacy CFI")
	f.IsDual = true
	f.DualTime = 90
	f.InstructorName = s("Legacy CFI")
	fs = append(fs, f)

	// 5. BUG REPRO #1: Dual + stored "Self" + crew instructor → instructor wins
	f = base(5, "D-EABC", "C172", "5) Dual + stale Self + crew instructor → CFI Mueller")
	f.IsDual = true
	f.DualTime = 90
	f.PICName = s("Self")
	f.CrewMembers = crewInstr("CFI Mueller")
	fs = append(fs, f)

	// 6. BUG REPRO #2: Dual + stored "SELF" (uppercase) + legacy instructor
	f = base(6, "D-EABC", "C172", "6) Dual + stale SELF + legacy instructor → Legacy CFI")
	f.IsDual = true
	f.DualTime = 90
	f.PICName = s("SELF")
	f.InstructorName = s("Legacy CFI")
	fs = append(fs, f)

	// 7. BUG REPRO #3: non-Dual flag mismatch + crew instructor → instructor wins
	f = base(7, "D-EABC", "C172", "7) IsDual=false + Self + crew instructor → CFI Mueller (legacy mismatch)")
	f.IsPIC = true
	f.PICTime = 90
	f.PICName = s("Self")
	f.CrewMembers = crewInstr("CFI Mueller")
	fs = append(fs, f)

	// 8. Crew instructor IS the user → SELF (instructor flying with self does not override)
	f = base(8, "D-EABC", "C172", "8) crew instructor = self → SELF")
	f.IsDual = true
	f.DualTime = 90
	f.CrewMembers = crewInstr(userName)
	fs = append(fs, f)

	// 9. Explicit third-party PIC name (e.g. ATPL line check) wins over everything
	f = base(9, "D-IMEP", "BE58", "9) explicit PICName CPT Doe wins")
	f.IsPIC = false
	f.PICName = s("CPT Doe")
	f.MultiPilotTime = 90
	f.CrewMembers = crewInstr("CFI Mueller")
	fs = append(fs, f)

	// 10. Dual with crew instructor in "Lastname, Firstname" form
	f = base(10, "D-EABC", "C172", "10) Dual + crew instructor reversed name → Mueller, CFI")
	f.IsDual = true
	f.DualTime = 90
	f.CrewMembers = crewInstr("Mueller, CFI")
	fs = append(fs, f)

	// 11. Dual + stale Self + NO instructor info → keep Self
	f = base(11, "D-EABC", "C172", "11) Dual + stale Self + no instructor → Self (no info to recover)")
	f.IsDual = true
	f.DualTime = 90
	f.PICName = s("Self")
	fs = append(fs, f)

	// 12. SIC crew member present, user is PIC → SELF (SIC is not PIC)
	f = base(12, "D-IMEP", "BE58", "12) PIC + SIC crew → SELF")
	f.IsPIC = true
	f.PICTime = 90
	f.MultiPilotTime = 90
	f.CrewMembers = crewSIC("FO Smith")
	fs = append(fs, f)

	// 13. Multi-pilot with stored MP PIC name
	f = base(13, "D-IMEP", "BE58", "13) MP flight with explicit PIC → CPT Doe")
	f.PICName = s("CPT Doe")
	f.MultiPilotTime = 90
	fs = append(fs, f)

	return fs
}
