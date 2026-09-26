package igc

import (
	"errors"
	"math"
	"time"
)

// Launch methods Analyze reports.
const (
	LaunchWinch      = "winch"
	LaunchAerotow    = "aerotow"
	LaunchSelfLaunch = "self-launch"
	LaunchUnknown    = "unknown"
)

// Analysis thresholds. docs/SAILPLANES.md "IGC import" describes them.
const (
	// SpeedWindow is the minimum span ground speed is averaged over.
	SpeedWindow = 4 * time.Second
	// TakeoffSpeedKmh must be exceeded for TakeoffSustain.
	TakeoffSpeedKmh = 25.0
	TakeoffSustain  = 20 * time.Second
	// LandingSpeedKmh must be undercut for LandingSustain.
	LandingSpeedKmh = 5.0
	LandingSustain  = 60 * time.Second

	// EngineOnThreshold is the ENL/MOP value (0-999) read as engine running.
	EngineOnThreshold = 500
	// EngineRunMin is the shortest engine run within EngineWindow of take-off
	// that marks a self-launch; EngineOffSustain ends it.
	EngineRunMin     = 30 * time.Second
	EngineWindow     = 2 * time.Minute
	EngineOffSustain = 30 * time.Second

	// WinchWindow bounds the time from take-off to the top of a winch launch.
	WinchWindow = 90 * time.Second
	// WinchMinGainM..WinchMaxGainM is the height gain of a winch launch;
	// WinchSoftMinGainM..WinchSoftMaxGainM is accepted at lower confidence.
	WinchMinGainM     = 250
	WinchMaxGainM     = 700
	WinchSoftMinGainM = 150
	WinchSoftMaxGainM = 900
	// WinchMinClimbMs is the minimum mean climb rate to the top of the launch.
	WinchMinClimbMs = 4.0
	// WinchLevelOff is the span after the top whose climb rate must be under
	// half the launch climb rate.
	WinchLevelOff = 30 * time.Second

	// AerotowMinDuration is the shortest tow; AerotowMinGainM its minimum
	// height gain; AerotowMinClimbMs..AerotowMaxClimbMs its mean climb rate.
	AerotowMinDuration = 2 * time.Minute
	AerotowMinGainM    = 200
	AerotowMinClimbMs  = 1.0
	AerotowMaxClimbMs  = 6.0
	// ReleaseTurnDeg within ReleaseTurnWindow, or a mean vario below
	// ReleaseSinkMs over ReleaseSinkWindow, marks the end of a tow.
	ReleaseTurnDeg    = 200.0
	ReleaseTurnWindow = 30 * time.Second
	ReleaseSinkMs     = -0.3
	ReleaseSinkWindow = 20 * time.Second
	// TowSettle is skipped after take-off before looking for the release.
	TowSettle = time.Minute
)

// ErrNoFlight is returned when no take-off is found.
var ErrNoFlight = errors.New("no take-off found in the file")

// Point is a position at a time.
type Point struct {
	Time time.Time
	Lat  float64
	Lon  float64
	// AltM is the altitude in metres from the source Analysis.AltitudeSource
	// names.
	AltM int
}

// Analysis is what Analyze derives from a file.
type Analysis struct {
	Takeoff Point
	Landing Point
	// LandingDetected is false when the file ends before a landing is seen;
	// Landing is then the last fix.
	LandingDetected bool
	// Duration is take-off to landing.
	Duration time.Duration

	LaunchMethod string
	// LaunchConfidence is 0..1; 0 for LaunchUnknown.
	LaunchConfidence float64
	// ReleaseHeightM is the height gained from take-off to release or engine
	// stop, nil when unknown.
	ReleaseHeightM *int
	// Release is the release or engine-stop point, nil when unknown.
	Release *Point

	// MaxAltitudeM is the highest altitude between take-off and landing.
	MaxAltitudeM int
	// FreeDistanceKm is the largest straight-line distance from the take-off
	// point to any fix in flight.
	FreeDistanceKm float64
	// OutAndReturnKm is 2 x FreeDistanceKm.
	OutAndReturnKm float64
	// AltitudeSource is "gnss" or "pressure".
	AltitudeSource string
}

