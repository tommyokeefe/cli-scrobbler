package scrobble

import (
	"testing"
	"time"

	"cli-scrobbler/internal/model"
)

func TestBuildTimeline(t *testing.T) {
	t.Parallel()

	album := model.Album{
		Tracks: []model.Track{
			{Title: "First", Duration: 2 * time.Minute},
			{Title: "Second", Duration: 3 * time.Minute},
		},
	}

	startedAt := time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC)
	got, err := BuildTimeline(album, startedAt)
	if err != nil {
		t.Fatalf("BuildTimeline() error = %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("BuildTimeline() len = %d, want 2", len(got))
	}

	if got[0].StartedAt != startedAt {
		t.Fatalf("first track StartedAt = %v, want %v", got[0].StartedAt, startedAt)
	}

	if want := startedAt.Add(2 * time.Minute); got[1].StartedAt != want {
		t.Fatalf("second track StartedAt = %v, want %v", got[1].StartedAt, want)
	}
}
