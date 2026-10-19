package solar

import (
	"math"
	"testing"
	"time"

	gosolar "github.com/mstephenholl/go-solar"
)

type site struct {
	icao     string
	lat, lon float64
}

var (
	kfdk = site{"KFDK", 39.4176, -77.3743}
	ksfo = site{"KSFO", 37.6188, -122.3750}
	yssy = site{"YSSY", -33.9461, 151.1772}
	eddf = site{"EDDF", 50.0333, 8.5706}
	ensb = site{"ENSB", 78.2461, 15.4656}
	nzaa = site{"NZAA", -37.0081, 174.7917}
	phnl = site{"PHNL", 21.3187, -157.9225}
	wsss = site{"WSSS", 1.3502, 103.9940}
	seqm = site{"SEQM", -0.1292, -78.3575}
)

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

func TestIsNight_Airports(t *testing.T) {
	tests := []struct {
		name  string
		site  site
		at    string
		night bool
	}{
		{"KFDK June 20:30 EDT before civil dusk", kfdk, "2026-06-20T00:30:00Z", false},
		{"KFDK June 21:11 EDT before civil dusk 01:11:48Z", kfdk, "2026-06-20T01:11:00Z", false},
		{"KFDK June 21:12 EDT after civil dusk", kfdk, "2026-06-20T01:12:00Z", true},
		{"KFDK June 05:00 EDT before civil dawn", kfdk, "2026-06-20T09:00:00Z", true},
		{"KFDK June noon EDT", kfdk, "2026-06-20T16:00:00Z", false},
		{"KSFO October 18:00 PDT", ksfo, "2026-10-20T01:00:00Z", false},
		{"KSFO July 20:00 PDT", ksfo, "2026-07-15T03:00:00Z", false},
		{"KSFO July 22:00 PDT", ksfo, "2026-07-15T05:00:00Z", true},
		{"YSSY October 07:00 AEDT", yssy, "2026-10-20T20:00:00Z", false},
		{"YSSY October 05:30 AEDT before civil dawn", yssy, "2026-10-20T18:30:00Z", true},
		{"YSSY October 20:00 AEDT after civil dusk", yssy, "2026-10-20T09:00:00Z", true},
		{"EDDF January 14:00Z", eddf, "2026-01-15T14:00:00Z", false},
		{"EDDF January 20:00Z", eddf, "2026-01-15T20:00:00Z", true},
		{"EDDF January 05:00Z", eddf, "2026-01-15T05:00:00Z", true},
		{"EDDF January 16:28Z before civil dusk 16:28:09Z", eddf, "2026-01-15T16:28:00Z", false},
		{"EDDF January 16:29Z after civil dusk", eddf, "2026-01-15T16:29:00Z", true},
		{"ENSB December polar night noon", ensb, "2026-12-15T11:00:00Z", true},
		{"ENSB December polar night midnight", ensb, "2026-12-15T00:00:00Z", true},
		{"ENSB June midnight sun 23:30Z", ensb, "2026-06-15T23:30:00Z", false},
		{"ENSB June midnight sun 00:00Z", ensb, "2026-06-16T00:00:00Z", false},
		{"NZAA October 07:30 NZDT", nzaa, "2026-10-19T18:30:00Z", false},
		{"NZAA October 06:00 NZDT before civil dawn", nzaa, "2026-10-19T17:00:00Z", true},
		{"PHNL October 18:00 HST", phnl, "2026-10-20T04:00:00Z", false},
		{"PHNL October 19:00 HST after civil dusk", phnl, "2026-10-20T05:00:00Z", true},
		{"PHNL October 06:00 HST before civil dawn", phnl, "2026-10-20T16:00:00Z", true},
		{"WSSS equinox 07:30 SGT", wsss, "2026-03-21T23:30:00Z", false},
		{"WSSS equinox 06:30 SGT", wsss, "2026-03-21T22:30:00Z", true},
		{"SEQM equinox 18:30 ECT", seqm, "2026-03-21T23:30:00Z", false}, //nolint:misspell // ECT is Ecuador Time
		{"SEQM equinox 19:00 ECT", seqm, "2026-03-22T00:00:00Z", true},  //nolint:misspell // ECT is Ecuador Time
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at := mustTime(t, tt.at)
			if got := IsNight(at, tt.site.lat, tt.site.lon); got != tt.night {
				t.Errorf("IsNight(%s at %s) = %v, want %v", tt.site.icao, tt.at, got, tt.night)
			}
		})
	}
}

func TestIsNight_MatchesSolarElevation(t *testing.T) {
	sites := []site{kfdk, ksfo, yssy, eddf, ensb, nzaa, phnl, wsss, seqm,
		{"PABR", 71.2854, -156.7660}, {"NZSP", -89.9999, 139.2728}}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(1, 0, 0)
	const toleranceDeg = 0.5
	for _, s := range sites {
		loc := gosolar.NewLocation(s.lat, s.lon)
		mismatches := 0
		for at := start; at.Before(end); at = at.Add(37 * time.Minute) {
			elev := gosolar.Elevation(loc, at)
			if math.Abs(elev+6) <= toleranceDeg {
				continue
			}
			if got := IsNight(at, s.lat, s.lon); got != (elev < -6) {
				mismatches++
				if mismatches <= 3 {
					t.Errorf("%s %s: IsNight=%v, elevation=%.2f°", s.icao, at.Format(time.RFC3339), got, elev)
				}
			}
		}
		if mismatches > 0 {
			t.Errorf("%s: %d instants disagree with solar elevation away from a twilight boundary", s.icao, mismatches)
		}
	}
}

func TestDaylightBetween_NoSeamGaps(t *testing.T) {
	tests := []struct {
		name  string
		site  site
		from  string
		night bool
	}{
		{"ENSB midnight sun is day for every minute", ensb, "2026-06-10T00:00:00Z", false},
		{"ENSB polar night is night for every minute", ensb, "2026-12-10T00:00:00Z", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			from := mustTime(t, tt.from)
			to := from.Add(72 * time.Hour)
			d := DaylightBetween(from, to, tt.site.lat, tt.site.lon)
			for at := from; at.Before(to); at = at.Add(time.Minute) {
				if d.IsNight(at) != tt.night {
					t.Fatalf("%s at %s: IsNight = %v, want %v", tt.site.icao, at.Format(time.RFC3339), !tt.night, tt.night)
				}
			}
		})
	}
}

func TestDaylightBetween_CoversNeighbouringDays(t *testing.T) {
	from := mustTime(t, "2026-06-20T00:00:00Z")
	to := mustTime(t, "2026-06-21T12:00:00Z")
	d := DaylightBetween(from, to, kfdk.lat, kfdk.lon)
	if len(d) != 4 {
		t.Fatalf("len(DaylightBetween) = %d, want 4 (19th through 22nd)", len(d))
	}
	if !d[0].Contains(from) {
		t.Errorf("first interval %v does not contain %s", d[0], from)
	}
}