type sample struct {
	t     time.Time
	lat   float64
	lon   float64
	alt   int
	eng   int
	speed float64 // km/h
}

// Analyze derives take-off, landing, launch method, release height, maximum
// altitude and distances from the valid fixes of f. The first take-off in the
// file and the first landing after it bound the flight.
func Analyze(f *File) (*Analysis, error) {
	useGNSS := preferGNSS(f.Fixes)
	s := make([]sample, 0, len(f.Fixes))
	for _, fx := range f.Fixes {
		if !fx.Valid || (fx.Lat == 0 && fx.Lon == 0) {
			continue
		}
		alt := fx.PressureAlt
		if useGNSS {
			alt = fx.GNSSAlt
		}
		eng := -1
		if fx.ENL > eng {
			eng = fx.ENL
		}
		if fx.MOP > eng {
			eng = fx.MOP
		}
		if n := len(s); n > 0 && !fx.Time.After(s[n-1].t) {
			continue
		}
		s = append(s, sample{t: fx.Time, lat: fx.Lat, lon: fx.Lon, alt: alt, eng: eng})
	}
	if len(s) < 2 {
		return nil, ErrNoFlight
	}
	computeSpeeds(s)

	to := findTakeoff(s)
	if to < 0 {
		return nil, ErrNoFlight
	}
	ldg, detected := findLanding(s, to)

	a := &Analysis{
		Takeoff:         point(s[to]),
		Landing:         point(s[ldg]),
		LandingDetected: detected,
		Duration:        s[ldg].t.Sub(s[to].t),
		LaunchMethod:    LaunchUnknown,
		AltitudeSource:  "pressure",
	}
	if useGNSS {
		a.AltitudeSource = "gnss"
	}

	maxAlt := s[to].alt
	maxDist := 0.0
	for i := to; i <= ldg; i++ {
		if s[i].alt > maxAlt {
			maxAlt = s[i].alt
		}
		if d := haversineKm(s[to].lat, s[to].lon, s[i].lat, s[i].lon); d > maxDist {
			maxDist = d
		}
	}
	a.MaxAltitudeM = maxAlt
	a.FreeDistanceKm = round1(maxDist)
	a.OutAndReturnKm = round1(2 * maxDist)

	flight := s[to : ldg+1]
	if f.HasENL || f.HasMOP {
		if rel, conf, ok := detectSelfLaunch(flight); ok {
			a.setLaunch(LaunchSelfLaunch, conf, flight, rel)
			return a, nil
		}
	}
	if rel, conf, ok := detectWinch(flight); ok {
		a.setLaunch(LaunchWinch, conf, flight, rel)
		return a, nil
	}
	if rel, conf, ok := detectAerotow(flight); ok {
		a.setLaunch(LaunchAerotow, conf, flight, rel)
		return a, nil
	}
	return a, nil
}

func (a *Analysis) setLaunch(method string, conf float64, flight []sample, rel int) {
	a.LaunchMethod = method
	a.LaunchConfidence = conf
	if rel <= 0 || rel >= len(flight) {
		return
	}
	h := flight[rel].alt - flight[0].alt
	if h < 0 {
		h = 0
	}
	a.ReleaseHeightM = &h
	p := point(flight[rel])
	a.Release = &p
}

func point(s sample) Point {
	return Point{Time: s.t, Lat: s.lat, Lon: s.lon, AltM: s.alt}
}

// preferGNSS reports whether more than half of the valid fixes carry a
// non-zero GNSS altitude.
func preferGNSS(fixes []Fix) bool {
	valid, withGNSS := 0, 0
	for _, fx := range fixes {
		if !fx.Valid {
			continue
		}
		valid++
		if fx.GNSSAlt != 0 {
			withGNSS++
		}
	}
	return valid > 0 && withGNSS*2 > valid
}

