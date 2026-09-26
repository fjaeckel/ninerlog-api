package currency

import (
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// lastDayInWindow returns the last date D whose window of the given length,
// evaluated during D, still contains flown: D − window before flown.
func lastDayInWindow(flown time.Time, years, months, days int) string {
	d := flown.AddDate(years, months, days)
	for !d.AddDate(-years, -months, -days).Before(flown) {
		d = d.AddDate(0, 0, -1)
	}
	return d.Format("2006-01-02")
}

// laplLog returns twelve 1h/1-landing SEP flights on days 10, 20 … 120 ago
// and a 1h dual flight 300 days ago.
func laplLog() *flightLog {
	l := &flightLog{}
	for i := 1; i <= 12; i++ {
		l.flights = append(l.flights, logFlight{date: today().AddDate(0, 0, -10*i), class: models.ClassTypeSEPLand, total: 60, pic: 60, landings: 1, takeoffs: 1})
	}
	l.flights = append(l.flights, logFlight{date: today().AddDate(0, 0, -300), class: models.ClassTypeSEPLand, total: 60, dual: 60})
	return l
}

func laplRating() (*models.License, *models.ClassRating) {
	license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "LAPL"}
	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, LicenseID: license.ID}
	return license, rating
}

func TestRecencyExpiresOn_LAPL_EarliestRequirementLapse(t *testing.T) {
	license, rating := laplRating()
	r := evalAll(t, laplLog(), license, rating).Ratings[0]
	mustEqual(t, StatusCurrent, r.Status, "status")
	mustNotNil(t, r.RecencyExpiresOn, "recencyExpiresOn")
	// The training flight 300 days ago leaves the window before the 12h/12 landings lapse.
	checkEqual(t, lastDayInWindow(today().AddDate(0, 0, -300), 2, 0, 0), *r.RecencyExpiresOn)
}

func TestRecencyExpiresOn_LAPL_ExperienceLapsesFirst(t *testing.T) {
	log := laplLog()
	log.flights = append(log.flights, logFlight{date: today().AddDate(0, 0, -5), class: models.ClassTypeSEPLand, total: 60, dual: 60, landings: 1, takeoffs: 1})
	license, rating := laplRating()
	r := evalAll(t, log, license, rating).Ratings[0]
	mustNotNil(t, r.RecencyExpiresOn, "recencyExpiresOn")
	// 14h and 13 landings: the flights 300 and 120 days ago can leave the window, the one 110 days ago cannot.
	checkEqual(t, lastDayInWindow(today().AddDate(0, 0, -110), 2, 0, 0), *r.RecencyExpiresOn)
}

func TestRecencyExpiresOn_LAPL_ProficiencyCheckExtends(t *testing.T) {
	log := laplLog()
	log.flights = append(log.flights, logFlight{date: today().AddDate(0, 0, -2), class: models.ClassTypeSEPLand, total: 60, profCheck: true})
	license, rating := laplRating()
	r := evalAll(t, log, license, rating).Ratings[0]
	mustNotNil(t, r.RecencyExpiresOn, "recencyExpiresOn")
	checkEqual(t, lastDayInWindow(today().AddDate(0, 0, -2), 2, 0, 0), *r.RecencyExpiresOn)
}

func TestRecencyExpiresOn_NotCurrent(t *testing.T) {
	log := laplLog()
	log.flights = log.flights[1:]
	license, rating := laplRating()
	r := evalAll(t, log, license, rating).Ratings[0]
	mustTrue(t, r.Status != StatusCurrent, "status not current")
	checkNil(t, r.RecencyExpiresOn, "recencyExpiresOn")
}

func TestRecencyExpiresOn_FAAInstrument(t *testing.T) {
	log := &flightLog{flights: []logFlight{
		{date: today().AddDate(0, 0, -20), class: models.ClassTypeSEPLand, total: 60, approaches: 4, holds: 1},
		{date: today().AddDate(0, 0, -40), class: models.ClassTypeSEPLand, total: 60, approaches: 3},
	}}
	license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: "PPL"}
	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeIR, LicenseID: license.ID}
	r := evalAll(t, log, license, rating).Ratings[0]
	mustEqual(t, StatusCurrent, r.Status, "status")
	mustNotNil(t, r.RecencyExpiresOn, "recencyExpiresOn")
	checkEqual(t, lastDayInWindow(today().AddDate(0, 0, -40), 0, 6, 0), *r.RecencyExpiresOn)
}

func TestRecencyExpiresOn_RevalidationRuleOmitted(t *testing.T) {
	license, rating := laplRating()
	license.LicenseType = "PPL"
	exp := today().AddDate(0, 0, 200)
	rating.ExpiryDate = &exp
	r := evalAll(t, laplLog(), license, rating).Ratings[0]
	checkNil(t, r.RecencyExpiresOn, "recencyExpiresOn")
}
