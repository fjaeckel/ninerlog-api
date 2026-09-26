// Command gen writes the synthetic IGC fixtures in pkg/igc/testdata.
//
//	go run ./pkg/igc/testdata/gen
package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
)

// Wasserkuppe (EDER), elevation 901 m.
const (
	homeLat = 50.49889
	homeLon = 9.95389
	homeElv = 901.0
)

type sim struct {
	sec      int
	lat, lon float64
	alt      float64
	hdg      float64
	spd      float64
	vs       float64
	enl      int
	mop      int
	withENL  bool
	withMOP  bool
	interval int
	rng      *rand.Rand
	lines    []string
}

func newSim(startSec int, lat, lon, alt float64, interval int, seed int64) *sim {
	return &sim{sec: startSec, lat: lat, lon: lon, alt: alt, interval: interval, rng: rand.New(rand.NewSource(seed)), enl: 5, mop: 3}
}

// run advances n seconds, calling f before each second.
func (s *sim) run(n int, f func(i int)) {
	for i := 0; i < n; i++ {
		if f != nil {
			f(i)
		}
		d := s.spd / 3.6
		s.lat += d * math.Cos(rad(s.hdg)) / 111320.0
		s.lon += d * math.Sin(rad(s.hdg)) / (111320.0 * math.Cos(rad(s.lat)))
		s.alt += s.vs
		s.sec++
		if s.sec%s.interval == 0 {
			s.emit()
		}
	}
}

func (s *sim) emit() {
	t := s.sec % 86400
	line := fmt.Sprintf("B%02d%02d%02d%s%sA%s%s",
		t/3600, (t/60)%60, t%60, fmtLat(s.lat), fmtLon(s.lon),
		fmtAlt(int(math.Round(s.alt))-28+s.rng.Intn(3)), fmtAlt(int(math.Round(s.alt))))
	if s.withENL {
		line += fmt.Sprintf("%03d", clamp(s.enl+s.rng.Intn(20)))
	}
	if s.withMOP {
		line += fmt.Sprintf("%03d", clamp(s.mop+s.rng.Intn(20)))
	}
	s.lines = append(s.lines, line)
}

func clamp(v int) int {
	if v > 999 {
		return 999
	}
	if v < 0 {
		return 0
	}
	return v
}

func (s *sim) ground(n int) {
	s.spd, s.vs = 0, 0
	s.run(n, nil)
}

// roll accelerates on the ground to v km/h over n seconds.
func (s *sim) roll(n int, v float64) {
	s.vs = 0
	start := s.spd
	s.run(n, func(i int) { s.spd = start + (v-start)*float64(i+1)/float64(n) })
}

// straight flies n seconds at speed v and vertical speed vs.
func (s *sim) straight(n int, v, vs float64) {
	s.spd, s.vs = v, vs
	s.run(n, nil)
}

// circle turns at rate deg/s for n seconds.
func (s *sim) circle(n int, v, vs, rate float64) {
	s.spd, s.vs = v, vs
	s.run(n, func(int) { s.hdg = math.Mod(s.hdg+rate+360, 360) })
}

// glideTo flies toward lat/lon at v with vertical speed vs until within 200 m.
func (s *sim) glideTo(lat, lon, v, vs float64) {
	s.spd, s.vs = v, vs
	for i := 0; i < 20000; i++ {
		if distM(s.lat, s.lon, lat, lon) < 200 {
			return
		}
		s.hdg = bearing(s.lat, s.lon, lat, lon)
		s.run(1, nil)
	}
}

// land spirals down to 60 m above groundAlt, flies a straight final and
// rolls out.
func (s *sim) land(groundAlt float64) {
	hdg := s.hdg
	s.spd = 90
	for s.alt > groundAlt+60 {
		s.vs = -3
		s.hdg = math.Mod(s.hdg+8, 360)
		s.run(1, nil)
	}
	s.hdg = hdg
	for s.alt > groundAlt+2 {
		s.vs = -2.5
		if s.alt-groundAlt < 20 {
			s.vs = -1
		}
		s.run(1, nil)
	}
	s.alt = groundAlt
	s.vs = 0
	start := s.spd
	s.run(15, func(i int) { s.spd = start * (1 - float64(i+1)/15) })
	s.spd = 0
}

