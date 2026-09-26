package igc

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

const (
	testHomeLat = 50.49889
	testHomeLon = 9.95389
)

func TestAnalyze_Fixtures(t *testing.T) {
	tests := []struct {
		name            string
		file            string
		launch          string
		minConfidence   float64
		takeoff         string
		landing         string
		minutes         int
		releaseMin      int
		releaseMax      int
		maxAltMin       int
		maxAltMax       int
		freeKmMin       float64
		freeKmMax       float64
		landedAwayKmMin float64
		landedAwayKmMax float64
	}{
		{name: "L winch launch to about 420 m", file: "winch.igc", launch: LaunchWinch, minConfidence: 0.8,
			takeoff: "2026-06-15 10:01:33", landing: "2026-06-15 10:08:09", minutes: 7,
			releaseMin: 380, releaseMax: 460, maxAltMin: 1280, maxAltMax: 1360, freeKmMin: 1.5, freeKmMax: 3,
			landedAwayKmMin: 0, landedAwayKmMax: 1.5},
		{name: "aerotow behind a noisy tug is not a self-launch", file: "aerotow.igc", launch: LaunchAerotow, minConfidence: 0.7,
			takeoff: "2026-07-02 12:01:06", landing: "2026-07-02 12:22:38", minutes: 22,
			releaseMin: 500, releaseMax: 650, maxAltMin: 1900, maxAltMax: 2000, freeKmMin: 5, freeKmMax: 8,
			landedAwayKmMin: 0, landedAwayKmMax: 1.5},
		{name: "out-and-return lands back home", file: "out_and_return.igc", launch: LaunchAerotow, minConfidence: 0.7,
			takeoff: "2026-07-02 12:01:06", landing: "2026-07-02 12:50:15", minutes: 49,
			releaseMin: 500, releaseMax: 650, maxAltMin: 2100, maxAltMax: 2200, freeKmMin: 33, freeKmMax: 35,
			landedAwayKmMin: 0, landedAwayKmMax: 1.5},
		{name: "P2 self-launch with ENL and MOP, out-and-return, outlanding", file: "selflaunch.igc", launch: LaunchSelfLaunch, minConfidence: 0.85,
			takeoff: "2026-08-10 09:31:46", landing: "2026-08-10 10:22:46", minutes: 51,
			releaseMin: 700, releaseMax: 800, maxAltMin: 2300, maxAltMax: 2400, freeKmMin: 39, freeKmMax: 41,
			landedAwayKmMin: 19, landedAwayKmMax: 21},
		{name: "winch launch then outlanding", file: "outlanding.igc", launch: LaunchWinch, minConfidence: 0.8,
			takeoff: "2026-06-20 13:01:03", landing: "2026-06-20 13:22:51", minutes: 22,
			releaseMin: 380, releaseMax: 460, maxAltMin: 1800, maxAltMax: 1950, freeKmMin: 14, freeKmMax: 17,
			landedAwayKmMin: 14, landedAwayKmMax: 17},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Parse(readFixture(t, tt.file))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			a, err := Analyze(f)
			if err != nil {
				t.Fatalf("Analyze: %v", err)
			}
			if a.LaunchMethod != tt.launch || a.LaunchConfidence < tt.minConfidence {
				t.Errorf("launch = %s (%.2f), want %s (>= %.2f)", a.LaunchMethod, a.LaunchConfidence, tt.launch, tt.minConfidence)
			}
			if got := a.Takeoff.Time.Format("2006-01-02 15:04:05"); got != tt.takeoff {
				t.Errorf("takeoff = %s, want %s", got, tt.takeoff)
			}
			if got := a.Landing.Time.Format("2006-01-02 15:04:05"); got != tt.landing {
				t.Errorf("landing = %s, want %s", got, tt.landing)
			}
			if !a.LandingDetected {
				t.Error("landing not detected")
			}
			if got := int(a.Duration.Round(time.Minute).Minutes()); got != tt.minutes {
				t.Errorf("duration = %s", a.Duration)
			}
			if a.ReleaseHeightM == nil || *a.ReleaseHeightM < tt.releaseMin || *a.ReleaseHeightM > tt.releaseMax {
				t.Errorf("release height = %v, want %d..%d", deref(a.ReleaseHeightM), tt.releaseMin, tt.releaseMax)
			}
			if a.Release == nil {
				t.Error("release point missing")
			}
			if a.MaxAltitudeM < tt.maxAltMin || a.MaxAltitudeM > tt.maxAltMax {
				t.Errorf("max altitude = %d", a.MaxAltitudeM)
			}
			if a.FreeDistanceKm < tt.freeKmMin || a.FreeDistanceKm > tt.freeKmMax {
				t.Errorf("free distance = %.1f", a.FreeDistanceKm)
			}
			if a.OutAndReturnKm != round1(2*a.FreeDistanceKm) && a.OutAndReturnKm-2*a.FreeDistanceKm > 0.11 {
				t.Errorf("out-and-return = %.1f for free %.1f", a.OutAndReturnKm, a.FreeDistanceKm)
			}
			away := DistanceKm(testHomeLat, testHomeLon, a.Landing.Lat, a.Landing.Lon)
			if away < tt.landedAwayKmMin || away > tt.landedAwayKmMax {
				t.Errorf("landed %.1f km from home", away)
			}
			if a.AltitudeSource != "gnss" {
				t.Errorf("altitude source = %s", a.AltitudeSource)
			}
		})
	}
}

