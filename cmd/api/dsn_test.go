package main

import (
	"net/url"
	"testing"
	"time"
)

func TestWithStatementTimeout_URLDSN(t *testing.T) {
	got := withStatementTimeout("postgresql://localhost:5432/ninerlog?sslmode=disable", 10*time.Second)

	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("result is not a valid URL: %v", err)
	}
	if sslmode := u.Query().Get("sslmode"); sslmode != "disable" {
		t.Errorf("existing sslmode param lost, got %q", sslmode)
	}
	if opts := u.Query().Get("options"); opts != "-c statement_timeout=10000" {
		t.Errorf("options = %q, want %q", opts, "-c statement_timeout=10000")
	}
}

func TestWithStatementTimeout_PreservesExistingOptions(t *testing.T) {
	got := withStatementTimeout("postgres://localhost/db?options=-c%20lock_timeout%3D5000", 10*time.Second)

	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("result is not a valid URL: %v", err)
	}
	opts := u.Query().Get("options")
	if opts != "-c lock_timeout=5000 -c statement_timeout=10000" {
		t.Errorf("options = %q, want existing option preserved and appended", opts)
	}
}

func TestWithStatementTimeout_KeywordValueDSN(t *testing.T) {
	got := withStatementTimeout("host=localhost user=postgres dbname=ninerlog", 10*time.Second)

	want := "host=localhost user=postgres dbname=ninerlog options='-c statement_timeout=10000'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