func header(date string, newDate bool, pilot, gtype, gid, cid, iRecord string) []string {
	h := []string{"AXXXLXV Synthetic NinerLog fixture"}
	if newDate {
		h = append(h, "HFDTEDATE:"+date+",01")
	} else {
		h = append(h, "HFDTE"+date)
	}
	h = append(h,
		"HFFXA035",
		"HFPLTPILOTINCHARGE:"+pilot,
		"HFCM2CREW2:NIL",
		"HFGTYGLIDERTYPE:"+gtype,
		"HFGIDGLIDERID:"+gid,
		"HFDTM100GPSDATUM:WGS-1984",
		"HFRFWFIRMWAREVERSION:9.1",
		"HFFTYFRTYPE:SYNTHETIC",
		"HFCIDCOMPETITIONID:"+cid,
	)
	if iRecord != "" {
		h = append(h, iRecord)
	}
	return h
}

func write(name string, head []string, s *sim, extra []string, crlf bool) {
	all := append(append(append([]string{}, head...), s.lines...), extra...)
	all = append(all, "LXXXNinerLog synthetic fixture, not a real flight", "G0000000000000000000000000000000000000000000000000000000000")
	sep := "\n"
	if crlf {
		sep = "\r\n"
	}
	out := strings.Join(all, sep) + sep
	if err := os.WriteFile(filepath.Join("pkg", "igc", "testdata", name), []byte(out), 0o644); err != nil {
		panic(err)
	}
}

// winch: Lena, ASK 21 D-1234, winch launch to ~420 m, 7 minute circuit.
func winch() {
	s := newSim(10*3600, homeLat, homeLon, homeElv, 1, 1)
	s.hdg = 250
	s.ground(90)
	s.roll(3, 95)
	// climb: ramp up, hold, then round out at the top
	s.run(42, func(i int) {
		switch {
		case i < 5:
			s.vs = float64(i+1) * 2.5
		case i < 35:
			s.vs = 12
		default:
			s.vs = 6 - float64(i-35)
		}
		s.spd = 100
	})
	s.straight(20, 95, -1.5)
	s.circle(60, 90, -1.0, 3)
	s.straight(120, 95, -1.2)
	s.circle(60, 90, -1.0, 3)
	s.glideTo(homeLat, homeLon, 95, -1.5)
	s.hdg = 250
	s.land(homeElv)
	s.ground(120)
	lines := s.lines
	// one 2D fix and one malformed B record mid-flight
	s.lines = append([]string{}, lines[:200]...)
	s.lines = append(s.lines, strings.Replace(lines[200], "A0", "V0", 1), "B1234XX")
	s.lines = append(s.lines, lines[201:]...)
	write("winch.igc", header("150626", false, "Lena Example", "ASK 21", "D-1234", "LE", ""), s, nil, true)
}

// aerotow: Lena, LS4 D-5678, 6 minute tow at 3 m/s behind a noisy tug, release, thermal, land.
func aerotow(name string, outAndReturn bool) {
	s := newSim(12*3600, homeLat, homeLon, homeElv, 1, 2)
	s.withENL = true
	s.enl = 10
	s.hdg = 70
	s.ground(60)
	s.enl = 180
	s.roll(18, 110)
	s.run(200, func(i int) {
		s.spd = 120
		s.vs = 3
		if i%120 > 90 {
			s.hdg = math.Mod(s.hdg+1.5, 360)
		}
	})
	s.enl = 10
	s.circle(40, 90, -0.5, 12)
	s.circle(240, 90, 2.0, 12)
	if outAndReturn {
		tpLat, tpLon := homeLat+0.30, homeLon-0.10
		s.glideTo(tpLat, tpLon, 130, -0.2)
		s.circle(200, 90, 2.0, 12)
		s.glideTo(homeLat, homeLon, 130, -0.4)
	} else {
		s.circle(300, 90, -0.2, 12)
		s.glideTo(homeLat, homeLon, 110, -1.0)
	}
	s.hdg = 70
	s.land(homeElv)
	s.ground(90)
	cid := "LS"
	if outAndReturn {
		cid = "OR"
	}
	write(name, header("020726", true, "Lena Example", "LS4", "D-5678", cid, "I013638ENL"), s, []string{"E120000PEV"}, false)
}

