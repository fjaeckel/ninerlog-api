package handlers

import (
	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
)

// convertToGeneratedWarnings maps service warnings to the response field,
// nil when there are none.
func convertToGeneratedWarnings(ws []models.Warning) *[]generated.SaveWarning {
	if len(ws) == 0 {
		return nil
	}
	out := make([]generated.SaveWarning, 0, len(ws))
	for _, w := range ws {
		params := w.Params
		if params == nil {
			params = map[string]any{}
		}
		out = append(out, generated.SaveWarning{
			Code:     generated.SaveWarningCode(w.Code),
			Severity: generated.SaveWarningSeverity(w.Severity),
			Params:   params,
		})
	}
	return &out
}
