package handlers

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightcalc"
	"github.com/fjaeckel/ninerlog-api/pkg/duration"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// In-memory upload session store (token → parsed rows)
type uploadSession struct {
	userID    uuid.UUID
	fileName  string
	format    generated.ImportFormat
	columns   []string
	rows      []map[string]string
	aircraft  []map[string]string // ForeFlight Aircraft Table rows
	createdAt time.Time
}

var (
	uploadSessions = make(map[string]*uploadSession)
	sessionMu      sync.Mutex
)

func newUploadToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func cleanupOldSessions() {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	cutoff := time.Now().Add(-1 * time.Hour)
	for token, s := range uploadSessions {
		if s.createdAt.Before(cutoff) {
			delete(uploadSessions, token)
		}
	}
}

// ForeFlight known column headers
var foreflightColumns = []string{
	"Date", "AircraftID", "From", "To", "Route", "TimeOut", "TimeOff", "TimeOn", "TimeIn",
	"OnDuty", "OffDuty", "TotalTime", "PIC", "SIC", "Night", "Solo", "CrossCountry",
	"NVG", "NVG Ops", "Distance", "DayTakeoffs", "DayLandingsFullStop", "NightTakeoffs",
	"NightLandingsFullStop", "AllLandings", "ActualInstrument", "SimulatedInstrument",
	"HobbsStart", "HobbsEnd", "TachStart", "TachEnd", "Holds", "Approach1", "Approach2",
	"Approach3", "Approach4", "Approach5", "Approach6", "DualGiven", "DualReceived",
	"SimulatedFlight", "GroundTraining", "InstructorName", "InstructorComments",
	"Person1", "Person2", "Person3", "Person4", "Person5", "Person6",
	"FlightReview", "Checkride", "IPC", "NVG Proficiency", "FAA6158",
	"PilotComments",
}

func isForeFlight(headers []string) bool {
	if len(headers) < 10 {
		return false
	}
	matches := 0
	ffSet := make(map[string]bool)
	for _, h := range foreflightColumns {
		ffSet[strings.ToLower(h)] = true
	}
	for _, h := range headers {
		if ffSet[strings.ToLower(strings.TrimSpace(h))] {
			matches++
		}
	}
	return matches >= 8
}

func suggestForeFlight() []generated.ImportColumnMapping {
	mapping := map[string]generated.ImportField{
		"Date":                  "date",
		"AircraftID":            "aircraftReg",
		"From":                  "departureIcao",
		"To":                    "arrivalIcao",
		"Route":                 "route",
		"TimeOut":               "offBlockTime",
		"TimeIn":                "onBlockTime",
		"TimeOff":               "departureTime",
		"TimeOn":                "arrivalTime",
		"TotalTime":             "totalTime",
		"PIC":                   "isPic",
		"Night":                 "nightTime",
		"DualReceived":          "isDual",
		"ActualInstrument":      "actualInstrumentTime",
		"SimulatedInstrument":   "simulatedInstrumentTime",
		"DayLandingsFullStop":   "landingsDay",
		"NightLandingsFullStop": "landingsNight",
		"Holds":                 "holds",
		"FlightReview":          "isFlightReview",
		"IPC":                   "isIpc",
		"PilotComments":         "remarks",
		"InstructorName":        "instructorName",
		"InstructorComments":    "instructorComments",
		"DualGiven":             "dualGivenTime",
		"Person1":               "person1",
		"Person2":               "person2",
		"Person3":               "person3",
		"Person4":               "person4",
		"Person5":               "person5",
		"Person6":               "person6",
	}

	var result []generated.ImportColumnMapping
	for src, tgt := range mapping {
		m := generated.ImportColumnMapping{
			SourceColumn: src,
			TargetField:  tgt,
		}
		if tgt == "date" {
			df := "2006-01-02"
			m.DateFormat = &df
		}
		result = append(result, m)
	}
	return result
}