func TestAnalyze_MidnightUTC(t *testing.T) {
	f, err := Parse(readFixture(t, "midnight.igc"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	a, err := Analyze(f)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if got := a.Takeoff.Time.Format("2006-01-02 15:04"); got != "2025-01-15 23:25" {
		t.Errorf("takeoff = %s", got)
	}
	if got := a.Landing.Time.Format("2006-01-02 15:04"); got != "2025-01-16 00:36" {
		t.Errorf("landing = %s", got)
	}
	if a.Duration < 70*time.Minute || a.Duration > 73*time.Minute {
		t.Errorf("duration = %s", a.Duration)
	}
	if a.LaunchMethod != LaunchAerotow {
		t.Errorf("launch = %s", a.LaunchMethod)
	}
}

// synth builds an IGC file from per-second samples of ground speed (km/h),
// vertical speed (m/s) and ENL, flying due east from 50N 10E at 500 m.
func synth(t *testing.T, header string, segs ...seg) *File {
	t.Helper()
	var b strings.Builder
	b.WriteString("AXXX\nHFDTE010726\n" + header)
	lat, lon, alt := 50.0, 10.0, 500.0
	sec := 10 * 3600
	for _, s := range segs {
		for i := 0; i < s.n; i++ {
			lon += s.kmh / 3.6 / (111320 * 0.6428)
			alt += s.vs
			sec++
			d := int(lat)
			m := int((lat - float64(d)) * 60000)
			ld := int(lon)
			lm := int((lon - float64(ld)) * 60000)
			fmt.Fprintf(&b, "B%02d%02d%02d%02d%05dN%03d%05dEA%05d%05d", sec/3600, sec/60%60, sec%60, d, m, ld, lm, int(alt), int(alt))
			if strings.Contains(header, "ENL") {
				fmt.Fprintf(&b, "%03d", s.enl)
			}
			b.WriteString("\n")
		}
	}
	f, err := Parse([]byte(b.String()))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return f
}

type seg struct {
	n   int
	kmh float64
	vs  float64
	enl int
}

func TestAnalyze_Synthetic(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		segs     []seg
		err      error
		launch   string
		detected bool
		release  *int
	}{
		{name: "never moves", segs: []seg{{n: 600}}, err: ErrNoFlight},
		{name: "taxi below take-off speed", segs: []seg{{n: 60}, {n: 300, kmh: 20}, {n: 60}}, err: ErrNoFlight},
		{name: "short burst is not a take-off", segs: []seg{{n: 60}, {n: 10, kmh: 60}, {n: 300}}, err: ErrNoFlight},
		{name: "file ends in flight", segs: []seg{{n: 60}, {n: 600, kmh: 90, vs: 0.5}}, launch: LaunchUnknown, detected: false},
		{name: "logger stops right after landing", segs: []seg{{n: 60}, {n: 600, kmh: 90}, {n: 5}}, launch: LaunchUnknown, detected: true},
		{
			name: "self-launch engine never stops (no release)", header: "I013638ENL\n",
			segs:   []seg{{n: 30}, {n: 900, kmh: 100, vs: 1, enl: 800}, {n: 90}},
			launch: LaunchSelfLaunch, detected: true,
		},
		{
			name: "engine noise without an ENL record is ignored",
			segs: []seg{{n: 30}, {n: 900, kmh: 100, vs: 0, enl: 800}, {n: 90}}, launch: LaunchUnknown, detected: true,
		},
		{
			name: "ENL declared but engine never run", header: "I013638ENL\n",
			segs:   []seg{{n: 30}, {n: 600, kmh: 100, enl: 100}, {n: 90}},
			launch: LaunchUnknown, detected: true,
		},
		{
			name: "climb too slow for a winch and too short for a tow",
			segs: []seg{{n: 30}, {n: 60, kmh: 100, vs: 2}, {n: 300, kmh: 100, vs: -1}, {n: 90}}, launch: LaunchUnknown, detected: true,
		},
		{
			name: "steep climb without level-off is not a winch",
			segs: []seg{{n: 30}, {n: 200, kmh: 100, vs: 8}, {n: 300, kmh: 100, vs: -1}, {n: 90}}, launch: LaunchUnknown, detected: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := Analyze(synth(t, tt.header, tt.segs...))
			if tt.err != nil {
				if !errors.Is(err, tt.err) {
					t.Fatalf("err = %v, want %v", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Analyze: %v", err)
			}
			if a.LaunchMethod != tt.launch {
				t.Errorf("launch = %s, want %s", a.LaunchMethod, tt.launch)
			}
			if a.LaunchMethod == LaunchUnknown && (a.LaunchConfidence != 0 || a.ReleaseHeightM != nil) {
				t.Errorf("unknown launch carries confidence %.2f, release %v", a.LaunchConfidence, a.ReleaseHeightM)
			}
			if a.LandingDetected != tt.detected {
				t.Errorf("landing detected = %v", a.LandingDetected)
			}
			if tt.launch == LaunchSelfLaunch && a.ReleaseHeightM != nil {
				t.Errorf("release height = %d for an engine that never stopped", *a.ReleaseHeightM)
			}
		})
	}
}

func TestAnalyze_IgnoresInvalidFixes(t *testing.T) {
	f := synth(t, "", seg{n: 30}, seg{n: 300, kmh: 90}, seg{n: 90})
	for i := range f.Fixes {
		if i%3 == 0 {
			f.Fixes[i].Valid = false
			f.Fixes[i].Lat, f.Fixes[i].Lon = 0, 0
		}
	}
	a, err := Analyze(f)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if a.FreeDistanceKm < 6 || a.FreeDistanceKm > 9 {
		t.Errorf("free distance = %.1f", a.FreeDistanceKm)
	}
}

func deref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
