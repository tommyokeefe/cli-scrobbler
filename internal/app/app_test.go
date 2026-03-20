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

func TestRunSearchNoScrobbleFlagParsed(t *testing.T) {
	t.Parallel()

	// --no-scrobble should be accepted; the error should be "search query is required", not a flag error.
	err := runSearch([]string{"--no-scrobble"}, strings.NewReader("\n"), &bytes.Buffer{})
	if err == nil {
		t.Fatal("runSearch() error = nil, want error")
	}
	if err.Error() != "search query is required" {
		t.Fatalf("runSearch() error = %q, want %q", err.Error(), "search query is required")
	}
}

func TestRunDispatchValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		args            []string
		wantErr         string
		wantOutContains string
	}{
		{
			name:            "help command",
			args:            []string{"help"},
			wantOutContains: "Commands:",
		},
		{
			name:    "unknown command",
			args:    []string{"unknown"},
			wantErr: "unknown command \"unknown\"",
		},
		{
			name:    "auth without target",
			args:    []string{"auth"},
			wantErr: "expected auth target: discogs or lastfm",
		},
		{
			name:    "search without query",
			args:    []string{"search"},
			wantErr: "search query is required",
		},
		{
			name:    "search accepts no-scrobble flag",
			args:    []string{"search", "--no-scrobble"},
			wantErr: "search query is required",
		},
		{
			name:    "scrobble without query",
			args:    []string{"scrobble"},
			wantErr: "album query is required",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer

			err := Run(tc.args, strings.NewReader("\n"), &out, &out)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("Run(%v) error = nil, want %q", tc.args, tc.wantErr)
				}
				if err.Error() != tc.wantErr {
					t.Fatalf("Run(%v) error = %q, want %q", tc.args, err.Error(), tc.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("Run(%v) error = %v, want nil", tc.args, err)
			}

			if tc.wantOutContains != "" && !strings.Contains(out.String(), tc.wantOutContains) {
				t.Fatalf("Run(%v) output missing %q, got:\n%s", tc.args, tc.wantOutContains, out.String())
			}
		})
	}
}

func TestRunScrobbleValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing album query",
			args:    nil,
			wantErr: "album query is required",
		},
		{
			name:    "missing started-at",
			args:    []string{"Disintegration"},
			wantErr: "--started-at is required",
		},
		{
			name:    "invalid started-at",
			args:    []string{"--started-at", "not-a-time", "Disintegration"},
			wantErr: "unsupported --started-at value \"not-a-time\"",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := runScrobble(tc.args, strings.NewReader("\n"), &bytes.Buffer{})
			if err == nil {
				t.Fatalf("runScrobble(%v) error = nil, want %q", tc.args, tc.wantErr)
			}
			if err.Error() != tc.wantErr {
				t.Fatalf("runScrobble(%v) error = %q, want %q", tc.args, err.Error(), tc.wantErr)
			}
		})
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
