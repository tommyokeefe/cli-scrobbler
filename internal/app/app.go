package app

import (
	"bufio"
	"context"
	"flag"
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

const appTitle = "Discogs CLI Scrobbler"

var appHeader = []string{
	"░███████   ░██",
	"░██   ░██",
	"░██    ░██ ░██ ░███████   ░███████   ░███████   ░████████  ░███████",
	"░██    ░██ ░██░██        ░██    ░██ ░██    ░██ ░██    ░██ ░██",
	"░██    ░██ ░██ ░███████  ░██        ░██    ░██ ░██    ░██  ░███████",
	"░██   ░██  ░██       ░██ ░██    ░██ ░██    ░██ ░██   ░███        ░██",
	"░███████   ░██ ░███████   ░███████   ░███████   ░█████░██  ░███████",
	"                                                      ░██",
	"                                                ░███████",
	"",
	"  ░██████  ░██         ░██████",
	" ░██   ░██ ░██           ░██",
	"░██        ░██           ░██",
	"░██        ░██           ░██",
	"░██        ░██           ░██",
	" ░██   ░██ ░██           ░██",
	"  ░██████  ░██████████ ░██████",
	"",
	"",
	"",
	"  ░██████                                  ░██        ░██        ░██",
	" ░██   ░██                                 ░██        ░██        ░██",
	"░██          ░███████  ░██░████  ░███████  ░████████  ░████████  ░██  ░███████  ░██░████",
	" ░████████  ░██    ░██ ░███     ░██    ░██ ░██    ░██ ░██    ░██ ░██ ░██    ░██ ░███",
	"        ░██ ░██        ░██      ░██    ░██ ░██    ░██ ░██    ░██ ░██ ░█████████ ░██",
	" ░██   ░██  ░██    ░██ ░██      ░██    ░██ ░███   ░██ ░███   ░██ ░██ ░██        ░██",
	"  ░██████    ░███████  ░██       ░███████  ░██░█████  ░██░█████  ░██  ░███████  ░██",
	"",
	"",
	"",
	"",
}

func Run(args []string, in io.Reader, out, errOut io.Writer) error {
	_ = errOut

	fs := flag.NewFlagSet("scrobble", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	noScrobble := fs.Bool("no-scrobble", false, "Dry run: skip sending scrobbles to Last.fm")
	if err := fs.Parse(args); err != nil {
		return err
	}

	remaining := fs.Args()
	if len(remaining) == 0 {
		return runInteractive(in, out, *noScrobble)
	}

	switch remaining[0] {
	case "auth":
		return runAuth(remaining[1:], in, out)
	case "search":
		return runSearch(remaining[1:], in, out)
	case "scrobble":
		return runScrobble(remaining[1:], in, out)
	case "help", "-h", "--help":
		printUsage(out)
		return nil
	default:
		return fmt.Errorf("unknown command %q", remaining[0])
	}
}

func runAuth(args []string, in io.Reader, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("expected auth target: discogs or lastfm")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	switch args[0] {
	case "discogs":
		fs := flag.NewFlagSet("auth discogs", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		token := fs.String("token", "", "Discogs personal access token")
		username := fs.String("username", strings.TrimSpace(cfg.DiscogsUsername), "Discogs username for collection access")
		userAgent := fs.String("user-agent", cfg.DiscogsUserAgent, "Discogs User-Agent header")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*token) == "" {
			return fmt.Errorf("discogs token is required")
		}

		cfg.DiscogsToken = strings.TrimSpace(*token)
		cfg.DiscogsUsername = strings.TrimSpace(*username)
		cfg.DiscogsUserAgent = strings.TrimSpace(*userAgent)
		if err := config.Save(cfg); err != nil {
			return err
		}

		fmt.Fprintln(out, "Saved Discogs credentials.")
		return nil
	case "lastfm":
		fs := flag.NewFlagSet("auth lastfm", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		sessionKey := fs.String("session-key", "", "Last.fm session key")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		if cfg.MissingLastFMAppCredentials() {
			return fmt.Errorf("missing Last.fm app credentials; set SCROBBLER_LASTFM_API_KEY / SCROBBLER_LASTFM_API_SECRET or add lastfm_api_key / lastfm_api_secret in a repo-root config.json during development")
		}

		if strings.TrimSpace(*sessionKey) != "" {
			cfg.LastFMSessionKey = strings.TrimSpace(*sessionKey)
		} else {
			reader := bufio.NewReader(in)
			guidedSessionKey, err := guideLastFMSessionKey(reader, out, cfg.LastFMAPIKey, cfg.LastFMAPISecret)
			if err != nil {
				return err
			}
			cfg.LastFMSessionKey = guidedSessionKey
		}

		if err := config.Save(cfg); err != nil {
			return err
		}

		fmt.Fprintln(out, "Saved Last.fm session.")
		return nil
	default:
		return fmt.Errorf("unknown auth target %q", args[0])
	}
}

func runSearch(args []string, in io.Reader, out io.Writer) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	noScrobble := fs.Bool("no-scrobble", false, "Dry run: skip sending scrobbles to Last.fm")
	if err := fs.Parse(args); err != nil {
		return err
	}

	query := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if query == "" {
		return fmt.Errorf("search query is required")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.MissingDiscogs() {
		return fmt.Errorf("missing Discogs token; run `auth discogs --token <token>` or set %s", "SCROBBLER_DISCOGS_TOKEN")
	}

	if !*noScrobble {
		if cfg.MissingLastFMAppCredentials() {
			return fmt.Errorf("missing Last.fm app credentials; set SCROBBLER_LASTFM_API_KEY / SCROBBLER_LASTFM_API_SECRET or add lastfm_api_key / lastfm_api_secret in a repo-root config.json during development")
		}
		if cfg.MissingLastFMSession() {
			return fmt.Errorf("missing Last.fm session; run `auth lastfm` to complete the browser auth flow or set SCROBBLER_LASTFM_SESSION_KEY")
		}
	}

	reader := bufio.NewReader(in)

	release, err := resolveRelease(context.Background(), cfg, query, reader, out)
	if err != nil {
		if strings.HasPrefix(err.Error(), "no albums in your Discogs collection matched") {
			fmt.Fprintln(out, "No matching albums found in your Discogs collection.")
			return nil
		}
		return err
	}

	startedAt, err := promptStartedAt(reader, out)
	if err != nil {
		return err
	}

	return scrobbleRelease(context.Background(), cfg, release, startedAt, reader, out, *noScrobble)
}

func runScrobble(args []string, in io.Reader, out io.Writer) error {
	fs := flag.NewFlagSet("scrobble", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	startedAtValue := fs.String("started-at", "", "Album start time (RFC3339 or YYYY-MM-DD HH:MM[:SS])")
	noScrobble := fs.Bool("no-scrobble", false, "Dry run: skip sending scrobbles to Last.fm")
	if err := fs.Parse(args); err != nil {
		return err
	}

	query := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if query == "" {
		return fmt.Errorf("album query is required")
	}
	if strings.TrimSpace(*startedAtValue) == "" {
		return fmt.Errorf("--started-at is required")
	}

	startedAt, err := parseStartedAt(*startedAtValue)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.MissingDiscogs() {
		return fmt.Errorf("missing Discogs token; run `auth discogs --token <token>` or set %s", "SCROBBLER_DISCOGS_TOKEN")
	}
	if !*noScrobble {
		if cfg.MissingLastFMAppCredentials() {
			return fmt.Errorf("missing Last.fm app credentials; set SCROBBLER_LASTFM_API_KEY / SCROBBLER_LASTFM_API_SECRET or add lastfm_api_key / lastfm_api_secret in a repo-root config.json during development")
		}
		if cfg.MissingLastFMSession() {
			return fmt.Errorf("missing Last.fm session; run `auth lastfm` to complete the browser auth flow or set SCROBBLER_LASTFM_SESSION_KEY")
		}
	}

	reader := bufio.NewReader(in)

	release, err := resolveRelease(context.Background(), cfg, query, reader, out)
	if err != nil {
		return err
	}

	return scrobbleRelease(context.Background(), cfg, release, startedAt, reader, out, *noScrobble)
}

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

func printUsage(out io.Writer) {
	printHeader(out)
	fmt.Fprintln(out, "Run with no arguments to start the interactive app.")
	fmt.Fprintln(out, "Pass --no-scrobble before any command for a dry run (tracks not sent to Last.fm).")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Commands:")
	fmt.Fprintln(out, "  auth discogs --token <token> [--username <name>] [--user-agent <ua>]")
	fmt.Fprintln(out, "  auth lastfm [--session-key <key>]")
	fmt.Fprintln(out, "  search [--no-scrobble] <album query>  # search, select an album, and scrobble with prompted start time")
	fmt.Fprintln(out, "  scrobble --started-at <time> [--no-scrobble] <album query>")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Environment overrides:")
	fmt.Fprintln(out, "  SCROBBLER_DISCOGS_TOKEN")
	fmt.Fprintln(out, "  SCROBBLER_DISCOGS_USERNAME")
	fmt.Fprintln(out, "  SCROBBLER_LASTFM_API_KEY")
	fmt.Fprintln(out, "  SCROBBLER_LASTFM_API_SECRET")
	fmt.Fprintln(out, "  SCROBBLER_LASTFM_SESSION_KEY")
}

func runInteractive(in io.Reader, out io.Writer, noScrobble bool) error {
	reader := bufio.NewReader(in)

	printHeader(out)
	fmt.Fprintln(out, "✨Scrobble albums from your Discogs collection to Last.fm, right from the command line!✨")
	fmt.Fprintln(out)

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	cfg, err = ensureConnections(reader, out, cfg)
	if err != nil {
		return err
	}

	for {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "What would you like to do?")
		fmt.Fprintln(out, "1. 🔎 Search and scrobble an album")
		fmt.Fprintln(out, "2. 🔐 Update Discogs / Last.fm connection settings")
		fmt.Fprintln(out, "3. 👋 Exit")

		selection, err := promptIndex(reader, out, 3)
		if err != nil {
			return err
		}

		switch selection {
		case 0:
			if err := interactiveSearch(reader, out, cfg, noScrobble); err != nil {
				return err
			}
		case 1:
			cfg, err = runConnectionWizard(reader, out, cfg, true)
			if err != nil {
				return err
			}
		case 2:
			fmt.Fprintln(out, "Goodbye.")
			return nil
		}
	}
}

func ensureConnections(reader *bufio.Reader, out io.Writer, cfg config.Config) (config.Config, error) {
	if !cfg.MissingDiscogs() && !cfg.MissingLastFM() {
		fmt.Fprintln(out, "✅ Discogs and Last.fm are configured.")
		return cfg, nil
	}

	fmt.Fprintln(out, "Let's connect your accounts.")
	return runConnectionWizard(reader, out, cfg, false)
}

func runConnectionWizard(reader *bufio.Reader, out io.Writer, cfg config.Config, allowSkip bool) (config.Config, error) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Discogs setup")
	fmt.Fprintln(out, "Provide a Discogs personal access token. You can also save your username if identity lookup is unavailable.")

	if allowSkip && !cfg.MissingDiscogs() {
		updateDiscogs, err := promptYesNo(reader, out, "Update Discogs settings?", false)
		if err != nil {
			return cfg, err
		}
		if updateDiscogs {
			if err := promptDiscogsConfig(reader, out, &cfg); err != nil {
				return cfg, err
			}
		}
	} else {
		if err := promptDiscogsConfig(reader, out, &cfg); err != nil {
			return cfg, err
		}
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "Last.fm setup")
	fmt.Fprintln(out, "Set Last.fm app credentials via environment variables or a repo-root config.json during development, then connect the current user's Last.fm account.")

	if allowSkip && !cfg.MissingLastFM() {
		updateLastFM, err := promptYesNo(reader, out, "Update Last.fm settings?", false)
		if err != nil {
			return cfg, err
		}
		if updateLastFM {
			if err := promptLastFMConfig(reader, out, &cfg); err != nil {
				return cfg, err
			}
		}
	} else {
		if err := promptLastFMConfig(reader, out, &cfg); err != nil {
			return cfg, err
		}
	}

	if err := config.Save(cfg); err != nil {
		return cfg, err
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "Saved connection settings.")
	return cfg, nil
}

func promptDiscogsConfig(reader *bufio.Reader, out io.Writer, cfg *config.Config) error {
	token, err := promptSecretValue(reader, out, "Discogs personal access token", cfg.DiscogsToken)
	if err != nil {
		return err
	}

	username, err := promptOptionalValue(reader, out, "Discogs username (optional; leave blank to auto-detect)", cfg.DiscogsUsername)
	if err != nil {
		return err
	}

	userAgent, err := promptRequiredValue(reader, out, "Discogs User-Agent", cfg.DiscogsUserAgent)
	if err != nil {
		return err
	}

	cfg.DiscogsToken = token
	cfg.DiscogsUsername = username
	cfg.DiscogsUserAgent = userAgent
	return nil
}

func promptLastFMConfig(reader *bufio.Reader, out io.Writer, cfg *config.Config) error {
	if cfg.MissingLastFMAppCredentials() {
		return fmt.Errorf("missing Last.fm app credentials; set SCROBBLER_LASTFM_API_KEY / SCROBBLER_LASTFM_API_SECRET or add lastfm_api_key / lastfm_api_secret in a repo-root config.json during development")
	}

	sessionKey, err := promptLastFMSessionKey(reader, out, cfg.LastFMAPIKey, cfg.LastFMAPISecret, cfg.LastFMSessionKey)
	if err != nil {
		return err
	}
	cfg.LastFMSessionKey = sessionKey
	return nil
}

func interactiveSearch(reader *bufio.Reader, out io.Writer, cfg config.Config, noScrobble bool) error {
	query, err := promptRequiredValue(reader, out, "Search your collection for", "")
	if err != nil {
		return err
	}

	release, err := resolveRelease(context.Background(), cfg, query, reader, out)
	if err != nil {
		if strings.HasPrefix(err.Error(), "no albums in your Discogs collection matched") {
			fmt.Fprintln(out, "No matching albums found in your Discogs collection.")
			return nil
		}
		return err
	}

	startedAt, err := promptStartedAt(reader, out)
	if err != nil {
		return err
	}

	return scrobbleRelease(context.Background(), cfg, release, startedAt, reader, out, noScrobble)
}

func interactiveScrobble(reader *bufio.Reader, out io.Writer, cfg config.Config, noScrobble bool) (config.Config, error) {
	if cfg.MissingDiscogs() || cfg.MissingLastFM() {
		var err error
		cfg, err = ensureConnections(reader, out, cfg)
		if err != nil {
			return cfg, err
		}
	}

	query, err := promptRequiredValue(reader, out, "Album to scrobble", "")
	if err != nil {
		return cfg, err
	}

	startedAt, err := promptStartedAt(reader, out)
	if err != nil {
		return cfg, err
	}

	release, err := resolveRelease(context.Background(), cfg, query, reader, out)
	if err != nil {
		return cfg, err
	}
	if err := scrobbleRelease(context.Background(), cfg, release, startedAt, reader, out, noScrobble); err != nil {
		return cfg, err
	}
	return cfg, nil
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
