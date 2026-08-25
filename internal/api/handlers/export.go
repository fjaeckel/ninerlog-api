package handlers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightrules"
	"github.com/fjaeckel/ninerlog-api/pkg/duration"
	"github.com/gin-gonic/gin"
)

// flightChronoTime returns a comparable HH:MM:SS string for sorting flights
// within the same date. Prefers OffBlockTime (chocks-off) and falls back to
// DepartureTime (takeoff). Returns "" when neither is set so such flights
// sort before flights with a known time.
func flightChronoTime(f *models.Flight) string {
	if f.OffBlockTime != nil && *f.OffBlockTime != "" {
		return *f.OffBlockTime
	}
	if f.DepartureTime != nil && *f.DepartureTime != "" {
		return *f.DepartureTime
	}
	return ""
}

// sortFlightsChronological sorts flights ascending by Date, then by
// off-block / departure time. Falls back to flight ID for a stable order.
func sortFlightsChronological(flights []*models.Flight) {
	sort.SliceStable(flights, func(i, j int) bool {
		a, b := flights[i], flights[j]
		if !a.Date.Equal(b.Date) {
			return a.Date.Before(b.Date)
		}
		ta, tb := flightChronoTime(a), flightChronoTime(b)
		if ta != tb {
			return ta < tb
		}
		return a.ID.String() < b.ID.String()
	})
}

// csvFormulaLeaders are the characters that make Excel, LibreOffice Calc and
// Google Sheets treat a cell as a formula rather than text. A tab or carriage
// return can also lead into one once the sheet re-parses the cell.
const csvFormulaLeaders = "=+-@\t\r"

// neutralizeCSVCell defuses spreadsheet formula injection by prefixing a
// leading formula character with an apostrophe.
func neutralizeCSVCell(s string) string {
	if s == "" || !strings.ContainsRune(csvFormulaLeaders, rune(s[0])) {
		return s
	}
	return "'" + s
}

// csvWrite writes a record to the CSV writer, logging errors, passing every
// cell through neutralizeCSVCell.
func csvWrite(w *csv.Writer, record []string) {
	safe := make([]string, len(record))
	for i, field := range record {
		safe[i] = neutralizeCSVCell(field)
	}
	if err := w.Write(safe); err != nil {
		slog.Error("csv write error", "error", err)
	}
}

// exportPrefs holds user formatting preferences for export.
type exportPrefs struct {
	DateFormat       string // "DD.MM.YYYY", "MM/DD/YYYY", "YYYY-MM-DD"
	DecimalSeparator string // "comma" or "dot"
}

func (p exportPrefs) formatDate(t time.Time) string {
	switch p.DateFormat {
	case "MM/DD/YYYY":
		return t.Format("01/02/2006")
	case "YYYY-MM-DD":
		return t.Format("2006-01-02")
	default: // DD.MM.YYYY
		return t.Format("02.01.2006")
	}
}

func (p exportPrefs) formatDecimal(minutes int) string {
	s := duration.FormatDecimal(minutes)
	if p.DecimalSeparator == "comma" {
		return strings.Replace(s, ".", ",", 1)
	}
	return s
}

// ExportFlightsCSV implements GET /exports/csv
func (h *APIHandler) ExportFlightsCSV(c *gin.Context, params generated.ExportFlightsCSVParams) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	flights, err := h.flightService.ListFlights(c.Request.Context(), userID, nil)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve flights")
		return
	}
	h.attachCrewMembers(c.Request.Context(), flights)
	sortFlightsChronological(flights)

	prefs := exportPrefs{DateFormat: "DD.MM.YYYY", DecimalSeparator: "dot"}
	userName := ""
	if user, err := h.authService.GetUserByID(c.Request.Context(), userID); err == nil && user != nil {
		if user.DateFormat != "" {
			prefs.DateFormat = user.DateFormat
		}
		if user.DecimalSeparator != "" {
			prefs.DecimalSeparator = user.DecimalSeparator
		}
		userName = user.Name
	}

	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=ninerlog_flights_%s.csv", time.Now().Format("2006-01-02")))

	w := csv.NewWriter(c.Writer)

	format := "standard"
	if params.Format != nil {
		format = string(*params.Format)
	}

	switch format {
	case "easa":
		writeEASACSV(w, flights, prefs, userName)
	case "faa":
		writeFAACSV(w, flights, prefs)
	default:
		writeStandardCSV(w, flights, prefs)
	}

	w.Flush()
}