func suggestGenericCSV(headers []string) []generated.ImportColumnMapping {
	guesses := map[string]generated.ImportField{
		"date": "date", "flight date": "date", "datum": "date",
		"aircraft": "aircraftReg", "registration": "aircraftReg", "reg": "aircraftReg", "aircraftid": "aircraftReg", "aircraft reg": "aircraftReg",
		"type": "aircraftType", "aircraft type": "aircraftType", "typecode": "aircraftType",
		"from": "departureIcao", "departure": "departureIcao", "dep": "departureIcao", "departure icao": "departureIcao",
		"to": "arrivalIcao", "arrival": "arrivalIcao", "arr": "arrivalIcao", "dest": "arrivalIcao", "arrival icao": "arrivalIcao",
		"off block": "offBlockTime", "offblock": "offBlockTime", "timeout": "offBlockTime", "chocks off": "offBlockTime",
		"on block": "onBlockTime", "onblock": "onBlockTime", "timein": "onBlockTime", "chocks on": "onBlockTime",
		"takeoff": "departureTime", "timeoff": "departureTime", "departure time": "departureTime",
		"landing": "arrivalTime", "timeon": "arrivalTime", "arrival time": "arrivalTime",
		"total": "totalTime", "total time": "totalTime", "totaltime": "totalTime", "block time": "totalTime",
		"pic": "isPic", "pic time": "isPic",
		"dual": "isDual", "dual received": "isDual", "dualreceived": "isDual",
		"night": "nightTime", "night time": "nightTime",
		"ifr": "ifrTime", "ifr time": "ifrTime", "instrument": "ifrTime", "actual instrument": "ifrTime", "actualinstrument": "ifrTime",
		"day landings": "landingsDay", "daylandingsfullstop": "landingsDay", "day ldg": "landingsDay",
		"night landings": "landingsNight", "nightlandingsfullstop": "landingsNight", "night ldg": "landingsNight",
		"remarks": "remarks", "comments": "remarks", "pilotcomments": "remarks", "notes": "remarks",
	}

	var result []generated.ImportColumnMapping
	for _, h := range headers {
		lower := strings.ToLower(strings.TrimSpace(h))
		if field, ok := guesses[lower]; ok {
			m := generated.ImportColumnMapping{
				SourceColumn: h,
				TargetField:  field,
			}
			if field == "date" {
				df := "2006-01-02"
				m.DateFormat = &df
			}
			result = append(result, m)
		}
	}
	return result
}

// UploadImportFile implements POST /imports/upload
func (h *APIHandler) UploadImportFile(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	cleanupOldSessions()

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		h.sendError(c, http.StatusBadRequest, "No file uploaded")
		return
	}
	defer file.Close()

	if header.Size > 10*1024*1024 {
		h.sendError(c, http.StatusBadRequest, "File too large (max 10 MB)")
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		h.sendError(c, http.StatusBadRequest, "Failed to read file")
		return
	}

	fileName := header.Filename
	var columns []string
	var rows []map[string]string
	var aircraftData []map[string]string
	var format generated.ImportFormat

	lower := strings.ToLower(fileName)
	if strings.HasSuffix(lower, ".csv") || strings.HasSuffix(lower, ".txt") {
		columns, rows, aircraftData, err = parseCSV(data)
		if err != nil {
			h.sendError(c, http.StatusBadRequest, "Failed to parse CSV file")
			return
		}
		if isForeFlight(columns) {
			format = "FOREFLIGHT_CSV"
		} else {
			format = "CSV"
		}
	} else {
		h.sendError(c, http.StatusBadRequest, "Unsupported file format. Please upload a CSV file.")
		return
	}

	token := newUploadToken()
	sessionMu.Lock()
	uploadSessions[token] = &uploadSession{
		userID:    userID,
		fileName:  fileName,
		format:    format,
		columns:   columns,
		rows:      rows,
		aircraft:  aircraftData,
		createdAt: time.Now(),
	}
	sessionMu.Unlock()

	// Build preview rows (first 5)
	previewRows := rows
	if len(previewRows) > 5 {
		previewRows = previewRows[:5]
	}

	// Suggest mappings
	var suggested []generated.ImportColumnMapping
	if format == "FOREFLIGHT_CSV" {
		suggested = suggestForeFlight()
	} else {
		suggested = suggestGenericCSV(columns)
	}

	c.JSON(http.StatusOK, generated.ImportUploadResponse{
		UploadToken:       token,
		Format:            format,
		Columns:           columns,
		PreviewRows:       previewRows,
		TotalRows:         len(rows),
		SuggestedMappings: suggested,
	})
}

// PreviewImport implements POST /imports/preview
func (h *APIHandler) PreviewImport(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req generated.ImportPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	sessionMu.Lock()
	session, ok := uploadSessions[req.UploadToken]
	sessionMu.Unlock()
	if !ok || session.userID != userID {
		h.sendError(c, http.StatusNotFound, "Upload session not found or expired")
		return
	}

	// Build mapping lookup: sourceColumn → targetField
	mappingLookup := make(map[string]generated.ImportColumnMapping)
	for _, m := range req.Mappings {
		mappingLookup[m.SourceColumn] = m
	}

	// Get existing flights for duplicate detection
	existingFlights, _ := h.flightService.ListFlights(c.Request.Context(), userID, nil)

	var flights []generated.ImportPreviewFlight
	validCount, dupCount, errCount := 0, 0, 0

	for i, row := range session.rows {
		flight, errs := mapRowToFlight(row, mappingLookup)
		rowIdx := i + 1

		if len(errs) > 0 {
			errDetails := make([]struct {
				Field   *string `json:"field,omitempty"`
				Message *string `json:"message,omitempty"`
			}, len(errs))
			for j, e := range errs {
				errDetails[j] = struct {
					Field   *string `json:"field,omitempty"`
					Message *string `json:"message,omitempty"`
				}{Field: &e.field, Message: &e.message}
			}
			flights = append(flights, generated.ImportPreviewFlight{
				RowIndex: rowIdx,
				Status:   "error",
				Flight:   flight,
				Errors:   &errDetails,
			})
			errCount++
			continue
		}

		// Duplicate detection
		skipDups := req.SkipDuplicates == nil || *req.SkipDuplicates
		if skipDups {
			if dupID := findDuplicate(flight, existingFlights); dupID != nil {
				flights = append(flights, generated.ImportPreviewFlight{
					RowIndex:         rowIdx,
					Status:           "duplicate",
					Flight:           flight,
					ExistingFlightId: dupID,
				})
				dupCount++
				continue
			}
		}

		flights = append(flights, generated.ImportPreviewFlight{
			RowIndex: rowIdx,
			Status:   "valid",
			Flight:   flight,
		})
		validCount++
	}

	c.JSON(http.StatusOK, generated.ImportPreviewResponse{
		UploadToken:    req.UploadToken,
		TotalRows:      len(session.rows),
		ValidCount:     validCount,
		DuplicateCount: dupCount,
		ErrorCount:     errCount,
		Flights:        flights,
	})
}

