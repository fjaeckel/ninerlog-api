package main

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// withStatementTimeout returns dbURL with a Postgres statement_timeout
// applied via the "options" connection parameter. dbURL may be a URL-style
// DSN ("postgres://…" / "postgresql://…") or a keyword/value DSN
// ("host=… user=…"); keyword/value DSNs are passed through with the option
// appended.
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
