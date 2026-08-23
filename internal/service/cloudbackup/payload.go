package cloudbackup

import (
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// Payload is the wire layout of one backup, shared by GET /exports/json,
// POST /imports/json and every cloud backup run. Field order matches the
// original ExportDataJSON output; new sections are appended.
//
// Every section holds data a user owns and would expect to survive moving to
// another server. Sections added here must be restored by ImportDataJSON in
// the same change.
type Payload struct {
	ExportedAt          string               `json:"exportedAt"`
	Version             string               `json:"version"`
	Format              string               `json:"format"`
	Flights             []*models.Flight     `json:"flights"`
	Aircraft            []*models.Aircraft   `json:"aircraft"`
	Licenses            []LicenseWithRatings `json:"licenses"`
	Credentials         []*models.Credential `json:"credentials"`
	Contacts            []*models.Contact    `json:"contacts"`
	CustomCurrencyRules []CustomCurrencyRule `json:"customCurrencyRules"`
	// NotificationPreferences and FlightBaseline are single-row settings and
	// are omitted when the user has none.
	NotificationPreferences *NotificationPreferences `json:"notificationPreferences,omitempty"`
	FlightBaseline          *FlightBaseline          `json:"flightBaseline,omitempty"`
}

// LicenseWithRatings pairs a licence with its class ratings so a restore can
// wire ratings to freshly minted licence IDs.
type LicenseWithRatings struct {
	License      *models.License       `json:"license"`
	ClassRatings []*models.ClassRating `json:"classRatings"`
}

// CustomCurrencyRule is the portable half of a user-authored currency rule.
// Sharing state (isShared, shareToken, importedFrom) is deliberately excluded:
// a share token is unique across the installation and belongs to the rule it
// was minted for, not to a copy restored elsewhere.
type CustomCurrencyRule struct {
	Name        string                        `json:"name"`
	Description *string                       `json:"description,omitempty"`
	Emoji       *string                       `json:"emoji,omitempty"`
	Definition  models.CustomCurrencyRuleBody `json:"definition"`
	Enabled     bool                          `json:"enabled"`
	Notify      bool                          `json:"notify"`
}

// NotificationPreferences is the portable half of a user's notification
// settings; identifiers and timestamps are reassigned on restore.
type NotificationPreferences struct {
	EmailEnabled      bool     `json:"emailEnabled"`
	EnabledCategories []string `json:"enabledCategories"`
	WarningDays       []int64  `json:"warningDays"`
	CheckHour         int      `json:"checkHour"`
}

// FlightBaseline is the portable half of a user's carried-forward hours
// snapshot. models.FlightBaseline carries no JSON tags, so the wire shape is
// declared here.
type FlightBaseline struct {
	BaselineDate        time.Time `json:"baselineDate"`
	TotalFlights        int       `json:"totalFlights"`
	TotalMinutes        int       `json:"totalMinutes"`
	PICMinutes          int       `json:"picMinutes"`
	SICMinutes          int       `json:"sicMinutes"`
	DualMinutes         int       `json:"dualMinutes"`
	DualGivenMinutes    int       `json:"dualGivenMinutes"`
	MultiPilotMinutes   int       `json:"multiPilotMinutes"`
	NightMinutes        int       `json:"nightMinutes"`
	IFRMinutes          int       `json:"ifrMinutes"`
	SoloMinutes         int       `json:"soloMinutes"`
	CrossCountryMinutes int       `json:"crossCountryMinutes"`
	LandingsDay         int       `json:"landingsDay"`
	LandingsNight       int       `json:"landingsNight"`
	Notes               *string   `json:"notes,omitempty"`
}

// NewCustomCurrencyRule projects a stored rule onto its portable half.
func NewCustomCurrencyRule(r *models.CustomCurrencyRule) CustomCurrencyRule {
	return CustomCurrencyRule{
		Name:        r.Name,
		Description: r.Description,
		Emoji:       r.Emoji,
		Definition:  r.Definition,
		Enabled:     r.Enabled,
		Notify:      r.Notify,
	}
}

// NewNotificationPreferences projects stored preferences onto their portable
// half.
func NewNotificationPreferences(p *models.NotificationPreferences) *NotificationPreferences {
	if p == nil {
		return nil
	}
	return &NotificationPreferences{
		EmailEnabled:      p.EmailEnabled,
		EnabledCategories: []string(p.EnabledCategories),
		WarningDays:       []int64(p.WarningDays),
		CheckHour:         p.CheckHour,
	}
}

// NewFlightBaseline projects a stored baseline onto its portable half.
func NewFlightBaseline(b *models.FlightBaseline) *FlightBaseline {
	if b == nil {
		return nil
	}
	return &FlightBaseline{
		BaselineDate:        b.BaselineDate,
		TotalFlights:        b.TotalFlights,
		TotalMinutes:        b.TotalMinutes,
		PICMinutes:          b.PICMinutes,
		SICMinutes:          b.SICMinutes,
		DualMinutes:         b.DualMinutes,
		DualGivenMinutes:    b.DualGivenMinutes,
		MultiPilotMinutes:   b.MultiPilotMinutes,
		NightMinutes:        b.NightMinutes,
		IFRMinutes:          b.IFRMinutes,
		SoloMinutes:         b.SoloMinutes,
		CrossCountryMinutes: b.CrossCountryMinutes,
		LandingsDay:         b.LandingsDay,
		LandingsNight:       b.LandingsNight,
		Notes:               b.Notes,
	}
}

// ToModel rebuilds a storable baseline owned by the given user.
func (b FlightBaseline) ToModel(userID uuid.UUID) *models.FlightBaseline {
	return &models.FlightBaseline{
		UserID:              userID,
		BaselineDate:        b.BaselineDate,
		TotalFlights:        b.TotalFlights,
		TotalMinutes:        b.TotalMinutes,
		PICMinutes:          b.PICMinutes,
		SICMinutes:          b.SICMinutes,
		DualMinutes:         b.DualMinutes,
		DualGivenMinutes:    b.DualGivenMinutes,
		MultiPilotMinutes:   b.MultiPilotMinutes,
		NightMinutes:        b.NightMinutes,
		IFRMinutes:          b.IFRMinutes,
		SoloMinutes:         b.SoloMinutes,
		CrossCountryMinutes: b.CrossCountryMinutes,
		LandingsDay:         b.LandingsDay,
		LandingsNight:       b.LandingsNight,
		Notes:               b.Notes,
	}
}