// computeSpeeds sets each sample's ground speed from the latest earlier
// sample at least SpeedWindow before it (or the first sample).
func computeSpeeds(s []sample) {
	j := 0
	for i := 1; i < len(s); i++ {
		for j+1 < i && s[i].t.Sub(s[j+1].t) >= SpeedWindow {
			j++
		}
		dt := s[i].t.Sub(s[j].t).Hours()
		if dt <= 0 {
			s[i].speed = s[i-1].speed
			continue
		}
		s[i].speed = haversineKm(s[j].lat, s[j].lon, s[i].lat, s[i].lon) / dt
	}
}

// findTakeoff returns the first sample starting a run above TakeoffSpeedKmh
// that lasts TakeoffSustain, or -1.
func findTakeoff(s []sample) int {
	run := -1
	for i := range s {
		if s[i].speed > TakeoffSpeedKmh {
			if run < 0 {
				run = i
			}
			if s[i].t.Sub(s[run].t) >= TakeoffSustain {
				return run
			}
		} else {
			run = -1
		}
	}
	return -1
}

// findLanding returns the first sample after take-off starting a run below
// LandingSpeedKmh that lasts LandingSustain or reaches the end of the file.
// Without one it returns the last sample and false.
func findLanding(s []sample, to int) (int, bool) {
	run := -1
	for i := to + 1; i < len(s); i++ {
		if s[i].t.Sub(s[to].t) < TakeoffSustain {
			continue
		}
		if s[i].speed < LandingSpeedKmh {
			if run < 0 {
				run = i
			}
			if s[i].t.Sub(s[run].t) >= LandingSustain {
				return run, true
			}
		} else {
			run = -1
		}
	}
	if run >= 0 && len(s)-run >= 2 {
		return run, true
	}
	return len(s) - 1, false
}

// detectSelfLaunch finds an engine run of at least EngineRunMin starting
// within EngineWindow of take-off and returns the engine-stop sample.
func detectSelfLaunch(f []sample) (int, float64, bool) {
	t0 := f[0].t
	run := -1
	found := -1
	for i := range f {
		if f[i].t.Sub(t0) > EngineWindow && run < 0 {
			break
		}
		if f[i].eng >= EngineOnThreshold {
			if run < 0 {
				run = i
			}
			if f[i].t.Sub(f[run].t) >= EngineRunMin {
				found = run
				break
			}
		} else {
			run = -1
		}
	}
	if found < 0 {
		return 0, 0, false
	}
	off := -1
	for i := found; i < len(f); i++ {
		if f[i].eng >= EngineOnThreshold {
			off = -1
			continue
		}
		if off < 0 {
			off = i
		}
		if f[i].t.Sub(f[off].t) >= EngineOffSustain {
			return off, 0.9, true
		}
	}
	return -1, 0.75, true
}

// detectWinch finds the top of a steep climb within WinchWindow of take-off
// followed by a level-off, and returns the top sample.
func detectWinch(f []sample) (int, float64, bool) {
	t0, alt0 := f[0].t, f[0].alt
	top := 0
	for i := range f {
		if f[i].t.Sub(t0) > WinchWindow {
			break
		}
		if f[i].alt > f[top].alt {
			top = i
		}
	}
	gain := f[top].alt - alt0
	dt := f[top].t.Sub(t0).Seconds()
	if top == 0 || dt <= 0 || gain < WinchSoftMinGainM || gain > WinchSoftMaxGainM {
		return 0, 0, false
	}
	rate := float64(gain) / dt
	if rate < WinchMinClimbMs {
		return 0, 0, false
	}
	after := f[top].alt
	end := top
	for i := top + 1; i < len(f); i++ {
		if f[i].t.Sub(f[top].t) > WinchLevelOff {
			break
		}
		end = i
		if f[i].alt > after {
			after = f[i].alt
		}
	}
	span := f[end].t.Sub(f[top].t).Seconds()
	if span < WinchLevelOff.Seconds()/2 {
		return 0, 0, false
	}
	if float64(after-f[top].alt)/span >= rate/2 {
		return 0, 0, false
	}
	conf := 0.85
	if gain < WinchMinGainM || gain > WinchMaxGainM {
		conf = 0.6
	}
	return top, conf, true
}