// ConfirmImport implements POST /imports/confirm
func (h *APIHandler) ConfirmImport(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req generated.ConfirmImportJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	sessionMu.Lock()
	session, ok := uploadSessions[req.UploadToken]
	sessionMu.Unlock()
	if !ok || session.userID != userID {
		h.sendError(c, http.StatusNotFound, "Upload session not found or expired")
		return
	}

	// Re-run preview to get validated flights
	// For simplicity, we re-parse — in production you'd cache preview results

	// Build selected row set
	selectedSet := make(map[int]bool)
	importAll := req.SelectedRows == nil || len(*req.SelectedRows) == 0
	if !importAll {
		for _, idx := range *req.SelectedRows {
			selectedSet[idx] = true
		}
	}
	includeDups := req.IncludeDuplicates != nil && *req.IncludeDuplicates

	// Get existing flights for duplicate check
	existingFlights, _ := h.flightService.ListFlights(c.Request.Context(), userID, nil)

	// Get the preview mappings from the session (we stored them in the session during upload)
	// Since we don't store mappings in the session, we'll need caller to include them
	// For now, we use the suggestedMappings from the format
	var mappingLookup map[string]generated.ImportColumnMapping
	// The confirm endpoint doesn't have mappings in the OpenAPI schema — they were applied during preview.
	// We'll use the suggested mappings based on format as fallback
	if session.format == "FOREFLIGHT_CSV" {
		mappingLookup = make(map[string]generated.ImportColumnMapping)
		for _, m := range suggestForeFlight() {
			mappingLookup[m.SourceColumn] = m
		}
	} else {
		mappingLookup = make(map[string]generated.ImportColumnMapping)
		for _, m := range suggestGenericCSV(session.columns) {
			mappingLookup[m.SourceColumn] = m
		}
	}

	// Auto-create aircraft from ForeFlight Aircraft Table if present
	if len(session.aircraft) > 0 {
		existingAircraft, _ := h.aircraftService.ListAircraft(c.Request.Context(), userID)
		existingRegs := make(map[string]bool)
		for _, a := range existingAircraft {
			existingRegs[strings.ToUpper(a.Registration)] = true
		}
		for _, acRow := range session.aircraft {
			reg := strings.ToUpper(strings.TrimSpace(acRow["AircraftID"]))
			if reg == "" || existingRegs[reg] {
				continue
			}
			typeCode := strings.ToUpper(strings.TrimSpace(acRow["TypeCode"]))
			make_ := strings.TrimSpace(acRow["Make"])
			model_ := strings.TrimSpace(acRow["Model"])
			if typeCode == "" {
				typeCode = reg
			}
			if make_ == "" {
				make_ = typeCode
			}
			if model_ == "" {
				model_ = typeCode
			}

			// Map ForeFlight class to aircraft class
			var aircraftClass *string
			ffClass := strings.ToLower(strings.TrimSpace(acRow["Class"]))
			switch ffClass {
			case "airplane_single_engine_land":
				c := "SEP_LAND"
				aircraftClass = &c
			case "airplane_single_engine_sea":
				c := "SEP_SEA"
				aircraftClass = &c
			case "airplane_multi_engine_land":
				c := "MEP_LAND"
				aircraftClass = &c
			case "airplane_multi_engine_sea":
				c := "MEP_SEA"
				aircraftClass = &c
			}

			newAircraft := &models.Aircraft{
				UserID:        userID,
				Registration:  reg,
				Type:          typeCode,
				Make:          make_,
				Model:         model_,
				IsActive:      true,
				AircraftClass: aircraftClass,
			}
			_ = h.aircraftService.CreateAircraft(c.Request.Context(), newAircraft)
			existingRegs[reg] = true
		}
	}

	var importedIDs []openapi_types.UUID
	var importErrors []struct {
		Field    string  `json:"field"`
		Message  string  `json:"message"`
		RawValue *string `json:"rawValue,omitempty"`
		RowIndex int     `json:"rowIndex"`
	}
	imported, skipped, errored, dups := 0, 0, 0, 0
	contactsCreated := 0
	// Cache for contact lookups to avoid repeated DB queries
	contactCache := make(map[string]*models.Contact) // lowercase name → contact

	for i, row := range session.rows {
		rowIdx := i + 1
		if !importAll && !selectedSet[rowIdx] {
			skipped++
			continue
		}

		flight, errs := mapRowToFlight(row, mappingLookup)
		if len(errs) > 0 {
			for _, e := range errs {
				importErrors = append(importErrors, struct {
					Field    string  `json:"field"`
					Message  string  `json:"message"`
					RawValue *string `json:"rawValue,omitempty"`
					RowIndex int     `json:"rowIndex"`
				}{Field: e.field, Message: e.message, RowIndex: rowIdx})
			}
			errored++
			continue
		}

		if !includeDups {
			if findDuplicate(flight, existingFlights) != nil {
				dups++
				skipped++
				continue
			}
		}

		// Create the flight
		totalTime := 0
		if flight.OffBlockTime != "" && flight.OnBlockTime != "" {
			totalTime, _ = calculateBlockTime(flight.OffBlockTime, flight.OnBlockTime)
		} else if flight.TotalTime != nil {
			totalTime = *flight.TotalTime
		}

		flightDate, _ := time.Parse("2006-01-02", flight.Date.String())

		offBlock := flight.OffBlockTime
		onBlock := flight.OnBlockTime
		depTime := flight.DepartureTime
		arrTime := flight.ArrivalTime

		newFlight := models.Flight{
			UserID:                  userID,
			Date:                    flightDate,
			AircraftReg:             flight.AircraftReg,
			AircraftType:            flight.AircraftType,
			DepartureICAO:           &flight.DepartureIcao,
			ArrivalICAO:             &flight.ArrivalIcao,
			TotalTime:               totalTime,
			IFRTime:                 getIntOrDefault(flight.IfrTime, 0),
			AllLandings:             flight.Landings,
			ActualInstrumentTime:    getIntOrDefault(flight.ActualInstrumentTime, 0),
			SimulatedInstrumentTime: getIntOrDefault(flight.SimulatedInstrumentTime, 0),
		}
		if flight.Holds != nil {
			newFlight.Holds = *flight.Holds
		}
		if flight.ApproachesCount != nil {
			newFlight.ApproachesCount = *flight.ApproachesCount
		}
		if flight.IsIpc != nil {
			newFlight.IsIPC = *flight.IsIpc
		}
		if flight.IsFlightReview != nil {
			newFlight.IsFlightReview = *flight.IsFlightReview
		}
		if flight.Route != nil {
			newFlight.Route = flight.Route
		}
		if offBlock != "" {
			newFlight.OffBlockTime = &offBlock
		}
		if onBlock != "" {
			newFlight.OnBlockTime = &onBlock
		}
		if depTime != nil && *depTime != "" {
			newFlight.DepartureTime = depTime
		}
		if arrTime != nil && *arrTime != "" {
			newFlight.ArrivalTime = arrTime
		}
		if flight.Remarks != nil {
			newFlight.Remarks = flight.Remarks
		}
		if flight.InstructorName != nil {
			newFlight.InstructorName = flight.InstructorName
		}
		if flight.InstructorComments != nil {
			newFlight.InstructorComments = flight.InstructorComments
		}
		newFlight.DualGivenTime = getIntOrDefault(flight.DualGivenTime, 0)

		// Build crew members from FlightCreate into model for auto-calculations
		if flight.CrewMembers != nil {
			for _, cm := range *flight.CrewMembers {
				member := models.FlightCrewMember{
					Name: cm.Name,
					Role: models.CrewRole(cm.Role),
				}
				newFlight.CrewMembers = append(newFlight.CrewMembers, member)
			}
		}

		// Apply auto-calculations (solo, cross-country, distance, night, landing split, PIC/Dual)
		flightcalc.ApplyAutoCalculations(&newFlight)

		if err := h.flightService.CreateFlight(c.Request.Context(), &newFlight); err != nil {
			importErrors = append(importErrors, struct {
				Field    string  `json:"field"`
				Message  string  `json:"message"`
				RawValue *string `json:"rawValue,omitempty"`
				RowIndex int     `json:"rowIndex"`
			}{Field: "flight", Message: "Failed to import flight", RowIndex: rowIdx})
			errored++
			continue
		}

		// Find or create contacts and persist crew members
		if len(newFlight.CrewMembers) > 0 && h.flightCrewRepo != nil {
			for i := range newFlight.CrewMembers {
				name := strings.TrimSpace(newFlight.CrewMembers[i].Name)
				if name == "" {
					continue
				}
				cacheKey := strings.ToLower(name)
				contact, ok := contactCache[cacheKey]
				if !ok {
					var created bool
					contact, created, _ = h.contactService.FindOrCreateContact(c.Request.Context(), userID, name)
					if contact != nil {
						contactCache[cacheKey] = contact
						if created {
							contactsCreated++
						}
					}
				}
				if contact != nil {
					newFlight.CrewMembers[i].ContactID = &contact.ID
				}
			}
			_ = h.flightCrewRepo.SetCrewMembers(c.Request.Context(), newFlight.ID, newFlight.CrewMembers)
		}

		importedIDs = append(importedIDs, openapi_types.UUID(newFlight.ID))
		imported++
	}

	// Determine status
	var status generated.ImportStatus
	if errored == 0 {
		status = "completed"
	} else if imported > 0 {
		status = "partial"
	} else {
		status = "failed"
	}

	// Save import record to DB
	importID := uuid.New()
	errorsJSON, _ := json.Marshal(importErrors)
	mappingsJSON, _ := json.Marshal(mappingLookup)
	_, _ = h.db.ExecContext(c.Request.Context(), `
		INSERT INTO flight_imports (id, user_id, file_name, import_format, import_status,
			total_rows, imported_count, skipped_count, error_count, duplicate_count,
			imported_flight_ids, errors, column_mappings)
		VALUES ($1, $2, $3, $4::import_format, $5::import_status, $6, $7, $8, $9, $10, $11, $12, $13)
	`, importID, userID, session.fileName, string(session.format), string(status),
		len(session.rows), imported, skipped, errored, dups,
		uuidSliceToStringArray(importedIDs), errorsJSON, mappingsJSON,
	)

	// Clean up session
	sessionMu.Lock()
	delete(uploadSessions, req.UploadToken)
	sessionMu.Unlock()

	var errorsPtr *[]struct {
		Field    string  `json:"field"`
		Message  string  `json:"message"`
		RawValue *string `json:"rawValue,omitempty"`
		RowIndex int     `json:"rowIndex"`
	}
	if len(importErrors) > 0 {
		errorsPtr = &importErrors
	}

	var idsPtr *[]openapi_types.UUID
	if len(importedIDs) > 0 {
		idsPtr = &importedIDs
	}

	c.JSON(http.StatusCreated, generated.ImportResult{
		Id:                openapi_types.UUID(importID),
		UserId:            openapi_types.UUID(userID),
		FileName:          session.fileName,
		Format:            session.format,
		Status:            status,
		TotalRows:         len(session.rows),
		ImportedCount:     imported,
		SkippedCount:      skipped,
		ErrorCount:        errored,
		DuplicateCount:    dups,
		ContactsCreated:   &contactsCreated,
		ImportedFlightIds: idsPtr,
		Errors:            errorsPtr,
		CreatedAt:         time.Now(),
	})
}