// selfLaunch: Petra, ASG 29E D-KXYZ, engine climb, out-and-return with an
// outlanding 20 km from home on the return leg (P2).
func selfLaunch() {
	s := newSim(9*3600+30*60, homeLat, homeLon, homeElv, 2, 3)
	s.withENL, s.withMOP = true, true
	s.enl, s.mop = 5, 2
	s.hdg = 250
	s.ground(60)
	s.enl, s.mop = 780, 850
	s.ground(40)
	s.roll(12, 100)
	s.straight(300, 110, 2.5)
	s.enl, s.mop = 30, 5
	s.circle(60, 95, -0.5, 10)
	tpLat, tpLon := destination(homeLat, homeLon, 45, 40)
	s.circle(300, 95, 2.5, 12)
	s.glideTo(tpLat, tpLon, 140, -0.4)
	s.circle(200, 95, 2.0, 12)
	olLat, olLon := destination(homeLat, homeLon, 45, 20)
	s.glideTo(olLat, olLon, 120, -1.2)
	s.hdg = 225
	s.land(550)
	s.ground(180)
	write("selflaunch.igc", header("100826", true, "Petra Example", "ASG 29E", "D-KXYZ", "PX", "I023638ENL3941MOP"), s, nil, true)
}

// outlanding: Lena, ASK 21 D-1234, winch launch, then lands out 15 km NE.
func outlanding() {
	s := newSim(13*3600, homeLat, homeLon, homeElv, 1, 4)
	s.hdg = 250
	s.ground(60)
	s.roll(3, 95)
	s.run(40, func(i int) {
		switch {
		case i < 5:
			s.vs = float64(i+1) * 2.4
		case i < 33:
			s.vs = 12.5
		default:
			s.vs = 6 - float64(i-33)
		}
		s.spd = 100
	})
	s.straight(20, 95, -1.5)
	s.circle(400, 90, 1.5, 12)
	olLat, olLon := destination(homeLat, homeLon, 45, 15)
	s.glideTo(olLat, olLon, 100, -1.0)
	s.hdg = 45
	s.land(600)
	s.ground(100)
	write("outlanding.igc", header("200626", false, "Lena Example", "ASK 21", "D-1234", "LE", ""), s, nil, false)
}

// midnight: aerotow at Omaka (NZ), take-off 23:25 UTC, landing after 00:00 UTC.
func midnight() {
	lat, lon, elv := -41.54000, 173.92200, 31.0
	s := newSim(23*3600+20*60, lat, lon, elv, 2, 5)
	s.hdg = 300
	s.ground(300)
	s.roll(20, 110)
	s.straight(300, 120, 2.8)
	s.circle(40, 90, -0.5, 12)
	s.circle(1800, 90, 1.0, 12)
	s.circle(1200, 95, -0.3, 10)
	s.glideTo(lat, lon, 110, -1.5)
	s.hdg = 300
	s.land(elv)
	s.ground(120)
	write("midnight.igc", header("150125", true, "Mark Example", "Duo Discus", "ZK-GXX", "XX", ""), s, nil, false)
}

func main() {
	winch()
	aerotow("aerotow.igc", false)
	aerotow("out_and_return.igc", true)
	selfLaunch()
	outlanding()
	midnight()
}

func fmtLat(v float64) string {
	h := "N"
	if v < 0 {
		h, v = "S", -v
	}
	d := int(v)
	m := int(math.Round((v - float64(d)) * 60000))
	if m >= 60000 {
		d, m = d+1, 0
	}
	return fmt.Sprintf("%02d%05d%s", d, m, h)
}

func fmtLon(v float64) string {
	h := "E"
	if v < 0 {
		h, v = "W", -v
	}
	d := int(v)
	m := int(math.Round((v - float64(d)) * 60000))
	if m >= 60000 {
		d, m = d+1, 0
	}
	return fmt.Sprintf("%03d%05d%s", d, m, h)
}

func fmtAlt(v int) string {
	if v < 0 {
		return fmt.Sprintf("-%04d", -v)
	}
	return fmt.Sprintf("%05d", v)
}

func rad(d float64) float64 { return d * math.Pi / 180 }

func distM(lat1, lon1, lat2, lon2 float64) float64 {
	dLat := rad(lat2 - lat1)
	dLon := rad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 6371000 * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func bearing(lat1, lon1, lat2, lon2 float64) float64 {
	y := math.Sin(rad(lon2-lon1)) * math.Cos(rad(lat2))
	x := math.Cos(rad(lat1))*math.Sin(rad(lat2)) - math.Sin(rad(lat1))*math.Cos(rad(lat2))*math.Cos(rad(lon2-lon1))
	return math.Mod(math.Atan2(y, x)*180/math.Pi+360, 360)
}

// destination returns the point distKm from lat/lon on bearing brg.
func destination(lat, lon, brg, distKm float64) (float64, float64) {
	return lat + distKm/111.32*math.Cos(rad(brg)), lon + distKm/(111.32*math.Cos(rad(lat)))*math.Sin(rad(brg))
}
