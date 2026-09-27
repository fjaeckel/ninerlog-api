package engine

import (
	"sort"
	"strings"
)

// Record is the neutral input the engine evaluates (DESIGN.md section 4).
type Record struct {
	Holder      Holder       `yaml:"holder,omitempty" json:"holder,omitempty"`
	Licences    []Licence    `yaml:"licences,omitempty" json:"licences,omitempty"`
	Ratings     []Rating     `yaml:"ratings,omitempty" json:"ratings,omitempty"`
	Privileges  []Privilege  `yaml:"privileges,omitempty" json:"privileges,omitempty"`
	Credentials []Credential `yaml:"credentials,omitempty" json:"credentials,omitempty"`
	Variants    []Variant    `yaml:"variants,omitempty" json:"variants,omitempty"`
	Events      []Event      `yaml:"events,omitempty" json:"events,omitempty"`
	Flights     []Flight     `yaml:"flights,omitempty" json:"flights,omitempty"`
}

// Holder describes the pilot.
type Holder struct {
	DateOfBirth *Date `yaml:"dateOfBirth,omitempty" json:"dateOfBirth,omitempty"`
}

// Licence is one pilot licence or certificate.
type Licence struct {
	ID        string `yaml:"id" json:"id"`
	Authority string `yaml:"authority" json:"authority"`
	Type      string `yaml:"type" json:"type"`
	Kind      string `yaml:"kind,omitempty" json:"kind,omitempty"`
	Number    string `yaml:"number,omitempty" json:"number,omitempty"`
	Issued    *Date  `yaml:"issued,omitempty" json:"issued,omitempty"`
	Expires   *Date  `yaml:"expires,omitempty" json:"expires,omitempty"`
}

// Rating is one class, type or instrument rating.
type Rating struct {
	ID             string `yaml:"id" json:"id"`
	LicenceID      string `yaml:"licenceId" json:"licenceId"`
	Class          string `yaml:"class" json:"class"`
	ULKind         string `yaml:"ulKind,omitempty" json:"ulKind,omitempty"`
	TypeDesignator string `yaml:"typeDesignator,omitempty" json:"typeDesignator,omitempty"`
	Issued         *Date  `yaml:"issued,omitempty" json:"issued,omitempty"`
	ValidFrom      *Date  `yaml:"validFrom,omitempty" json:"validFrom,omitempty"`
	Expires        *Date  `yaml:"expires,omitempty" json:"expires,omitempty"`
	Notes          string `yaml:"notes,omitempty" json:"notes,omitempty"`
}

// Privilege is one licence privilege.
type Privilege struct {
	ID        string `yaml:"id" json:"id"`
	LicenceID string `yaml:"licenceId" json:"licenceId"`
	Kind      string `yaml:"kind" json:"kind"`
	Detail    string `yaml:"detail,omitempty" json:"detail,omitempty"`
	Issued    *Date  `yaml:"issued,omitempty" json:"issued,omitempty"`
	ValidFrom *Date  `yaml:"validFrom,omitempty" json:"validFrom,omitempty"`
	Expires   *Date  `yaml:"expires,omitempty" json:"expires,omitempty"`
}

// Credential is one medical, language, radio or other certificate.
type Credential struct {
	ID        string `yaml:"id" json:"id"`
	Type      string `yaml:"type" json:"type"`
	Issued    *Date  `yaml:"issued,omitempty" json:"issued,omitempty"`
	ValidFrom *Date  `yaml:"validFrom,omitempty" json:"validFrom,omitempty"`
	Expires   *Date  `yaml:"expires,omitempty" json:"expires,omitempty"`
}

// Variant is a variant within a class or type rating the holder has extended to.
type Variant struct {
	ID       string `yaml:"id" json:"id"`
	RatingID string `yaml:"ratingId" json:"ratingId"`
	Name     string `yaml:"name" json:"name"`
	Issued   *Date  `yaml:"issued,omitempty" json:"issued,omitempty"`
}

// Event is a dated occurrence that is not a flight row: a check, test, course or review.
type Event struct {
	ID             string `yaml:"id,omitempty" json:"id,omitempty"`
	Date           Date   `yaml:"date" json:"date"`
	Kind           string `yaml:"kind" json:"kind"`
	Class          string `yaml:"class,omitempty" json:"class,omitempty"`
	ULKind         string `yaml:"ulKind,omitempty" json:"ulKind,omitempty"`
	TypeDesignator string `yaml:"typeDesignator,omitempty" json:"typeDesignator,omitempty"`
	Variant        string `yaml:"variant,omitempty" json:"variant,omitempty"`
	Rating         string `yaml:"rating,omitempty" json:"rating,omitempty"`
	IsSimulator    bool   `yaml:"isSimulator,omitempty" json:"isSimulator,omitempty"`
}