// ListImports implements GET /imports
func (h *APIHandler) ListImports(c *gin.Context, params generated.ListImportsParams) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	page := 1
	pageSize := 20
	if params.Page != nil && *params.Page > 0 {
		page = *params.Page
	}
	if params.PageSize != nil && *params.PageSize > 0 {
		pageSize = *params.PageSize
	}

	var total int
	scanCount(h.db.QueryRowContext(c.Request.Context(),
		"SELECT COUNT(*) FROM flight_imports WHERE user_id = $1", userID,
	), &total)

	rows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT id, user_id, file_name, import_format, import_status,
			total_rows, imported_count, skipped_count, error_count, duplicate_count,
			imported_flight_ids, errors, created_at
		FROM flight_imports WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, pageSize, (page-1)*pageSize)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to list imports")
		return
	}
	defer rows.Close()

	var results []generated.ImportResult
	for rows.Next() {
		r, err := scanImportResult(rows)
		if err != nil {
			continue
		}
		results = append(results, r)
	}
	if results == nil {
		results = []generated.ImportResult{}
	}

	totalPages := (total + pageSize - 1) / pageSize
	c.JSON(http.StatusOK, generated.PaginatedImports{
		Data: results,
		Pagination: struct {
			Page       int `json:"page"`
			PageSize   int `json:"pageSize"`
			Total      int `json:"total"`
			TotalPages int `json:"totalPages"`
		}{Page: page, PageSize: pageSize, Total: total, TotalPages: totalPages},
	})
}

