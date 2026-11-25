package handlers

import (
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightcalc"
	"github.com/google/uuid"
)

func picDualLookup() map[string]generated.ImportColumnMapping {
	return toMappingLookup([]generated.ImportColumnMapping{
		{SourceColumn: "Date", TargetField: generated.ImportFieldDate},
		{SourceColumn: "Ident", TargetField: generated.ImportFieldAircraftReg},
		{SourceColumn: "Type", TargetField: generated.ImportFieldAircraftType},
		{SourceColumn: "From", TargetField: generated.ImportFieldDepartureIcao},
		{SourceColumn: "To", TargetField: generated.ImportFieldArrivalIcao},
		{SourceColumn: "Total", TargetField: generated.ImportFieldTotalTime},
		{SourceColumn: "PIC", TargetField: generated.ImportFieldIsPic},
		{SourceColumn: "Dual", TargetField: generated.ImportFieldIsDual},
		{SourceColumn: "Instructor", TargetField: generated.ImportFieldInstructorName},
	})
}

func TestMapImportRow_PICAndDualReceived_DeclaresPICWhileDual(t *testing.T) {
	row := map[string]string{
		"Date": "2026-03-14", "Ident": "N734TB", "Type": "C182", "From": "KFDK", "To": "KFDK",
		"Total": "1.6", "PIC": "1.6", "Dual": "1.6", "Instructor": "Jane Instructor",
	}
	flight, extras, errs := mapImportRow(row, picDualLookup(), nil, nil)
	if len(errs) > 0 {
		t.Fatalf("errs = %v", errs)
	}
	if flight.IsPic == nil || !*flight.IsPic {
		t.Fatalf("IsPic = %v, want true", flight.IsPic)
	}

	f := buildImportedFlight(uuid.New(), flight, extras)
	flightcalc.ApplyAutoCalculations(&f, "Tom Pilot", nil)
	if !f.IsPIC || !f.IsDual || f.PICTime != 96 || f.DualTime != 96 {
		t.Errorf("IsPIC=%v IsDual=%v PICTime=%d DualTime=%d, want true/true/96/96", f.IsPIC, f.IsDual, f.PICTime, f.DualTime)
	}
}

func TestMapImportRow_DualReceivedWithoutPIC_DualOnly(t *testing.T) {
	row := map[string]string{
		"Date": "2026-03-14", "Ident": "N734TB", "Type": "C182", "From": "KFDK", "To": "KFDK",
		"Total": "1.6", "PIC": "", "Dual": "1.6", "Instructor": "Jane Instructor",
	}
	flight, extras, errs := mapImportRow(row, picDualLookup(), nil, nil)
	if len(errs) > 0 {
		t.Fatalf("errs = %v", errs)
	}
	if flight.IsPic != nil {
		t.Fatalf("IsPic = %v, want unset", *flight.IsPic)
	}

	f := buildImportedFlight(uuid.New(), flight, extras)
	flightcalc.ApplyAutoCalculations(&f, "Tom Pilot", nil)
	if f.IsPIC || !f.IsDual || f.PICTime != 0 {
		t.Errorf("IsPIC=%v IsDual=%v PICTime=%d, want false/true/0", f.IsPIC, f.IsDual, f.PICTime)
	}
}

func TestDeclarePICWhileDual(t *testing.T) {
	yes, no := true, false
	f := models.Flight{IsPIC: true}
	declarePICWhileDual(&f, nil)
	if !f.IsPIC || f.IsDual {
		t.Errorf("nil: IsPIC=%v IsDual=%v, want unchanged true/false", f.IsPIC, f.IsDual)
	}
	declarePICWhileDual(&f, &yes)
	if !f.IsPIC || !f.IsDual {
		t.Errorf("true: IsPIC=%v IsDual=%v, want true/true", f.IsPIC, f.IsDual)
	}
	declarePICWhileDual(&f, &no)
	if f.IsPIC || f.IsDual {
		t.Errorf("false: IsPIC=%v IsDual=%v, want false/false", f.IsPIC, f.IsDual)
	}
}
