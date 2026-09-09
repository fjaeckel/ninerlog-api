package currency

// Message keys for client-side localisation. Every user-facing string the
// currency engine produces is emitted as one of these stable keys plus the
// params the key needs.
//
// Keys are plain strings, not an enum, so that adding one is not a breaking
// change for generated clients. The catalogue is the cross-repo contract in
// docs/CURRENCY_MESSAGES.md.
//
// Params that are already fields on the enclosing object (classType,
// regulatoryAuthority, licenseType, expiryDate, current/required/unit) are
// never repeated in MessageParams; clients compose from the fields they have.

// Rating currency keys (ClassRatingCurrency.MessageKey).
const (
	MsgRatingNoExpiryDate       = "rating.no_expiry_date"
	MsgRatingNoExpiryDateManual = "rating.no_expiry_date_manual"
	MsgRatingEvaluationFailed   = "rating.evaluation_failed"
	MsgRatingExpired            = "rating.expired"
	MsgRatingExpiring           = "rating.expiring"
	MsgRatingValidUntil         = "rating.valid_until"
	MsgRatingWindowNotOpen      = "rating.window_not_open"

	MsgRatingRevalidationNotMet          = "rating.revalidation_not_met"
	MsgRatingRevalidationNotMetProfCheck = "rating.revalidation_not_met_prof_check"
	MsgRatingRevalidationExpiringMet     = "rating.revalidation_expiring_met"
	MsgRatingRevalidationCurrent         = "rating.revalidation_current"

	MsgRatingRecencyNotMet  = "rating.recency_not_met"
	MsgRatingRecencyCurrent = "rating.recency_current"

	MsgRatingIRHoursAndCheckNotMet = "rating.ir_hours_and_check_not_met"
	MsgRatingIRHoursNotMet         = "rating.ir_hours_not_met"
	MsgRatingIRCheckNotMet         = "rating.ir_check_not_met"
	MsgRatingIRCurrent             = "rating.ir_current"
	MsgRatingIRLapsedSafetyPilot   = "rating.ir_lapsed_safety_pilot"
	MsgRatingIRExpiredIPC          = "rating.ir_expired_ipc"
	MsgRatingIRNotApplicable       = "rating.ir_not_applicable"

	MsgRatingPaxNotCurrent        = "rating.pax_not_current"
	MsgRatingPaxDayCurrentNightNo = "rating.pax_day_current_night_not"
	MsgRatingPaxCurrentDayNight   = "rating.pax_current_day_night"

	MsgRatingGliderNotCurrent = "rating.glider_not_current"
	MsgRatingGliderCurrent    = "rating.glider_current"
)

// Passenger currency keys (PassengerCurrency.MessageKey).
const (
	MsgPaxEvaluationFailed        = "pax.evaluation_failed"
	MsgPaxNotCurrent              = "pax.not_current"
	MsgPaxCurrentDayNoNight       = "pax.current_day_no_night_privilege"
	MsgPaxCurrentDayNightIRWaived = "pax.current_day_night_ir_waived"
	MsgPaxCurrentDayNight         = "pax.current_day_night"
	MsgPaxDayCurrentNightNot      = "pax.day_current_night_not"
	MsgPaxCurrentPrivilegeSeparat = "pax.current_day_privilege_separate"
)

// Flight review keys (FlightReviewStatus.MessageKey).
const (
	MsgFlightReviewEvaluationFailed = "flight_review.evaluation_failed"
	MsgFlightReviewNoneOnRecord     = "flight_review.none_on_record"
	MsgFlightReviewExpired          = "flight_review.expired"
	MsgFlightReviewExpiring         = "flight_review.expiring"
	MsgFlightReviewCurrent          = "flight_review.current"
)

// Requirement message keys (Requirement.MessageKey). The common case is
// progress, which the client renders from current/required/unit.
const (
	MsgRequirementProgress           = "requirement.progress"
	MsgRequirementProfCheckCompleted = "requirement.prof_check_completed"
	MsgRequirementProfCheckMissing   = "requirement.prof_check_missing"
)

// Requirement name keys (Requirement.NameKey).
const (
	ReqKeyTotalTime          = "requirement.total_time"
	ReqKeyPICTime            = "requirement.pic_time"
	ReqKeyIFRTime            = "requirement.ifr_time"
	ReqKeyLandings           = "requirement.landings"
	ReqKeyDayLandings        = "requirement.day_landings"
	ReqKeyNightLandings      = "requirement.night_landings"
	ReqKeyRefresherTraining  = "requirement.refresher_training"
	ReqKeyTrainingFlight     = "requirement.training_flight"
	ReqKeyProficiencyCheck   = "requirement.proficiency_check"
	ReqKeyApproaches         = "requirement.approaches"
	ReqKeyHolds              = "requirement.holds"
	ReqKeyRouteSectors       = "requirement.route_sectors"
	ReqKeyLaunches           = "requirement.launches"
	ReqKeyLaunchesAndLanding = "requirement.launches_and_landings"
)

// Launch method key (LaunchMethodCurrency.MessageKey).
const MsgLaunchMethodProgress = "launch_method.progress"

// MessageParams carries the variable parts of a message that are not already
// fields on the enclosing object. Every field is optional; a key documents
// which of them it requires.
type MessageParams struct {
	// Days until expiry, for expiry countdown keys.
	Days *int `json:"days,omitempty"`
	// Needed is the outstanding count for a shortfall key (landings, launches).
	Needed *int `json:"needed,omitempty"`
	// Date is an ISO date the message refers to that is not the object's own
	// expiry — a proficiency check, a flight review completion, a window opening.
	Date *string `json:"date,omitempty"`
}

// msgDays builds params for a countdown key.
func msgDays(d int) *MessageParams { return &MessageParams{Days: &d} }

// msgNeeded builds params for a shortfall key.
func msgNeeded(n int) *MessageParams { return &MessageParams{Needed: &n} }

// msgDate builds params for a key that names a date.
func msgDate(d string) *MessageParams { return &MessageParams{Date: &d} }

// msgDaysDate builds params for a key that counts down to a named date.
func msgDaysDate(d int, date string) *MessageParams {
	return &MessageParams{Days: &d, Date: &date}
}

// setMsg records the localisation key and its params.
func (r *ClassRatingCurrency) setMsg(key string, params *MessageParams) {
	r.MessageKey = key
	r.MessageParams = params
}

// setMsg records the localisation key and its params for a passenger currency.
func (p *PassengerCurrency) setMsg(key string, params *MessageParams) {
	p.MessageKey = key
	p.MessageParams = params
}

// setMsg records the localisation key and its params for a flight review.
func (f *FlightReviewStatus) setMsg(key string, params *MessageParams) {
	f.MessageKey = key
	f.MessageParams = params
}