// GetImport implements GET /imports/{importId}
func (h *APIHandler) GetImport(c *gin.Context, importId generated.ImportId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	row := h.db.QueryRowContext(c.Request.Context(), `
		SELECT id, user_id, file_name, import_format, import_status,
			total_rows, imported_count, skipped_count, error_count, duplicate_count,
			imported_flight_ids, errors, created_at
		FROM flight_imports WHERE id = $1 AND user_id = $2
	`, uuid.UUID(importId), userID)

	var r generated.ImportResult
	var flightIDs string
	var errorsJSON []byte
	var formatStr, statusStr string
	err = row.Scan(
		&r.Id, &r.UserId, &r.FileName, &formatStr, &statusStr,
		&r.TotalRows, &r.ImportedCount, &r.SkippedCount, &r.ErrorCount, &r.DuplicateCount,
		&flightIDs, &errorsJSON, &r.CreatedAt,
	)
	if err != nil {
		h.sendError(c, http.StatusNotFound, "Import not found")
		return
	}
	r.Format = generated.ImportFormat(formatStr)
	r.Status = generated.ImportStatus(statusStr)

	c.JSON(http.StatusOK, r)
}

// --- Helpers ---

type fieldError struct {
	field   string
	message string
}

func mapRowToFlight(row map[string]string, mappings map[string]generated.ImportColumnMapping) (generated.FlightCreate, []fieldError) {
	flight := generated.FlightCreate{
		Landings: 0,
	}
	var errs []fieldError

	// Collect person names from person1-6 fields
	personNames := make(map[string]string) // "person1" → name
	var instructorName string
	var dualReceivedVal float64
	var dualGivenVal float64

	for col, mapping := range mappings {
		val := strings.TrimSpace(row[col])
		if val == "" {
			continue
		}

		switch mapping.TargetField {
		case "date":
			df := "2006-01-02"
			if mapping.DateFormat != nil {
				df = *mapping.DateFormat
			}
			t, err := time.Parse(df, val)
			if err != nil {
				errs = append(errs, fieldError{"date", fmt.Sprintf("Invalid date '%s'", val)})
			} else {
				flight.Date = openapi_types.Date{Time: t}
			}
		case "aircraftReg":
			flight.AircraftReg = strings.ToUpper(val)
		case "aircraftType":
			flight.AircraftType = strings.ToUpper(val)
		case "departureIcao":
			flight.DepartureIcao = strings.ToUpper(val)
		case "arrivalIcao":
			flight.ArrivalIcao = strings.ToUpper(val)
		case "offBlockTime":
			flight.OffBlockTime = normalizeTime(val)
		case "onBlockTime":
			flight.OnBlockTime = normalizeTime(val)
		case "departureTime":
			s := normalizeTime(val)
			flight.DepartureTime = &s
		case "arrivalTime":
			s := normalizeTime(val)
			flight.ArrivalTime = &s
		case "totalTime":
			if mins, err := duration.ParseDuration(val); err == nil {
				flight.TotalTime = &mins
			}
		case "isPic":
			// Auto-calculated from crew; ignore imported value
		case "isDual":
			// Track DualReceived for crew role inference
			if f, err := strconv.ParseFloat(val, 64); err == nil {
				dualReceivedVal = f
			}
		case "nightTime":
			// Auto-calculated from solar data; ignore imported value
		case "ifrTime":
			if mins, err := duration.ParseDuration(val); err == nil {
				flight.IfrTime = &mins
			}
		case "actualInstrumentTime":
			if mins, err := duration.ParseDuration(val); err == nil {
				flight.ActualInstrumentTime = &mins
			}
		case "simulatedInstrumentTime":
			if mins, err := duration.ParseDuration(val); err == nil {
				flight.SimulatedInstrumentTime = &mins
			}
		case "holds":
			n, _ := strconv.Atoi(val)
			flight.Holds = &n
		case "approachesCount":
			n, _ := strconv.Atoi(val)
			flight.ApproachesCount = &n
		case "isIpc":
			b := parseBoolish(val, nil)
			flight.IsIpc = &b
		case "isFlightReview":
			b := parseBoolish(val, nil)
			flight.IsFlightReview = &b
		case "route":
			flight.Route = &val
		case "landingsDay":
			n, _ := strconv.Atoi(val)
			flight.Landings += n
		case "landingsNight":
			n, _ := strconv.Atoi(val)
			flight.Landings += n
		case "remarks":
			flight.Remarks = &val
		case "instructorName":
			instructorName = val
			flight.InstructorName = &val
		case "instructorComments":
			flight.InstructorComments = &val
		case "dualGivenTime":
			if f, err := strconv.ParseFloat(val, 64); err == nil {
				mins := duration.DecimalHoursToMinutes(f)
				flight.DualGivenTime = &mins
				dualGivenVal = f
			}
		case "person1":
			personNames["person1"] = val
		case "person2":
			personNames["person2"] = val
		case "person3":
			personNames["person3"] = val
		case "person4":
			personNames["person4"] = val
		case "person5":
			personNames["person5"] = val
		case "person6":
			personNames["person6"] = val
		}
	}

	// Build crew members from person data
	isTrainingFlight := instructorName != "" || dualReceivedVal > 0
	isInstructorGiving := dualGivenVal > 0

	var crewMembers []generated.FlightCrewMemberInput

	// Determine Person1 role
	if name, ok := personNames["person1"]; ok {
		role := generated.CrewRole("PIC") // default
		if isTrainingFlight {
			// Person1 is the instructor when it's a training flight
			role = "Instructor"
		} else if isInstructorGiving {
			// Logged-in user is instructor giving training; Person1 is student
			role = "Student"
		}
		// Avoid duplicate if InstructorName equals Person1
		if isTrainingFlight && instructorName != "" && !strings.EqualFold(name, instructorName) {
			// InstructorName is someone different from Person1 — add both
			crewMembers = append(crewMembers, generated.FlightCrewMemberInput{
				Name: instructorName,
				Role: "Instructor",
			})
			// Person1 gets a PIC or Passenger role
			crewMembers = append(crewMembers, generated.FlightCrewMemberInput{
				Name: name,
				Role: "PIC",
			})
		} else {
			crewMembers = append(crewMembers, generated.FlightCrewMemberInput{
				Name: name,
				Role: role,
			})
		}
	} else if instructorName != "" {
		// No Person1 but InstructorName is set — add instructor
		crewMembers = append(crewMembers, generated.FlightCrewMemberInput{
			Name: instructorName,
			Role: "Instructor",
		})
	}

	// Person2: Student if training flight, otherwise Passenger
	if name, ok := personNames["person2"]; ok {
		role := generated.CrewRole("Passenger")
		if isTrainingFlight {
			role = "Student"
		}
		crewMembers = append(crewMembers, generated.FlightCrewMemberInput{
			Name: name,
			Role: role,
		})
	}

	// Person3-6: Passenger
	for _, key := range []string{"person3", "person4", "person5", "person6"} {
		if name, ok := personNames[key]; ok {
			crewMembers = append(crewMembers, generated.FlightCrewMemberInput{
				Name: name,
				Role: "Passenger",
			})
		}
	}

	if len(crewMembers) > 0 {
		flight.CrewMembers = &crewMembers
	}

	// Validate required fields
	if flight.Date.IsZero() {
		errs = append(errs, fieldError{"date", "Date is required"})
	}
	if flight.AircraftReg == "" {
		errs = append(errs, fieldError{"aircraftReg", "Aircraft registration is required"})
	}
	if flight.AircraftType == "" {
		// Default to registration if type not mapped
		if flight.AircraftReg != "" {
			flight.AircraftType = flight.AircraftReg
		} else {
			errs = append(errs, fieldError{"aircraftType", "Aircraft type is required"})
		}
	}
	if flight.DepartureIcao == "" {
		errs = append(errs, fieldError{"departureIcao", "Departure ICAO is required"})
	}
	if flight.ArrivalIcao == "" {
		errs = append(errs, fieldError{"arrivalIcao", "Arrival ICAO is required"})
	}

	// ForeFlight: count approaches from Approach1-6 columns
	if flight.ApproachesCount == nil || *flight.ApproachesCount == 0 {
		approachCount := 0
		for _, key := range []string{"Approach1", "Approach2", "Approach3", "Approach4", "Approach5", "Approach6"} {
			if v := strings.TrimSpace(row[key]); v != "" {
				approachCount++
			}
		}
		if approachCount > 0 {
			flight.ApproachesCount = &approachCount
		}
	}

	// ForeFlight: auto-calculate IFR time from actual + simulated instrument if not explicitly set
	if flight.IfrTime == nil || *flight.IfrTime == 0 {
		var ifrTotal int
		if flight.ActualInstrumentTime != nil {
			ifrTotal += *flight.ActualInstrumentTime
		}
		if flight.SimulatedInstrumentTime != nil {
			ifrTotal += *flight.SimulatedInstrumentTime
		}
		if ifrTotal > 0 {
			flight.IfrTime = &ifrTotal
		}
	}

	return flight, errs
}

