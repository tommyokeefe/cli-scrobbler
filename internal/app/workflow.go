package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"cli-scrobbler/internal/cache"
	"cli-scrobbler/internal/config"
	"cli-scrobbler/internal/discogs"
	"cli-scrobbler/internal/lastfm"
	"cli-scrobbler/internal/model"
	"cli-scrobbler/internal/scrobble"
)

func scrobbleRelease(ctx context.Context, cfg config.Config, release model.Album, startedAt time.Time, reader *bufio.Reader, out io.Writer, noScrobble bool) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	store, err := cache.Load(paths.DurationCache)
	if err != nil {
		return err
	}

	release, askedForDurations, err := hydrateDurations(release, store, reader, out)
	if err != nil {
		return err
	}
	if askedForDurations {
		if err := store.Save(); err != nil {
			return err
		}
	}

	timeline, err := scrobble.BuildTimeline(release, startedAt)
	if err != nil {
		return err
	}

	client := lastfm.NewClient(cfg.LastFMAPIKey, cfg.LastFMAPISecret, cfg.LastFMSessionKey)
	if !noScrobble {
		if err := client.Scrobble(ctx, timeline, release.Title); err != nil {
			return err
		}
	}

	printTimeline(out, release, timeline)
	if noScrobble {
		fmt.Fprintln(out, "Dry run — tracks not sent to Last.fm.")
	}
	return nil
}

func resolveRelease(ctx context.Context, cfg config.Config, query string, reader *bufio.Reader, out io.Writer) (model.Album, error) {
	client := discogs.NewClient(cfg.DiscogsToken, cfg.DiscogsUserAgent)
	results, err := searchCollection(ctx, cfg, query)
	if err != nil {
		return model.Album{}, err
	}
	if len(results) == 0 {
		return model.Album{}, fmt.Errorf("no albums in your Discogs collection matched %q", query)
	}

	releaseID := results[0].ReleaseID
	if len(results) > 1 {
		fmt.Fprintln(out, "Select an album from your Discogs collection:")
		for i, result := range results {
			fmt.Fprintf(out, "%d. %s - %s", i+1, result.Artist, result.Title)
			if result.Year != 0 {
				fmt.Fprintf(out, " (%d)", result.Year)
			}
			if len(result.Formats) > 0 {
				fmt.Fprintf(out, " - %s", strings.Join(result.Formats, ", "))
			}
			fmt.Fprintf(out, " [release_id=%d]\n", result.ReleaseID)
		}

		selection, err := promptIndex(reader, out, len(results))
		if err != nil {
			return model.Album{}, err
		}
		releaseID = results[selection].ReleaseID
	}

	album, err := client.GetRelease(ctx, releaseID)
	if err != nil {
		return model.Album{}, err
	}

	printAlbumDetails(out, album)
	return album, nil
}

func hydrateDurations(release model.Album, store *cache.DurationStore, reader *bufio.Reader, out io.Writer) (model.Album, bool, error) {
	asked := false

	for i, track := range release.Tracks {
		if track.Duration > 0 {
			continue
		}

		if cached, ok := store.Lookup(release.ReleaseID, track); ok {
			release.Tracks[i].Duration = cached
			continue
		}

		duration, err := promptDuration(reader, out, track)
		if err != nil {
			return model.Album{}, false, err
		}

		release.Tracks[i].Duration = duration
		store.Put(release.ReleaseID, release.Tracks[i], duration)
		asked = true
	}

	return release, asked, nil
}

func searchCollection(ctx context.Context, cfg config.Config, query string) ([]discogs.CollectionRelease, error) {
	client := discogs.NewClient(cfg.DiscogsToken, cfg.DiscogsUserAgent)
	username, err := resolveDiscogsUsername(ctx, client, cfg)
	if err != nil {
		return nil, err
	}

	releases, err := client.CollectionReleases(ctx, username)
	if err != nil {
		return nil, err
	}

	return discogs.SearchCollection(query, releases), nil
}

func resolveDiscogsUsername(ctx context.Context, client *discogs.Client, cfg config.Config) (string, error) {
	if username := strings.TrimSpace(cfg.DiscogsUsername); username != "" {
		return username, nil
	}

	username, err := client.Identity(ctx)
	if err != nil {
		return "", fmt.Errorf("resolve Discogs username: %w (or configure it with `auth discogs --token <token> --username <name>` / SCROBBLER_DISCOGS_USERNAME)", err)
	}

	return username, nil
}
