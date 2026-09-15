// Copyright (C) The NinerLog Authors
// SPDX-License-Identifier: AGPL-3.0-only

package handlers

import (
	"net/http"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/updatecheck"
	"github.com/gin-gonic/gin"
)

// Program identity reported by GET /about.
const (
	programName = "NinerLog API"
	licenseID   = "AGPL-3.0-only"
	licenseURL  = "https://www.gnu.org/licenses/agpl-3.0.html"

	// DefaultSourceURL is the source offer of an unmodified deployment.
	DefaultSourceURL = "https://github.com/fjaeckel/ninerlog-api"
)

// SetSourceURL sets where GET /about sends users for the corresponding source.
func (h *APIHandler) SetSourceURL(u string) {
	h.sourceURL = u
}

// effectiveSourceURL returns the configured source offer, or DefaultSourceURL.
func (h *APIHandler) effectiveSourceURL() string {
	if h.sourceURL != "" {
		return h.sourceURL
	}
	return DefaultSourceURL
}

// GetAbout implements GET /about
func (h *APIHandler) GetAbout(c *gin.Context) {
	about := generated.About{
		Name:       programName,
		Version:    updatecheck.RunningVersion(),
		License:    licenseID,
		LicenseUrl: licenseURL,
		SourceUrl:  h.effectiveSourceURL(),
	}
	if commit := updatecheck.RunningCommit(); commit != "" {
		about.Commit = &commit
	}
	c.JSON(http.StatusOK, about)
}