func findDuplicate(flight generated.FlightCreate, existing []*models.Flight) *openapi_types.UUID {
	for _, e := range existing {
		if e.Date.Format("2006-01-02") != flight.Date.String() {
			continue
		}
		if !strings.EqualFold(e.AircraftReg, flight.AircraftReg) {
			continue
		}
		depMatch := (e.DepartureICAO != nil && strings.EqualFold(*e.DepartureICAO, flight.DepartureIcao)) ||
			(e.DepartureICAO == nil && flight.DepartureIcao == "")
		arrMatch := (e.ArrivalICAO != nil && strings.EqualFold(*e.ArrivalICAO, flight.ArrivalIcao)) ||
			(e.ArrivalICAO == nil && flight.ArrivalIcao == "")
		if !depMatch || !arrMatch {
			continue
		}
		// Total time within ±6 minutes tolerance
		importTotal := 0
		if flight.TotalTime != nil {
			importTotal = *flight.TotalTime
		}
		diff := e.TotalTime - importTotal
		if diff < 0 {
			diff = -diff
		}
		if diff <= 6 {
			id := openapi_types.UUID(e.ID)
			return &id
		}
	}
	return nil
}

func parseCSV(data []byte) ([]string, []map[string]string, []map[string]string, error) {
	// ForeFlight exports have metadata preamble sections:
	// "ForeFlight Logbook Import", "Aircraft Table", "Flights Table"
	// We need to find both sections and parse them.
	content := string(data)
	lines := strings.Split(content, "\n")

	// Find section markers
	aircraftTableIdx := -1
	flightsTableIdx := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Aircraft Table") {
			aircraftTableIdx = i
		}
		if strings.HasPrefix(trimmed, "Flights Table") {
			flightsTableIdx = i
		}
	}

	// Parse Aircraft Table if present
	var aircraftRows []map[string]string
	if aircraftTableIdx >= 0 {
		endIdx := flightsTableIdx
		if endIdx < 0 {
			endIdx = len(lines)
		}
		if aircraftTableIdx+1 < endIdx {
			acData := []byte(strings.Join(lines[aircraftTableIdx+1:endIdx], "\n"))
			_, acRows, _ := parseSectionCSV(acData)
			aircraftRows = acRows
		}
	}

	var csvData []byte
	if flightsTableIdx >= 0 && flightsTableIdx+1 < len(lines) {
		csvData = []byte(strings.Join(lines[flightsTableIdx+1:], "\n"))
	} else {
		csvData = data
	}

	headers, rows, err := parseSectionCSV(csvData)
	if err != nil {
		return nil, nil, nil, err
	}

	return headers, rows, aircraftRows, nil
}

