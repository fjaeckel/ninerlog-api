package handlers

import (
	"os"
	"strings"
)

// frontendBaseURL resolves the canonical frontend origin used to build links
// embedded in outbound emails (verification, password reset, signature
// requests, ...). Mirrors the FRONTEND_URL -> CORS_ORIGIN -> localhost
// fallback chain used ad hoc elsewhere; CORS_ORIGIN may contain multiple
// comma-separated origins, so only the first is used.
func frontendBaseURL() string {
	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = os.Getenv("CORS_ORIGIN")
	}
	if frontendURL == "" {
		frontendURL = "http://localhost:5173"
	}
	if idx := strings.Index(frontendURL, ","); idx > 0 {
		frontendURL = frontendURL[:idx]
	}
	return strings.TrimSpace(frontendURL)
}
