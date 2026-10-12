package postgres

import "testing"

func TestLaunchCountExpr(t *testing.T) {
	tests := []struct {
		name, got, want string
	}{
		{"flight row", flightLaunchesSQL,
			`COALESCE(launches, CASE WHEN is_simulator OR is_passenger THEN 0 ELSE GREATEST(takeoffs_day + takeoffs_night, 1) END)`},
		{"aggregate over f", launchCountSQL,
			`COALESCE(f.launches, CASE WHEN f.is_simulator OR f.is_passenger THEN 0 ELSE GREATEST(f.takeoffs_day + f.takeoffs_night, 1) END)`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got  %s\nwant %s", tt.got, tt.want)
			}
		})
	}
}