func parseSectionCSV(csvData []byte) ([]string, []map[string]string, error) {

	// Detect delimiter by trying each
	delimiters := []rune{';', ',', '\t'}
	var records [][]string
	var err error

	for _, delim := range delimiters {
		reader := csv.NewReader(bytes.NewReader(csvData))
		reader.LazyQuotes = true
		reader.Comma = delim
		reader.FieldsPerRecord = -1 // allow variable field counts
		records, err = reader.ReadAll()
		if err == nil && len(records) >= 2 && len(records[0]) >= 3 {
			break
		}
	}

	if err != nil || len(records) < 2 {
		return nil, nil, fmt.Errorf("could not parse as CSV (tried semicolon, comma, tab)")
	}

	// Find the header row (first row with multiple non-empty columns)
	headerIdx := 0
	for i, record := range records {
		nonEmpty := 0
		for _, cell := range record {
			if strings.TrimSpace(cell) != "" {
				nonEmpty++
			}
		}
		if nonEmpty >= 3 {
			headerIdx = i
			break
		}
	}

	headers := records[headerIdx]
	// Clean header names
	for i := range headers {
		headers[i] = strings.TrimSpace(headers[i])
	}

	var rows []map[string]string
	for _, record := range records[headerIdx+1:] {
		// Skip empty rows
		nonEmpty := 0
		for _, cell := range record {
			if strings.TrimSpace(cell) != "" {
				nonEmpty++
			}
		}
		if nonEmpty < 2 {
			continue
		}

		row := make(map[string]string)
		for j, h := range headers {
			if j < len(record) && h != "" {
				row[h] = strings.TrimSpace(record[j])
			}
		}
		rows = append(rows, row)
	}

	if len(rows) == 0 {
		return nil, nil, fmt.Errorf("no data rows found")
	}

	return headers, rows, nil
}

