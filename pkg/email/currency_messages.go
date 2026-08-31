package email

import (
	"fmt"
	"strings"
)

// CurrencyMessageParams carries the params a currency message key interpolates.
// Mirrors currency.MessageParams; see ninerlog-api docs/CURRENCY_MESSAGES.md.
type CurrencyMessageParams struct {
	Days   *int
	Needed *int
	Date   *string
}

// currencyMessagesEN renders the currency message catalogue in English.
// Sentences omit the rating identity — the surrounding template already names
// the licence and class.
var currencyMessagesEN = map[string]string{
	"rating.no_expiry_date":                  "no expiry date is recorded for this rating",
	"rating.no_expiry_date_manual":           "no expiry date is recorded; this rating is tracked manually",
	"rating.evaluation_failed":               "the flight experience could not be evaluated",
	"rating.expired":                         "the rating has expired",
	"rating.expiring":                        "the rating expires in %[1]d days",
	"rating.valid_until":                     "the rating is valid",
	"rating.window_not_open":                 "recently revalidated; the experience window opens on %[3]s",
	"rating.revalidation_not_met":            "the revalidation requirements are not fully met",
	"rating.revalidation_not_met_prof_check": "the revalidation requirements are not fully met; a proficiency check may apply",
	"rating.revalidation_expiring_met":       "the requirements are met, and the rating expires in %[1]d days",
	"rating.revalidation_current":            "all revalidation requirements are met",
	"rating.recency_not_met":                 "the recency requirements are not fully met",
	"rating.recency_current":                 "the recency requirements are met",
	"rating.ir_hours_and_check_not_met":      "neither the IFR hours nor the proficiency check are complete",
	"rating.ir_hours_not_met":                "the IFR hour requirement is not met",
	"rating.ir_check_not_met":                "the annual proficiency check is not complete",
	"rating.ir_current":                      "the instrument currency requirements are met",
	"rating.ir_lapsed_safety_pilot":          "instrument currency has lapsed past 6 months; it can be regained by practice with a safety pilot (§61.57(c))",
	"rating.ir_expired_ipc":                  "instrument currency expired more than 12 months ago; an IPC is required (§61.57(d))",
	"rating.ir_not_applicable":               "instrument privileges are not available on this certificate (§61.315)",
	"rating.pax_not_current":                 "%[2]d more landings are needed to carry passengers",
	"rating.pax_day_current_night_not":       "day currency is met; %[2]d more night landings are needed",
	"rating.pax_current_day_night":           "day and night passenger currency are met",
	"rating.glider_not_current":              "%[2]d more launches are needed",
	"rating.glider_current":                  "the launch requirements are met",
	"flight_review.evaluation_failed":        "your flight review status could not be determined.",
	"flight_review.none_on_record":           "No flight review is on record — one is required every 24 calendar months (14 CFR §61.56).",
	"flight_review.expired":                  "Your flight review has expired — it was last completed on %[3]s.",
	"flight_review.expiring":                 "Your flight review expires in %[1]d days — it was last completed on %[3]s.",
	"flight_review.current":                  "Your flight review is current — last completed on %[3]s.",
}

// currencyMessagesDE renders the currency message catalogue in German.
var currencyMessagesDE = map[string]string{
	"rating.no_expiry_date":                  "für diese Berechtigung ist kein Ablaufdatum hinterlegt",
	"rating.no_expiry_date_manual":           "kein Ablaufdatum hinterlegt; diese Berechtigung wird manuell verwaltet",
	"rating.evaluation_failed":               "die Flugerfahrung konnte nicht ausgewertet werden",
	"rating.expired":                         "die Berechtigung ist abgelaufen",
	"rating.expiring":                        "die Berechtigung läuft in %[1]d Tagen ab",
	"rating.valid_until":                     "die Berechtigung ist gültig",
	"rating.window_not_open":                 "kürzlich verlängert; der Erfahrungszeitraum beginnt am %[3]s",
	"rating.revalidation_not_met":            "die Verlängerungsanforderungen sind nicht vollständig erfüllt",
	"rating.revalidation_not_met_prof_check": "die Verlängerungsanforderungen sind nicht vollständig erfüllt; eine Befähigungsüberprüfung kann erforderlich sein",
	"rating.revalidation_expiring_met":       "die Anforderungen sind erfüllt, die Berechtigung läuft in %[1]d Tagen ab",
	"rating.revalidation_current":            "alle Verlängerungsanforderungen sind erfüllt",
	"rating.recency_not_met":                 "die Flugerfahrungsanforderungen sind nicht vollständig erfüllt",
	"rating.recency_current":                 "die Flugerfahrungsanforderungen sind erfüllt",
	"rating.ir_hours_and_check_not_met":      "weder die IFR-Stunden noch die Befähigungsüberprüfung sind vollständig",
	"rating.ir_hours_not_met":                "die IFR-Stundenanforderung ist nicht erfüllt",
	"rating.ir_check_not_met":                "die jährliche Befähigungsüberprüfung fehlt",
	"rating.ir_current":                      "die Instrumentenflug-Anforderungen sind erfüllt",
	"rating.ir_lapsed_safety_pilot":          "die Instrumentenflugerfahrung ist seit über 6 Monaten abgelaufen; sie kann durch Übung mit einem Sicherheitspiloten wiedererlangt werden (§61.57(c))",
	"rating.ir_expired_ipc":                  "die Instrumentenflugerfahrung ist seit über 12 Monaten abgelaufen; ein IPC ist erforderlich (§61.57(d))",
	"rating.ir_not_applicable":               "auf dieser Lizenz bestehen keine Instrumentenflugrechte (§61.315)",
	"rating.pax_not_current":                 "es fehlen noch %[2]d Landungen für Passagierflüge",
	"rating.pax_day_current_night_not":       "die Tagesanforderung ist erfüllt; es fehlen noch %[2]d Nachtlandungen",
	"rating.pax_current_day_night":           "die Passagieranforderungen für Tag und Nacht sind erfüllt",
	"rating.glider_not_current":              "es fehlen noch %[2]d Starts",
	"rating.glider_current":                  "die Startanforderungen sind erfüllt",
	"flight_review.evaluation_failed":        "Ihr Flight-Review-Status konnte nicht ermittelt werden.",
	"flight_review.none_on_record":           "Es ist kein Flight Review hinterlegt — erforderlich alle 24 Kalendermonate (14 CFR §61.56).",
	"flight_review.expired":                  "Ihr Flight Review ist abgelaufen — zuletzt absolviert am %[3]s.",
	"flight_review.expiring":                 "Ihr Flight Review läuft in %[1]d Tagen ab — zuletzt absolviert am %[3]s.",
	"flight_review.current":                  "Ihr Flight Review ist aktuell — zuletzt absolviert am %[3]s.",
}

// renderCurrencyMessage interpolates a catalogue entry. An unrecognised key
// renders as the key itself so a missing translation is visible rather than blank.
func renderCurrencyMessage(catalogue map[string]string, key string, p CurrencyMessageParams) string {
	format, ok := catalogue[key]
	if !ok {
		return key
	}
	if !strings.Contains(format, "%[") {
		return format
	}
	days, needed, date := 0, 0, ""
	if p.Days != nil {
		days = *p.Days
	}
	if p.Needed != nil {
		needed = *p.Needed
	}
	if p.Date != nil {
		date = *p.Date
	}
	return fmt.Sprintf(format, days, needed, date)
}
