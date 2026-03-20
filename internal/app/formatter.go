package app

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"cli-scrobbler/internal/model"
	"cli-scrobbler/internal/scrobble"
)

func printHeader(out io.Writer) {
	fmt.Fprintln(out)

	if colorsEnabled() {
		for i, line := range appHeader {
			_ = i
			fmt.Fprintf(out, "\x1b[1;38;5;205m%s\x1b[0m\n", line)
		}
		return
	}

	for _, line := range appHeader {
		fmt.Fprintln(out, line)
	}
	fmt.Fprintln(out)
}

func colorsEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	term := strings.ToLower(strings.TrimSpace(os.Getenv("TERM")))
	if term == "" || term == "dumb" {
		return false
	}

	return true
}

func printAlbumDetails(out io.Writer, album model.Album) {
	fmt.Fprintln(out)
	if album.Year != 0 {
		fmt.Fprintf(out, "%s - %s (%d)\n", album.Artist, album.Title, album.Year)
	} else {
		fmt.Fprintf(out, "%s - %s\n", album.Artist, album.Title)
	}
	for _, track := range album.Tracks {
		fmt.Fprintf(out, "  %-4s  %-40s  %5s\n", track.Position, track.Title, formatTrackDuration(track.Duration))
	}
	fmt.Fprintln(out)
}

func printTimeline(out io.Writer, release model.Album, timeline []scrobble.ScheduledTrack) {
	fmt.Fprintln(out)
	if release.Year != 0 {
		fmt.Fprintf(out, "Scrobbled %s - %s (%d)\n", release.Artist, release.Title, release.Year)
	} else {
		fmt.Fprintf(out, "Scrobbled %s - %s\n", release.Artist, release.Title)
	}
	for _, st := range timeline {
		fmt.Fprintf(out, "  %-4s  %-40s  %5s  %s\n",
			st.Track.Position,
			st.Track.Title,
			formatTrackDuration(st.Track.Duration),
			st.StartedAt.Local().Format("3:04 PM"),
		)
	}
	fmt.Fprintln(out)
}

func formatTrackDuration(d time.Duration) string {
	if d <= 0 {
		return "--:--"
	}
	totalSec := int(d.Seconds())
	min := totalSec / 60
	sec := totalSec % 60
	if min >= 60 {
		h := min / 60
		min = min % 60
		return fmt.Sprintf("%d:%02d:%02d", h, min, sec)
	}
	return fmt.Sprintf("%d:%02d", min, sec)
}
