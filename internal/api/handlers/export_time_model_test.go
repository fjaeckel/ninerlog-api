package handlers

import (
	"encoding/csv"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

func TestExport_TimeModelClocks(t *testing.T) {
	s := func(v string) *string { return &v }
	takeoffLanding := &models.Flight{
		Date: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), AircraftReg: "D-1234", AircraftType: "ASK21",
		DepartureICAO: s("EDBO"), ArrivalICAO: s("EDBO"),
		DepartureTime: s("10:05:00"), ArrivalTime: s("10:47:00"),
		TotalTime: 42, PICTime: 42, IsPIC: true, AllLandings: 1, LandingsDay: 1,
	}
	block := &models.Flight{
		Date: time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC), AircraftReg: "D-EFGH", AircraftType: "C172",
		DepartureICAO: s("EDDF"), ArrivalICAO: s("EDDH"),
		OffBlockTime: s("08:00:00"), OnBlockTime: s("09:30:00"),
		DepartureTime: s("08:10:00"), ArrivalTime: s("09:20:00"),
		TotalTime: 90, PICTime: 90, IsPIC: true, AllLandings: 1, LandingsDay: 1,
	}
	flights := []*models.Flight{takeoffLanding, block}

	tests := []struct {
		name           string
		write          func(w *csv.Writer)
		depCol, arrCol string
		want           [][2]string
	}{
		{"easa", func(w *csv.Writer) { writeEASACSV(w, flights, exportPrefs{}, "Pilot") },
			"Dep Time", "Arr Time", [][2]string{{"10:05", "10:47"}, {"08:00", "09:30"}}},
		{"weblogbook", func(w *csv.Writer) { writeWebLogbookCSV(w, flights, "Pilot") },
			"Departure Time", "Arrival Time", [][2]string{{"1005", "1047"}, {"0800", "0930"}}},
		{"standard keeps block columns empty", func(w *csv.Writer) { writeStandardCSV(w, flights, exportPrefs{}) },
			"TimeOut", "TimeIn", [][2]string{{"", ""}, {"08:00:00", "09:30:00"}}},
		{"standard take-off/landing columns", func(w *csv.Writer) { writeStandardCSV(w, flights, exportPrefs{}) },
			"TimeOff", "TimeOn", [][2]string{{"10:05:00", "10:47:00"}, {"08:10:00", "09:20:00"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			records := renderCSV(t, tt.write)
			for i, want := range tt.want {
				row := records[i+1]
				if got := column(t, records, row, tt.depCol); got != want[0] {
					t.Errorf("row %d %s = %q, want %q", i, tt.depCol, got, want[0])
				}
				if got := column(t, records, row, tt.arrCol); got != want[1] {
					t.Errorf("row %d %s = %q, want %q", i, tt.arrCol, got, want[1])
				}
			}
		})
	}
}

func TestEASAPDFRows_TimeModelClocks(t *testing.T) {
	s := func(v string) *string { return &v }
	flights := []*models.Flight{
		{AircraftReg: "D-1234", DepartureTime: s("10:05:00"), ArrivalTime: s("10:47:00")},
		{AircraftReg: "D-EFGH", OffBlockTime: s("08:00:00"), OnBlockTime: s("09:30:00"), DepartureTime: s("08:10:00"), ArrivalTime: s("09:20:00")},
	}
	rows := buildEASARows(flights, nil, "Pilot", 40)
	want := [][2]string{{"10:05", "10:47"}, {"08:00", "09:30"}}
	for i, w := range want {
		if dep, arr := fmtTime(rows[i].depClock), fmtTime(rows[i].arrClock); dep != w[0] || arr != w[1] {
			t.Errorf("row %d = %s/%s, want %s/%s", i, dep, arr, w[0], w[1])
		}
	}
}
