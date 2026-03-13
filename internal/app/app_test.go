package app

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestParseStartedAt(t *testing.T) {
	t.Parallel()

	if _, err := parseStartedAt("2026-03-13T12:00:00Z"); err != nil {
		t.Fatalf("parseStartedAt(RFC3339) error = %v", err)
	}

	got, err := parseStartedAt("2026-03-13 12:00")
	if err != nil {
		t.Fatalf("parseStartedAt(local) error = %v", err)
	}

	if got.Year() != 2026 || got.Month() != time.March || got.Day() != 13 {
		t.Fatalf("parseStartedAt(local) = %v, unexpected date", got)
	}
}

func TestRunSearchRequiresQuery(t *testing.T) {
	t.Parallel()

	err := runSearch(nil, strings.NewReader("\n"), &bytes.Buffer{})
	if err == nil {
		t.Fatal("runSearch() error = nil, want error")
	}
	if err.Error() != "search query is required" {
		t.Fatalf("runSearch() error = %q, want %q", err.Error(), "search query is required")
	}
}
