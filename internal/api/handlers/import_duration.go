package handlers

import (
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/pkg/duration"
)

var importDecimalHours = regexp.MustCompile(`^(?:[0-9]{1,9}(?:\.[0-9]{1,6})?|\.[0-9]{1,6})$`)
var importDurationSyntax = regexp.MustCompile(`^[0-9.hHmM: \t]+$`)
var oversizedDurationNumber = regexp.MustCompile(`[0-9]{11,}`)

// Bound components before the legacy parser multiplies hours by 60. Its flexible
// formats remain supported without allowing integer overflow to become a small,
// apparently valid imported duration.
func boundedLegacyImportDuration(value string) (int, error) {
	if len(value) > 64 || !importDurationSyntax.MatchString(value) || oversizedDurationNumber.MatchString(value) {
		return 0, fmt.Errorf("invalid or excessive duration")
	}
	minutes, err := duration.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	if minutes < 0 || minutes > 2147483647 {
		return 0, fmt.Errorf("duration exceeds integer-minute limit")
	}
	return minutes, nil
}

func parseImportDuration(value string, unit *generated.ImportColumnMappingDurationUnit) (int, error) {
	value = normalizeDecimalSeparator(strings.TrimSpace(value))
	if unit == nil || *unit == generated.ImportColumnMappingDurationUnitAuto {
		return boundedLegacyImportDuration(value)
	}
	if *unit == generated.ImportColumnMappingDurationUnitMinutes {
		n, err := strconv.ParseInt(value, 10, 32)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("expected non-negative integer minutes")
		}
		return int(n), nil
	}
	if *unit != generated.ImportColumnMappingDurationUnitDecimalHours {
		return 0, fmt.Errorf("invalid duration unit")
	}
	// Exact H:M values exported by NinerLog remain unambiguous in any unit mode.
	if strings.Contains(value, ":") || strings.ContainsAny(value, "hHmM") {
		return boundedLegacyImportDuration(value)
	}
	if !importDecimalHours.MatchString(value) {
		return 0, fmt.Errorf("expected decimal hours")
	}
	r, ok := new(big.Rat).SetString(value)
	if !ok {
		return 0, fmt.Errorf("invalid decimal hours")
	}
	r.Mul(r, big.NewRat(60, 1))
	r.Add(r, big.NewRat(1, 2))
	minutes := new(big.Int).Quo(r.Num(), r.Denom())
	if !minutes.IsInt64() || minutes.Int64() > 2147483647 {
		return 0, fmt.Errorf("duration exceeds integer-minute limit")
	}
	return int(minutes.Int64()), nil
}
