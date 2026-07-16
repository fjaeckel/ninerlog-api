package main

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// withStatementTimeout returns dbURL with a Postgres statement_timeout
// applied via the "options" connection parameter, so the server itself
// kills any query that runs longer than timeout — a backstop independent of
// client-side context deadlines (see middleware.RequestTimeoutMiddleware).
//
// dbURL may be a URL-style DSN ("postgres://…" / "postgresql://…", the only
// form this codebase constructs) or a keyword/value DSN ("host=… user=…").
// Keyword/value DSNs are passed through with the option appended, since
// url.Parse cannot round-trip them without risking corruption of an
// already-set "options" value.
func withStatementTimeout(dbURL string, timeout time.Duration) string {
	setting := fmt.Sprintf("-c statement_timeout=%d", timeout.Milliseconds())

	u, err := url.Parse(dbURL)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return dbURL + " options='" + setting + "'"
	}

	q := u.Query()
	if existing := q.Get("options"); existing != "" {
		q.Set("options", strings.TrimSpace(existing)+" "+setting)
	} else {
		q.Set("options", setting)
	}
	u.RawQuery = q.Encode()
	return u.String()
}