func normalizeTime(val string) string {
	// Handle HH:MM, HH:MM:SS, H:MM
	val = strings.TrimSpace(val)
	if len(val) == 4 && val[1] == ':' {
		val = "0" + val // H:MM → 0H:MM
	}
	if len(val) == 5 {
		val += ":00" // HH:MM → HH:MM:00
	}
	return val
}

func parseBoolish(val string, trueValue *string) bool {
	if trueValue != nil {
		return strings.EqualFold(val, *trueValue)
	}
	lower := strings.ToLower(val)
	if lower == "true" || lower == "yes" || lower == "1" || lower == "x" || lower == "y" {
		return true
	}
	// If it's a float > 0, treat as true (ForeFlight PIC column)
	if f, err := strconv.ParseFloat(val, 64); err == nil && f > 0 {
		return true
	}
	return false
}

func uuidSliceToStringArray(ids []openapi_types.UUID) string {
	if len(ids) == 0 {
		return "{}"
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = uuid.UUID(id).String()
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func scanImportResult(rows *sql.Rows) (generated.ImportResult, error) {
	var r generated.ImportResult
	var flightIDs string
	var errorsJSON []byte
	var formatStr, statusStr string
	err := rows.Scan(
		&r.Id, &r.UserId, &r.FileName, &formatStr, &statusStr,
		&r.TotalRows, &r.ImportedCount, &r.SkippedCount, &r.ErrorCount, &r.DuplicateCount,
		&flightIDs, &errorsJSON, &r.CreatedAt,
	)
	if err != nil {
		return r, err
	}
	r.Format = generated.ImportFormat(formatStr)
	r.Status = generated.ImportStatus(statusStr)
	return r, nil
}
