package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

// signedFields is the canonical form of the flight fields a signature
// attests. Field order is part of the hash.
type signedFields struct {
	Date                    string           `json:"date"`
	AircraftReg             string           `json:"aircraftReg"`
	AircraftType            string           `json:"aircraftType"`
	DepartureICAO           *string          `json:"departureIcao"`
	ArrivalICAO             *string          `json:"arrivalIcao"`
	OffBlockTime            *string          `json:"offBlockTime"`
	OnBlockTime             *string          `json:"onBlockTime"`
	DepartureTime           *string          `json:"departureTime"`
	ArrivalTime             *string          `json:"arrivalTime"`
	TotalTime               int              `json:"totalTime"`
	PICTime                 int              `json:"picTime"`
	DualTime                int              `json:"dualTime"`
	NightTime               int              `json:"nightTime"`
	IFRTime                 int              `json:"ifrTime"`
	SoloTime                int              `json:"soloTime"`
	CrossCountryTime        int              `json:"crossCountryTime"`
	SICTime                 int              `json:"sicTime"`
	DualGivenTime           int              `json:"dualGivenTime"`
	PICUSTime               int              `json:"picusTime"`
	SPICTime                int              `json:"spicTime"`
	ExaminerTime            int              `json:"examinerTime"`
	ReliefTime              int              `json:"reliefTime"`
	MultiPilotTime          int              `json:"multiPilotTime"`
	SimulatedFlightTime     int              `json:"simulatedFlightTime"`
	GroundTrainingTime      int              `json:"groundTrainingTime"`
	ActualInstrumentTime    int              `json:"actualInstrumentTime"`
	SimulatedInstrumentTime int              `json:"simulatedInstrumentTime"`
	LandingsDay             int              `json:"landingsDay"`
	LandingsNight           int              `json:"landingsNight"`
	AllLandings             int              `json:"allLandings"`
	TakeoffsDay             int              `json:"takeoffsDay"`
	TakeoffsNight           int              `json:"takeoffsNight"`
	Remarks                 *string          `json:"remarks"`
	InstructorName          *string          `json:"instructorName"`
	InstructorComments      *string          `json:"instructorComments"`
	PICName                 *string          `json:"picName"`
	Crew                    []models.CrewKey `json:"crew"`
}

// SignedFieldsHash returns the SHA-256 (hex) over the canonical signed fields
// of a flight and its crew.
func SignedFieldsHash(f *models.Flight, crew []models.FlightCrewMember) string {
	fields := signedFields{
		Date:                    f.Date.Format("2006-01-02"),
		AircraftReg:             f.AircraftReg,
		AircraftType:            f.AircraftType,
		DepartureICAO:           f.DepartureICAO,
		ArrivalICAO:             f.ArrivalICAO,
		OffBlockTime:            f.OffBlockTime,
		OnBlockTime:             f.OnBlockTime,
		DepartureTime:           f.DepartureTime,
		ArrivalTime:             f.ArrivalTime,
		TotalTime:               f.TotalTime,
		PICTime:                 f.PICTime,
		DualTime:                f.DualTime,
		NightTime:               f.NightTime,
		IFRTime:                 f.IFRTime,
		SoloTime:                f.SoloTime,
		CrossCountryTime:        f.CrossCountryTime,
		SICTime:                 f.SICTime,
		DualGivenTime:           f.DualGivenTime,
		PICUSTime:               f.PICUSTime,
		SPICTime:                f.SPICTime,
		ExaminerTime:            f.ExaminerTime,
		ReliefTime:              f.ReliefTime,
		MultiPilotTime:          f.MultiPilotTime,
		SimulatedFlightTime:     f.SimulatedFlightTime,
		GroundTrainingTime:      f.GroundTrainingTime,
		ActualInstrumentTime:    f.ActualInstrumentTime,
		SimulatedInstrumentTime: f.SimulatedInstrumentTime,
		LandingsDay:             f.LandingsDay,
		LandingsNight:           f.LandingsNight,
		AllLandings:             f.AllLandings,
		TakeoffsDay:             f.TakeoffsDay,
		TakeoffsNight:           f.TakeoffsNight,
		Remarks:                 f.Remarks,
		InstructorName:          f.InstructorName,
		InstructorComments:      f.InstructorComments,
		PICName:                 f.PICName,
		Crew:                    models.CrewKeys(crew),
	}
	raw, _ := json.Marshal(fields)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
