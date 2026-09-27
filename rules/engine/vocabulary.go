package engine

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Vocabulary is vocabulary.yaml.
type Vocabulary struct {
	Version              int                       `yaml:"version"`
	CatalogueAuthorities []string                  `yaml:"catalogue_authorities"`
	SourceKinds          map[string]string         `yaml:"source_kinds"`
	SupportLevels        []string                  `yaml:"support_levels"`
	RecordAuthorities    []string                  `yaml:"record_authorities"`
	Categories           []string                  `yaml:"categories"`
	Classes              map[string]ClassDef       `yaml:"classes"`
	ULKinds              []string                  `yaml:"ul_kinds"`
	LaunchMethods        []string                  `yaml:"launch_methods"`
	FSTDTypes            []string                  `yaml:"fstd_types"`
	TowKinds             []string                  `yaml:"tow_kinds"`
	LicenceKinds         map[string]LicenceKindDef `yaml:"licence_kinds"`
	PrivilegeKinds       []string                  `yaml:"privilege_kinds"`
	CredentialTypes      []string                  `yaml:"credential_types"`
	FlightFlags          []string                  `yaml:"flight_flags"`
	EventKinds           map[string]EventKindDef   `yaml:"event_kinds"`
	RecordFields         map[string]RecordFieldDef `yaml:"record_fields"`
	MinuteFields         []string                  `yaml:"minute_fields"`
	Subjects             map[string]SubjectDef     `yaml:"subjects"`
	Statuses             map[string]string         `yaml:"statuses"`
	Units                []string                  `yaml:"units"`
	Metrics              map[string]MetricDef      `yaml:"metrics"`
	Filters              map[string]FilterDef      `yaml:"filters"`
	Roles                []string                  `yaml:"roles"`
	Windows              map[string]WindowDef      `yaml:"windows"`
	Combinators          map[string]string         `yaml:"combinators"`
	StageConditions      map[string]string         `yaml:"stage_conditions"`
	MissingInputs        []string                  `yaml:"missing_inputs"`
	RuleEvents           map[string]string         `yaml:"rule_events"`
	ParamSources         map[string]string         `yaml:"param_sources"`
	Validity             ValidityDef               `yaml:"validity"`
	Hatches              map[string]string         `yaml:"hatches"`
	aliasIndex           map[string]string
}

// ClassDef describes an aircraft class.
type ClassDef struct {
	Category string `yaml:"category"`
}

// LicenceKindDef lists the licence type spellings of a kind.
type LicenceKindDef struct {
	Aliases []string `yaml:"aliases"`
}

// EventKindDef describes an event kind.
type EventKindDef struct {
	FromFlag string `yaml:"from_flag"`
}

// RecordFieldDef describes a record input field.
type RecordFieldDef struct {
	Type     string `yaml:"type"`
	Optional bool   `yaml:"optional"`
}

// SubjectDef describes a subject kind.
type SubjectDef struct {
	Description string `yaml:"description"`
	Expiry      bool   `yaml:"expiry"`
}

// MetricDef describes a metric.
type MetricDef struct {
	Source      string   `yaml:"source"`
	Aggregate   string   `yaml:"aggregate"`
	Reads       []string `yaml:"reads"`
	Optional    bool     `yaml:"optional"`
	Units       []string `yaml:"units"`
	Description string   `yaml:"description"`
}

// FilterDef describes a filter.
type FilterDef struct {
	AppliesTo   []string `yaml:"applies_to"`
	Reads       []string `yaml:"reads"`
	Values      any      `yaml:"values"`
	SubjectRef  bool     `yaml:"subject_ref"`
	Absent      string   `yaml:"absent"`
	Description string   `yaml:"description"`
}

// WindowDef describes a window kind.
type WindowDef struct {
	Param       any    `yaml:"param"`
	Moving      bool   `yaml:"moving"`
	Anchor      string `yaml:"anchor"`
	Description string `yaml:"description"`
}

// ValidityDef lists the derived-validity vocabulary.
type ValidityDef struct {
	Anchors          []string `yaml:"anchors"`
	PeriodConditions []string `yaml:"period_conditions"`
	PeriodFields     []string `yaml:"period_fields"`
}

// LoadVocabulary reads vocabulary.yaml.
func LoadVocabulary(path string) (*Vocabulary, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v Vocabulary
	if err := yaml.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	v.index()
	return &v, nil
}

func (v *Vocabulary) index() {
	v.aliasIndex = map[string]string{}
	for kind, def := range v.LicenceKinds {
		for _, a := range def.Aliases {
			v.aliasIndex[normLicenceType(a)] = kind
		}
	}
}

func normLicenceType(s string) string {
	return strings.ToUpper(strings.Join(strings.Fields(s), ""))
}

// ClassifyLicence returns the licence kind: explicit kind, then DULV/DAeC, then aliases.
func (v *Vocabulary) ClassifyLicence(typ, authority, kind string) string {
	if kind != "" {
		return strings.ToUpper(strings.TrimSpace(kind))
	}
	switch normAuthority(authority) {
	case "DULV", "DAEC":
		return "UL"
	}
	return v.aliasIndex[normLicenceType(typ)]
}

// Category returns the category of a class.
func (v *Vocabulary) Category(class string) string {
	if c, ok := v.Classes[class]; ok {
		return c.Category
	}
	return ""
}

// HasUnit reports whether u is a declared unit.
func (v *Vocabulary) HasUnit(u string) bool { return slices.Contains(v.Units, u) }