func writeStandardCSV(w *csv.Writer, flights []*models.Flight, prefs exportPrefs) {
	// Header row — compatible with ForeFlight import format
	headers := []string{
		"Date", "AircraftID", "AircraftType", "From", "To", "Route",
		"TimeOut", "TimeOff", "TimeOn", "TimeIn",
		"TotalTime", "PIC", "SIC", "Night", "Solo", "CrossCountry",
		"Distance", "DayTakeoffs", "DayLandingsFullStop",
		"NightTakeoffs", "NightLandingsFullStop", "AllLandings",
		"ActualInstrument", "SimulatedInstrument",
		"Holds", "ApproachesCount",
		"DualGiven", "DualReceived", "SimulatedFlight", "GroundTraining",
		"InstructorName", "InstructorComments",
		"FlightReview", "IPC",
		"IFRTime", "Remarks",
		"PICName", "MultiPilotTime", "FSTDType", "Endorsements",
		"PICUS", "SPIC", "ExaminerTime", "ReliefTime",
	}
	csvWrite(w, headers)

	for _, f := range flights {
		dep, arr := "", ""
		if f.DepartureICAO != nil {
			dep = *f.DepartureICAO
		}
		if f.ArrivalICAO != nil {
			arr = *f.ArrivalICAO
		}
		offBlock, onBlock, depTime, arrTime := "", "", "", ""
		if f.OffBlockTime != nil {
			offBlock = *f.OffBlockTime
		}
		if f.OnBlockTime != nil {
			onBlock = *f.OnBlockTime
		}
		if f.DepartureTime != nil {
			depTime = *f.DepartureTime
		}
		if f.ArrivalTime != nil {
			arrTime = *f.ArrivalTime
		}
		route := ""
		if f.Route != nil {
			route = *f.Route
		}
		remarks := ""
		if f.Remarks != nil {
			remarks = *f.Remarks
		}
		instrName, instrComments := "", ""
		if f.InstructorName != nil {
			instrName = *f.InstructorName
		}
		if f.InstructorComments != nil {
			instrComments = *f.InstructorComments
		}
		picName := ""
		if f.PICName != nil {
			picName = *f.PICName
		}
		fstdType := ""
		if f.FSTDType != nil {
			fstdType = *f.FSTDType
		}
		endorsements := ""
		if f.Endorsements != nil {
			endorsements = *f.Endorsements
		}

		row := []string{
			prefs.formatDate(f.Date),
			f.AircraftReg,
			f.AircraftType,
			dep,
			arr,
			route,
			offBlock,
			depTime,
			arrTime,
			onBlock,
			prefs.formatDecimal(f.TotalTime),
			prefs.formatDecimal(f.PICTime),
			prefs.formatDecimal(f.SICTime),
			prefs.formatDecimal(f.NightTime),
			prefs.formatDecimal(f.SoloTime),
			prefs.formatDecimal(f.CrossCountryTime),
			fmt.Sprintf("%.1f", f.Distance),
			fmt.Sprintf("%d", f.TakeoffsDay),
			fmt.Sprintf("%d", f.LandingsDay),
			fmt.Sprintf("%d", f.TakeoffsNight),
			fmt.Sprintf("%d", f.LandingsNight),
			fmt.Sprintf("%d", f.AllLandings),
			prefs.formatDecimal(f.ActualInstrumentTime),
			prefs.formatDecimal(f.SimulatedInstrumentTime),
			fmt.Sprintf("%d", f.Holds),
			fmt.Sprintf("%d", f.ApproachesCount),
			prefs.formatDecimal(f.DualGivenTime),
			prefs.formatDecimal(f.DualTime),
			prefs.formatDecimal(f.SimulatedFlightTime),
			prefs.formatDecimal(f.GroundTrainingTime),
			instrName,
			instrComments,
			fmt.Sprintf("%t", f.IsFlightReview),
			fmt.Sprintf("%t", f.IsIPC),
			prefs.formatDecimal(f.IFRTime),
			remarks,
			picName,
			prefs.formatDecimal(f.MultiPilotTime),
			fstdType,
			endorsements,
			prefs.formatDecimal(f.PICUSTime),
			prefs.formatDecimal(f.SPICTime),
			prefs.formatDecimal(f.ExaminerTime),
			prefs.formatDecimal(f.ReliefTime),
		}
		csvWrite(w, row)
	}
}

