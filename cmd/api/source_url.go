// Copyright (C) The NinerLog Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/api/handlers"
)

// sourceURLFromEnv returns SOURCE_URL, or handlers.DefaultSourceURL when it
// is unset. A set value must be an http(s) URL with a host.
func sourceURLFromEnv() (string, error) {
	raw := strings.TrimSpace(os.Getenv("SOURCE_URL"))
	if raw == "" {
		return handlers.DefaultSourceURL, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("SOURCE_URL %q: %w", raw, err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("SOURCE_URL %q: must be an http(s) URL", raw)
	}
	return raw, nil
}
