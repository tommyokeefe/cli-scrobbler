package app

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"cli-scrobbler/internal/model"
	"cli-scrobbler/internal/scrobble"
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

func TestFormatTrackDuration(t *testing.T) {
	t.Parallel()

	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "--:--"},
		{-1 * time.Second, "--:--"},
		{3*time.Minute + 45*time.Second, "3:45"},
		{10*time.Minute + 5*time.Second, "10:05"},
		{1*time.Hour + 2*time.Minute + 3*time.Second, "1:02:03"},
	}

	for _, tc := range cases {
		got := formatTrackDuration(tc.d)
		if got != tc.want {
			t.Errorf("formatTrackDuration(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestPrintAlbumDetails(t *testing.T) {
	t.Parallel()

	album := model.Album{
		Artist: "The Cure",
		Title:  "Disintegration",
		Year:   1989,
		Tracks: []model.Track{
			{Position: "A1", Title: "Plainsong", Duration: 5*time.Minute + 17*time.Second},
			{Position: "A2", Title: "Pictures of You", Duration: 0},
		},
	}

	var buf bytes.Buffer
	printAlbumDetails(&buf, album)
	out := buf.String()

	if !strings.Contains(out, "The Cure - Disintegration (1989)") {
		t.Errorf("output missing album header, got:\n%s", out)
	}
	if !strings.Contains(out, "Plainsong") {
		t.Errorf("output missing track title, got:\n%s", out)
	}
	if !strings.Contains(out, "5:17") {
		t.Errorf("output missing formatted duration, got:\n%s", out)
	}
	if !strings.Contains(out, "--:--") {
		t.Errorf("output missing placeholder for zero duration, got:\n%s", out)
	}
}

func TestPrintTimeline(t *testing.T) {
	t.Parallel()

	release := model.Album{
		Artist: "Portishead",
		Title:  "Dummy",
		Year:   1994,
	}
	startedAt := time.Date(2026, 3, 20, 20, 0, 0, 0, time.UTC)
	timeline := []scrobble.ScheduledTrack{
		{Track: model.Track{Position: "1", Title: "Mysterons", Duration: 5 * time.Minute}, StartedAt: startedAt},
		{Track: model.Track{Position: "2", Title: "Sour Times", Duration: 4 * time.Minute}, StartedAt: startedAt.Add(5 * time.Minute)},
	}

	var buf bytes.Buffer
	printTimeline(&buf, release, timeline)
	out := buf.String()

	if !strings.Contains(out, "Scrobbled Portishead - Dummy (1994)") {
		t.Errorf("output missing scrobble header, got:\n%s", out)
	}
	if !strings.Contains(out, "Mysterons") || !strings.Contains(out, "Sour Times") {
		t.Errorf("output missing track titles, got:\n%s", out)
	}
	if !strings.Contains(out, "5:00") {
		t.Errorf("output missing track duration, got:\n%s", out)
	}
}
