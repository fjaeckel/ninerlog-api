package engine

// Evaluation is the engine's result for one (rule, subject) (DESIGN.md section 4).
type Evaluation struct {
	RuleID             string              `yaml:"ruleId" json:"ruleId"`
	Subject            Subject             `yaml:"subject" json:"subject"`
	Status             string              `yaml:"status" json:"status"`
	MessageKey         string              `yaml:"messageKey" json:"messageKey"`
	MessageParams      map[string]any      `yaml:"messageParams,omitempty" json:"messageParams,omitempty"`
	RuleDescriptionKey string              `yaml:"ruleDescriptionKey" json:"ruleDescriptionKey"`
	ExpiresOn          *Date               `yaml:"expiresOn,omitempty" json:"expiresOn,omitempty"`
	WindowOpensAt      *Date               `yaml:"windowOpensAt,omitempty" json:"windowOpensAt,omitempty"`
	ValidUntil         *Date               `yaml:"validUntil,omitempty" json:"validUntil,omitempty"`
	Requirements       []RequirementResult `yaml:"requirements" json:"requirements"`
	Citations          []string            `yaml:"citations" json:"citations"`
}

// Subject identifies what an evaluation is about.
type Subject struct {
	Kind   string `yaml:"kind" json:"kind"`
	ID     string `yaml:"id,omitempty" json:"id,omitempty"`
	Class  string `yaml:"class,omitempty" json:"class,omitempty"`
	ULKind string `yaml:"ulKind,omitempty" json:"ulKind,omitempty"`
	Detail string `yaml:"detail,omitempty" json:"detail,omitempty"`
}

// RequirementResult is one requirement row.
type RequirementResult struct {
	ID           string         `yaml:"id" json:"id"`
	NameKey      string         `yaml:"nameKey" json:"nameKey"`
	Metric       string         `yaml:"metric" json:"metric"`
	Current      float64        `yaml:"current" json:"current"`
	Required     float64        `yaml:"required" json:"required"`
	Unit         string         `yaml:"unit" json:"unit"`
	Met          bool           `yaml:"met" json:"met"`
	Tracked      bool           `yaml:"tracked" json:"tracked"`
	ValidUntil   *Date          `yaml:"validUntil,omitempty" json:"validUntil,omitempty"`
	LastDate     *Date          `yaml:"lastDate,omitempty" json:"lastDate,omitempty"`
	MessageKey   string         `yaml:"messageKey,omitempty" json:"messageKey,omitempty"`
	RemedyKey    string         `yaml:"remedyKey,omitempty" json:"remedyKey,omitempty"`
	RemedyParams map[string]any `yaml:"remedyParams,omitempty" json:"remedyParams,omitempty"`
}
