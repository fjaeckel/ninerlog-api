package importtemplate

import (
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

func TestParseLaunchMethod(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"W", "winch"},
		{"F", "aerotow"},
		{"E", "self-launch"},
		{"A", "car"},
		{"G", "bungee"},
		{" w ", "winch"},
		{"f", "aerotow"},
		{"Winde", "winch"},
		{"Windenstart", "winch"},
		{"F-Schlepp", "aerotow"},
		{"Flugzeugschlepp", "aerotow"},
		{"EIGENSTART", "self-launch"},
		{"Autoschlepp", "car"},
		{"Gummiseil", "bungee"},
		{"winch", "winch"},
		{"Aerotow", "aerotow"},
		{"self-launch", "self-launch"},
		{"car", "car"},
		{"bungee", "bungee"},
		{"", ""},
		{"X", ""},
		{"catapult", ""},
		{"WF", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := ParseLaunchMethod(tt.in); got != tt.want {
				t.Errorf("ParseLaunchMethod(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestLaunchMethodAliasesAreValidMethods(t *testing.T) {
	for alias, m := range launchMethodAliases {
		if !models.IsValidLaunchMethod(m) {
			t.Errorf("alias %q maps to %q, not a valid launch method", alias, m)
		}
	}
}

func TestForeFlightAircraftClass(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"airplane_single_engine_land", "SEP_LAND"},
		{"Airplane_Multi_Engine_Land", "MEP_LAND"},
		{"airplane_single_engine_sea", "SEP_SEA"},
		{"airplane_multi_engine_sea", "MEP_SEA"},
		{" glider ", "GLIDER"},
		{"rotorcraft_gyroplane", "GYROPLANE"},
		{"rotorcraft_helicopter", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := ForeFlightAircraftClass(tt.in); got != tt.want {
				t.Errorf("ForeFlightAircraftClass(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSuggestMapsLaunchMethod(t *testing.T) {
	tests := []struct {
		name     string
		template string
		headers  []string
		column   string
	}{
		{"L2 Vereinsflieger S.-Art", "VEREINSFLIEGER_CSV", vereinsfliegerHeaders, "S.-Art"},
		{"generic German Startart", FormatGenericCSV, []string{"Datum", "Kennzeichen", "Startart", "Flugzeit"}, "Startart"},
		{"NinerLog standard LaunchMethod", "NINERLOG_CSV", []string{"Date", "AircraftID", "TotalTime", "LaunchMethod"}, "LaunchMethod"},
		{"generic Launch Method", FormatGenericCSV, []string{"Date", "Registration", "Total Time", "Launch Method"}, "Launch Method"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ""
			for _, m := range ByID(tt.template).Suggest(tt.headers) {
				if m.TargetField == FieldLaunchMethod {
					got = m.SourceColumn
				}
			}
			if got != tt.column {
				t.Errorf("launchMethod mapped from %q, want %q", got, tt.column)
			}
		})
	}
}

func TestSuggestMapsGliderFacts(t *testing.T) {
	tests := []struct {
		name     string
		template string
		headers  []string
		want     map[string]Field
	}{
		{
			name:     "NinerLog standard layout",
			template: "NINERLOG_CSV",
			headers:  []string{"Date", "AircraftID", "TotalTime", "LaunchMethod", "Launches", "Outlanding", "TowFlight", "ReleaseHeightM"},
			want: map[string]Field{
				"Launches": FieldLaunches, "Outlanding": FieldIsOutlanding,
				"TowFlight": FieldIsTowFlight, "ReleaseHeightM": FieldReleaseHeightM,
			},
		},
		{
			name:     "German Starts and Außenlandung",
			template: FormatGenericCSV,
			headers:  []string{"Datum", "Kennzeichen", "Startart", "Starts", "Außenlandung", "Flugzeit"},
			want:     map[string]Field{"Starts": FieldLaunches, "Außenlandung": FieldIsOutlanding},
		},
		{
			name:     "L2 Vereinsflieger Start stays the take-off time",
			template: "VEREINSFLIEGER_CSV",
			headers:  vereinsfliegerHeaders,
			want:     map[string]Field{"Start": FieldDepartureTime, "Landungen": FieldLandingsTotal},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := map[string]Field{}
			for _, m := range ByID(tt.template).Suggest(tt.headers) {
				got[m.SourceColumn] = m.TargetField
			}
			for col, field := range tt.want {
				if got[col] != field {
					t.Errorf("%q mapped to %q, want %q", col, got[col], field)
				}
			}
		})
	}
}