// Flight is one logbook row.
type Flight struct {
	Date                  Date     `yaml:"date" json:"date"`
	Class                 string   `yaml:"class" json:"class"`
	ULKind                string   `yaml:"ulKind,omitempty" json:"ulKind,omitempty"`
	TypeDesignator        string   `yaml:"typeDesignator,omitempty" json:"typeDesignator,omitempty"`
	Variant               string   `yaml:"variant,omitempty" json:"variant,omitempty"`
	Registration          string   `yaml:"registration,omitempty" json:"registration,omitempty"`
	LaunchMethod          string   `yaml:"launchMethod,omitempty" json:"launchMethod,omitempty"`
	IsSimulator           bool     `yaml:"isSimulator,omitempty" json:"isSimulator,omitempty"`
	FSTDType              string   `yaml:"fstdType,omitempty" json:"fstdType,omitempty"`
	MTOMKg                *int     `yaml:"mtomKg,omitempty" json:"mtomKg,omitempty"`
	Tailwheel             *bool    `yaml:"tailwheel,omitempty" json:"tailwheel,omitempty"`
	Minutes               Minutes  `yaml:"minutes,omitempty" json:"minutes"`
	Takeoffs              DayNight `yaml:"takeoffs,omitempty" json:"takeoffs"`
	Landings              DayNight `yaml:"landings,omitempty" json:"landings"`
	FullStopLandings      *int     `yaml:"fullStopLandings,omitempty" json:"fullStopLandings,omitempty"`
	FullStopNightLandings *int     `yaml:"fullStopNightLandings,omitempty" json:"fullStopNightLandings,omitempty"`
	Launches              int      `yaml:"launches,omitempty" json:"launches"`
	Approaches            int      `yaml:"approaches,omitempty" json:"approaches"`
	Holds                 int      `yaml:"holds,omitempty" json:"holds"`
	InterceptAndTrack     *bool    `yaml:"interceptAndTrack,omitempty" json:"interceptAndTrack,omitempty"`
	CruiseMinutes         *int     `yaml:"cruiseMinutes,omitempty" json:"cruiseMinutes,omitempty"`
	SoleManipulator       *bool    `yaml:"soleManipulator,omitempty" json:"soleManipulator,omitempty"`
	PilotFlying           *bool    `yaml:"pilotFlying,omitempty" json:"pilotFlying,omitempty"`
	TowKind               string   `yaml:"towKind,omitempty" json:"towKind,omitempty"`
	TowedGliders          *int     `yaml:"towedGliders,omitempty" json:"towedGliders,omitempty"`
	DistanceKm            *float64 `yaml:"distanceKm,omitempty" json:"distanceKm,omitempty"`
	CheckRating           string   `yaml:"checkRating,omitempty" json:"checkRating,omitempty"`
	Flags                 Flags    `yaml:"flags,omitempty" json:"flags"`
}

// Minutes holds a flight's times in whole minutes.
type Minutes struct {
	Total               int `yaml:"total,omitempty" json:"total"`
	PIC                 int `yaml:"pic,omitempty" json:"pic"`
	Dual                int `yaml:"dual,omitempty" json:"dual"`
	SPIC                int `yaml:"spic,omitempty" json:"spic"`
	PICUS               int `yaml:"picus,omitempty" json:"picus"`
	SIC                 int `yaml:"sic,omitempty" json:"sic"`
	DualGiven           int `yaml:"dualGiven,omitempty" json:"dualGiven"`
	Examiner            int `yaml:"examiner,omitempty" json:"examiner"`
	MultiPilot          int `yaml:"multiPilot,omitempty" json:"multiPilot"`
	Night               int `yaml:"night,omitempty" json:"night"`
	IFR                 int `yaml:"ifr,omitempty" json:"ifr"`
	ActualInstrument    int `yaml:"actualInstrument,omitempty" json:"actualInstrument"`
	SimulatedInstrument int `yaml:"simulatedInstrument,omitempty" json:"simulatedInstrument"`
	CrossCountry        int `yaml:"crossCountry,omitempty" json:"crossCountry"`
}

// DayNight is a day and night count.
type DayNight struct {
	Day   int `yaml:"day,omitempty" json:"day"`
	Night int `yaml:"night,omitempty" json:"night"`
}

// Total returns day plus night.
func (d DayNight) Total() int { return d.Day + d.Night }

