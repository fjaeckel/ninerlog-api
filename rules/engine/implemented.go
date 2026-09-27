package engine

import (
	"reflect"
	"sort"
	"strings"

	"github.com/fjaeckel/ninerlog-rules/engine/hatches"
)

// Implemented lists, per vocabulary section, the names the engine implements.
func Implemented() map[string][]string {
	return map[string][]string{
		"subjects":         {"rating", "licence", "privilege", "credential", "passengers", "flight_review", "training", "type", "launch_method"},
		"metrics":          implementedMetrics(),
		"filters":          implementedFilters(),
		"windows":          {"rolling_days", "rolling_months", "calendar_months", "before_expiry_months", "validity_period", "since_issue", "lifetime"},
		"combinators":      {"all_of", "any_of", "n_of"},
		"stage_conditions": ImplementedConditions,
		"rule_events":      {"restored_by", "resets"},
		"param_sources":    ImplementedParamSources,
		"hatches":          implementedHatches(),
	}
}

func implementedMetrics() []string {
	var out []string
	for n := range flightMetricFuncs {
		out = append(out, n)
	}
	for n := range eventMetricFuncs {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func implementedFilters() []string {
	var out []string
	t := reflect.TypeFor[Filter]()
	for i := range t.NumField() {
		if tag := t.Field(i).Tag.Get("yaml"); tag != "" {
			out = append(out, strings.Split(tag, ",")[0])
		}
	}
	sort.Strings(out)
	return out
}

func implementedHatches() []string {
	var out []string
	for n := range hatches.Registry {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
