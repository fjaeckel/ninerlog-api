package cloudbackup

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
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
	CustomReports       []CustomReport       `json:"customReports"`
	// AircraftReminders name their aircraft by id and registration in this
	// backup.
	AircraftReminders []*models.AircraftReminder `json:"aircraftReminders"`
	// NotificationPreferences and FlightBaseline are single-row settings and
	// are omitted when the user has none.
	NotificationPreferences *NotificationPreferences `json:"notificationPreferences,omitempty"`
	FlightBaseline          *FlightBaseline          `json:"flightBaseline,omitempty"`
	// PilotProfile is a single-row setting, omitted when the user has none.
	PilotProfile *PilotProfile `json:"pilotProfile,omitempty"`
	// FlightFiles name their flight by its id in this backup.
	FlightFiles []FlightFile `json:"flightFiles"`
}

// LicenseWithRatings pairs a licence with its class ratings and privileges so
// a restore can wire them to freshly minted licence IDs.
type LicenseWithRatings struct {
	License      *models.License       `json:"license"`
	ClassRatings []*models.ClassRating `json:"classRatings"`
	// Privileges is omitted when the licence has none.
	Privileges []*models.LicencePrivilege `json:"privileges,omitempty"`
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

// CustomReport is the portable half of a saved custom report, in display
// order. A licence-scoped report names the licence by its id in this backup.
type CustomReport struct {
	Name       string                        `json:"name"`
	Definition models.CustomReportDefinition `json:"definition"`
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
	PICUSMinutes        int       `json:"picusMinutes"`
	SPICMinutes         int       `json:"spicMinutes"`
	ExaminerMinutes     int       `json:"examinerMinutes"`
	ReliefMinutes       int       `json:"reliefMinutes"`
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
		PICUSMinutes:        b.PICUSMinutes,
		SPICMinutes:         b.SPICMinutes,
		ExaminerMinutes:     b.ExaminerMinutes,
		ReliefMinutes:       b.ReliefMinutes,
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
		PICUSMinutes:        b.PICUSMinutes,
		SPICMinutes:         b.SPICMinutes,
		ExaminerMinutes:     b.ExaminerMinutes,
		ReliefMinutes:       b.ReliefMinutes,
		LandingsDay:         b.LandingsDay,
		LandingsNight:       b.LandingsNight,
		Notes:               b.Notes,
	}
}

// PilotProfile is the portable half of a user's pilot profile: the mode and the
// per-discipline intent and acknowledgement. Derived evidence is not carried.
type PilotProfile struct {
	Mode        models.PilotProfileMode                        `json:"mode"`
	Disciplines map[models.Discipline]models.DisciplineSetting `json:"disciplines"`
}

// NewPilotProfile projects a stored pilot profile onto its portable half.
func NewPilotProfile(p *models.PilotProfile) *PilotProfile {
	if p == nil {
		return nil
	}
	out := &PilotProfile{Mode: p.Mode, Disciplines: map[models.Discipline]models.DisciplineSetting{}}
	for d, s := range p.Disciplines {
		out.Disciplines[d] = s
	}
	return out
}

// ToModel rebuilds a storable pilot profile owned by the given user, dropping
// disciplines this version does not know.
func (p PilotProfile) ToModel(userID uuid.UUID) *models.PilotProfile {
	out := &models.PilotProfile{UserID: userID, Mode: p.Mode, Disciplines: map[models.Discipline]models.DisciplineSetting{}}
	for d, s := range p.Disciplines {
		if d.IsValid() {
			out.Disciplines[d] = s
		}
	}
	return out
}

// FlightFileEncodingGzipBase64 is the only content encoding FlightFile uses.
const FlightFileEncodingGzipBase64 = "gzip+base64"

// ErrInvalidFlightFileContent is returned by FlightFile.Decode for content
// that does not decode, exceeds models.MaxFlightFileBytes or does not match
// its SHA-256.
var ErrInvalidFlightFileContent = errors.New("invalid flight file content")

// FlightFile is the portable half of a flight recorder file: its flight's id
// in this backup, its metadata and its content gzipped and base64-encoded.
type FlightFile struct {
	FlightID        uuid.UUID `json:"flightId"`
	Kind            string    `json:"kind"`
	Filename        string    `json:"filename"`
	SizeBytes       int       `json:"sizeBytes"`
	SHA256          string    `json:"sha256"`
	ContentEncoding string    `json:"contentEncoding"`
	Content         string    `json:"content"`
}

// NewFlightFile projects a stored file, content included, onto its portable
// half.
func NewFlightFile(f *models.FlightFile) (FlightFile, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(f.Content); err != nil {
		return FlightFile{}, fmt.Errorf("gzip flight file: %w", err)
	}
	if err := gz.Close(); err != nil {
		return FlightFile{}, fmt.Errorf("gzip flight file: %w", err)
	}
	return FlightFile{
		FlightID:        f.FlightID,
		Kind:            string(f.Kind),
		Filename:        f.Filename,
		SizeBytes:       f.SizeBytes,
		SHA256:          f.SHA256,
		ContentEncoding: FlightFileEncodingGzipBase64,
		Content:         base64.StdEncoding.EncodeToString(buf.Bytes()),
	}, nil
}

// Decode returns the file's content. It accepts plain base64 when
// ContentEncoding is "base64", reads at most models.MaxFlightFileBytes, and
// checks SHA256 when set.
func (f FlightFile) Decode() ([]byte, error) {
	if len(f.Content) > base64.StdEncoding.EncodedLen(models.MaxFlightFileBytes) {
		return nil, ErrInvalidFlightFileContent
	}
	raw, err := base64.StdEncoding.DecodeString(f.Content)
	if err != nil {
		return nil, ErrInvalidFlightFileContent
	}
	var data []byte
	switch f.ContentEncoding {
	case FlightFileEncodingGzipBase64:
		gz, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, ErrInvalidFlightFileContent
		}
		data, err = io.ReadAll(io.LimitReader(gz, models.MaxFlightFileBytes+1))
		if err != nil {
			return nil, ErrInvalidFlightFileContent
		}
	case "base64":
		data = raw
	default:
		return nil, ErrInvalidFlightFileContent
	}
	if len(data) == 0 || len(data) > models.MaxFlightFileBytes {
		return nil, ErrInvalidFlightFileContent
	}
	if f.SHA256 != "" {
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != f.SHA256 {
			return nil, ErrInvalidFlightFileContent
		}
	}
	return data, nil
}