func writeEASACSV(w *csv.Writer, flights []*models.Flight, prefs exportPrefs, userName string) {
	// EASA AMC1 FCL.050 columns (24 cols)
	headers := []string{
		"Date", "Dep Place", "Dep Time", "Arr Place", "Arr Time",
		"A/C Type", "A/C Reg",
		"SP-SE", "SP-ME", "Multi-Pilot",
		"Total Time",
		"PIC Name",
		"Ldg Day", "Ldg Night",
		"Night", "IFR",
		"PIC", "Co-Pilot", "Dual", "Instructor",
		"FSTD Date", "FSTD Type", "FSTD Time",
		"Remarks & Endorsements",
	}
	csvWrite(w, headers)

	for _, f := range flights {
		dep := safeStrCSV(f.DepartureICAO)
		arr := safeStrCSV(f.ArrivalICAO)
		depTime := fmtTimeCSV(f.OffBlockTime)
		arrTime := fmtTimeCSV(f.OnBlockTime)
		picName := flightrules.DisplayPICName(f, userName)

		// SP-SE / SP-ME / MP from the centralised rule; acClass is empty
		// (CSV has no access to the user's aircraft fleet here), defaulting
		// non-MP rows to SP-SE.
		spSEMin, spMEMin, mpMin := flightrules.RowTimes(f, "")
		spSE, spME, mp := "", "", ""
		if spSEMin > 0 {
			spSE = fmtHM(spSEMin)
		}
		if spMEMin > 0 {
			spME = fmtHM(spMEMin)
		}
		if mpMin > 0 {
			mp = fmtHM(mpMin)
		}

		// FSTD columns
		fstdDate, fstdType, fstdTime := flightrules.FSTDFields(f, "02.01.2006", fmtHM)

		remarksAndEndorsements := flightrules.CombinedRemarks(f)

		row := []string{
			f.Date.Format("02.01.2006"),
			dep, depTime, arr, arrTime,
			f.AircraftType, f.AircraftReg,
			spSE, spME, mp,
			fmtHM(f.TotalTime),
			picName,
			fmt.Sprintf("%d", f.LandingsDay), fmt.Sprintf("%d", f.LandingsNight),
			fmtHM(f.NightTime), fmtHM(flightrules.EffectiveIFRTime(f)),
			fmtHM(flightrules.PICColumnTime(f)), fmtHM(flightrules.CoPilotColumnTime(f)), fmtHM(f.DualTime), fmtHM(f.DualGivenTime),
			fstdDate, fstdType, fstdTime,
			remarksAndEndorsements,
		}
		csvWrite(w, row)
	}
}

func writeFAACSV(w *csv.Writer, flights []*models.Flight, prefs exportPrefs) {
	// FAA standard logbook columns (ASA/Jeppesen layout)
	headers := []string{
		"Date", "A/C Type", "A/C Ident", "From", "To",
		"Solo", "PIC", "SIC", "Dual Rcvd", "Instr Given",
		"Actual Inst", "Sim Inst", "XC", "Night",
		"Day Ldg", "Night Ldg",
		"Approaches", "Holds",
		"Total",
		"Remarks/Endorsements",
	}
	csvWrite(w, headers)

	for _, f := range flights {
		dep := safeStrCSV(f.DepartureICAO)
		arr := safeStrCSV(f.ArrivalICAO)

		remarks := flightrules.CombinedRemarks(f, flightrules.FlagIPC, flightrules.FlagFlightReview)

		row := []string{
			f.Date.Format("01/02/2006"),
			f.AircraftType, f.AircraftReg,
			dep, arr,
			duration.FormatDecimal(f.SoloTime),
			duration.FormatDecimal(f.PICTime),
			duration.FormatDecimal(flightrules.FAASICColumnTime(f)),
			duration.FormatDecimal(flightrules.FAADualColumnTime(f)),
			duration.FormatDecimal(f.DualGivenTime),
			duration.FormatDecimal(f.ActualInstrumentTime),
			duration.FormatDecimal(f.SimulatedInstrumentTime),
			duration.FormatDecimal(f.CrossCountryTime),
			duration.FormatDecimal(f.NightTime),
			fmt.Sprintf("%d", f.LandingsDay),
			fmt.Sprintf("%d", f.LandingsNight),
			fmt.Sprintf("%d", f.ApproachesCount),
			fmt.Sprintf("%d", f.Holds),
			duration.FormatDecimal(f.TotalTime),
			remarks,
		}
		csvWrite(w, row)
	}
}

func safeStrCSV(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func fmtTimeCSV(s *string) string {
	if s == nil {
		return ""
	}
	v := *s
	if len(v) >= 5 {
		return v[:5]
	}
	return v
}

func fmtHM(v int) string {
	if v == 0 {
		return ""
	}
	h := v / 60
	m := v % 60
	return fmt.Sprintf("%d:%02d", h, m)
}

// ExportDataJSON implements GET /exports/json
// BackupPayloadBuilder composes the services that make up a backup payload.
// GET /exports/json and every cloud backup run share it, so the two can never
// carry different sets of a user's data.
func (h *APIHandler) BackupPayloadBuilder() *cloudbackup.DefaultJSONBuilder {
	return &cloudbackup.DefaultJSONBuilder{
		Flights:        h.flightService,
		Aircraft:       h.aircraftService,
		Licenses:       h.licenseService,
		Credentials:    h.credentialService,
		ClassRating:    h.classRatingService,
		Contacts:       h.contactService,
		CustomCurrency: h.customCurrencyService,
		Notifications:  h.notificationService,
		AttachCrew:     h.AttachCrewMembers,
	}
}

// ExportDataJSON implements GET /exports/json.
func (h *APIHandler) ExportDataJSON(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	backup, err := h.BackupPayloadBuilder().Gather(c.Request.Context(), userID)
	if err != nil {
		slog.Error("json export gather error", "error", err)
		h.sendError(c, http.StatusInternalServerError, "Failed to build export")
		return
	}

	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=ninerlog_backup_%s.json", time.Now().Format("2006-01-02")))

	encoder := json.NewEncoder(c.Writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(backup); err != nil {
		slog.Error("json export encode error", "error", err)
	}
}
