package scrobble

import (
	"fmt"
	"time"

	"cli-scrobbler/internal/model"
)

type ScheduledTrack struct {
	Track     model.Track
	StartedAt time.Time
}

func BuildTimeline(album model.Album, startedAt time.Time) ([]ScheduledTrack, error) {
	tracks := make([]ScheduledTrack, 0, len(album.Tracks))
	current := startedAt

	for _, track := range album.Tracks {
		if track.Duration <= 0 {
			return nil, fmt.Errorf("track %q is missing a duration", track.Title)
		}

		tracks = append(tracks, ScheduledTrack{
			Track:     track,
			StartedAt: current,
		})

		current = current.Add(track.Duration)
	}

	return tracks, nil
}