// Flags are a flight's boolean markers.
type Flags struct {
	ProficiencyCheck  bool `yaml:"proficiencyCheck,omitempty" json:"proficiencyCheck,omitempty"`
	FlightReview      bool `yaml:"flightReview,omitempty" json:"flightReview,omitempty"`
	IPC               bool `yaml:"ipc,omitempty" json:"ipc,omitempty"`
	TrainingFlight    bool `yaml:"trainingFlight,omitempty" json:"trainingFlight,omitempty"`
	TowFlight         bool `yaml:"towFlight,omitempty" json:"towFlight,omitempty"`
	Outlanding        bool `yaml:"outlanding,omitempty" json:"outlanding,omitempty"`
	InstructorOnBoard bool `yaml:"instructorOnBoard,omitempty" json:"instructorOnBoard,omitempty"`
	ExaminerOnBoard   bool `yaml:"examinerOnBoard,omitempty" json:"examinerOnBoard,omitempty"`
	Accompanied       bool `yaml:"accompanied,omitempty" json:"accompanied,omitempty"`
}

// Get returns the flag with the vocabulary name.
func (f Flags) Get(name string) (bool, bool) {
	switch name {
	case "proficiencyCheck":
		return f.ProficiencyCheck, true
	case "flightReview":
		return f.FlightReview, true
	case "ipc":
		return f.IPC, true
	case "trainingFlight":
		return f.TrainingFlight, true
	case "towFlight":
		return f.TowFlight, true
	case "outlanding":
		return f.Outlanding, true
	case "instructorOnBoard":
		return f.InstructorOnBoard, true
	case "examinerOnBoard":
		return f.ExaminerOnBoard, true
	case "accompanied":
		return f.Accompanied, true
	}
	return false, false
}

// Minute returns the minute field with the vocabulary name.
func (m Minutes) Minute(name string) (int, bool) {
	switch name {
	case "total":
		return m.Total, true
	case "pic":
		return m.PIC, true
	case "dual":
		return m.Dual, true
	case "spic":
		return m.SPIC, true
	case "picus":
		return m.PICUS, true
	case "sic":
		return m.SIC, true
	case "dualGiven":
		return m.DualGiven, true
	case "examiner":
		return m.Examiner, true
	case "multiPilot":
		return m.MultiPilot, true
	case "night":
		return m.Night, true
	case "ifr":
		return m.IFR, true
	case "actualInstrument":
		return m.ActualInstrument, true
	case "simulatedInstrument":
		return m.SimulatedInstrument, true
	case "crossCountry":
		return m.CrossCountry, true
	}
	return 0, false
}

// roleMinutes returns the minutes a role logged on a flight.
func roleMinutes(m Minutes, role string) int {
	switch role {
	case "pic":
		return m.PIC
	case "dual":
		return m.Dual
	case "spic":
		return m.SPIC
	case "picus":
		return m.PICUS
	case "sic":
		return m.SIC
	case "instructor":
		return m.DualGiven
	case "examiner":
		return m.Examiner
	}
	return 0
}

// normAuthority trims and upper-cases an authority (DESIGN.md decision 3).
func normAuthority(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

// flagEvents returns the events implied by flight flags.
func flagEvents(fl []Flight) []Event {
	var out []Event
	for _, f := range fl {
		add := func(kind string) {
			out = append(out, Event{
				Date: f.Date, Kind: kind, Class: f.Class, ULKind: f.ULKind,
				TypeDesignator: f.TypeDesignator, Variant: f.Variant,
				Rating: f.CheckRating, IsSimulator: f.IsSimulator,
			})
		}
		if f.Flags.ProficiencyCheck {
			add("proficiency_check")
		}
		if f.Flags.FlightReview {
			add("flight_review")
		}
		if f.Flags.IPC {
			add("ipc")
		}
	}
	return out
}

// prepared is a record indexed for evaluation.
type prepared struct {
	rec         *Record
	flights     []Flight
	events      []Event
	licences    map[string]*Licence
	licenceKind map[string]string
	ratings     []Rating
}

func prepare(rec *Record, v *Vocabulary) *prepared {
	p := &prepared{rec: rec, licences: map[string]*Licence{}, licenceKind: map[string]string{}}
	p.flights = append([]Flight(nil), rec.Flights...)
	sort.SliceStable(p.flights, func(i, j int) bool { return p.flights[i].Date.Before(p.flights[j].Date) })
	p.events = append(append([]Event(nil), rec.Events...), flagEvents(p.flights)...)
	sort.SliceStable(p.events, func(i, j int) bool { return p.events[i].Date.Before(p.events[j].Date) })
	for i := range rec.Licences {
		l := &rec.Licences[i]
		p.licences[l.ID] = l
		p.licenceKind[l.ID] = v.ClassifyLicence(l.Type, l.Authority, l.Kind)
	}
	p.ratings = rec.Ratings
	return p
}

func (p *prepared) licenceOf(licenceID string) *Licence { return p.licences[licenceID] }