// detectAerotow finds the release of a sustained moderate climb: the first
// sample, TowSettle after take-off, from which the glider turns
// ReleaseTurnDeg within ReleaseTurnWindow or sinks over ReleaseSinkWindow.
func detectAerotow(f []sample) (int, float64, bool) {
	t0, alt0 := f[0].t, f[0].alt
	rel := -1
	for i := range f {
		if f[i].t.Sub(t0) < TowSettle {
			continue
		}
		if turnWithin(f, i, ReleaseTurnWindow) >= ReleaseTurnDeg || meanVario(f, i, ReleaseSinkWindow) < ReleaseSinkMs {
			rel = i
			break
		}
	}
	if rel < 0 {
		return 0, 0, false
	}
	top := 0
	for i := 0; i <= rel; i++ {
		if f[i].alt > f[top].alt {
			top = i
		}
	}
	rel = top
	dur := f[rel].t.Sub(t0)
	gain := f[rel].alt - alt0
	if dur < AerotowMinDuration || gain < AerotowMinGainM {
		return 0, 0, false
	}
	rate := float64(gain) / dur.Seconds()
	if rate < AerotowMinClimbMs || rate > AerotowMaxClimbMs {
		return 0, 0, false
	}
	conf := 0.7
	if dur >= 3*time.Minute && gain >= 300 {
		conf = 0.75
	}
	return rel, conf, true
}

// turnWithin returns the absolute net heading change over the samples from
// i spanning at most w.
func turnWithin(f []sample, i int, w time.Duration) float64 {
	total := 0.0
	prev := math.NaN()
	for k := i + 1; k < len(f) && f[k].t.Sub(f[i].t) <= w; k++ {
		if haversineKm(f[k-1].lat, f[k-1].lon, f[k].lat, f[k].lon) < 0.005 {
			continue
		}
		b := bearing(f[k-1].lat, f[k-1].lon, f[k].lat, f[k].lon)
		if !math.IsNaN(prev) {
			d := b - prev
			for d > 180 {
				d -= 360
			}
			for d < -180 {
				d += 360
			}
			total += d
		}
		prev = b
	}
	return math.Abs(total)
}

// meanVario returns the mean climb rate in m/s over the samples from i
// spanning w, or 0 when the file ends first.
func meanVario(f []sample, i int, w time.Duration) float64 {
	k := i
	for k+1 < len(f) && f[k+1].t.Sub(f[i].t) <= w {
		k++
	}
	dt := f[k].t.Sub(f[i].t).Seconds()
	if dt < w.Seconds()/2 {
		return 0
	}
	return float64(f[k].alt-f[i].alt) / dt
}

const earthRadiusKm = 6371.0

func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	dLat := rad(lat2 - lat1)
	dLon := rad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return earthRadiusKm * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// DistanceKm returns the great-circle distance between two points.
func DistanceKm(lat1, lon1, lat2, lon2 float64) float64 {
	return haversineKm(lat1, lon1, lat2, lon2)
}

func bearing(lat1, lon1, lat2, lon2 float64) float64 {
	y := math.Sin(rad(lon2-lon1)) * math.Cos(rad(lat2))
	x := math.Cos(rad(lat1))*math.Sin(rad(lat2)) - math.Sin(rad(lat1))*math.Cos(rad(lat2))*math.Cos(rad(lon2-lon1))
	return math.Mod(math.Atan2(y, x)*180/math.Pi+360, 360)
}

func rad(d float64) float64 { return d * math.Pi / 180 }

func round1(v float64) float64 { return math.Round(v*10) / 10 }
