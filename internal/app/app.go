package app

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"cli-scrobbler/internal/cache"
	"cli-scrobbler/internal/config"
	"cli-scrobbler/internal/discogs"
	"cli-scrobbler/internal/lastfm"
	"cli-scrobbler/internal/model"
	"cli-scrobbler/internal/scrobble"
)

func Run(args []string, in io.Reader, out, errOut io.Writer) error {
	_ = errOut

	if len(args) == 0 {
		return runInteractive(in, out)
	}

	switch args[0] {
	case "auth":
		return runAuth(args[1:], in, out)
	case "search":
		return runSearch(args[1:], in, out)
	case "scrobble":
		return runScrobble(args[1:], in, out)
	case "help", "-h", "--help":
		printUsage(out)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
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
	case "lastfm-app":
		fs := flag.NewFlagSet("auth lastfm-app", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		apiKey := fs.String("api-key", "", "Last.fm API key")
		apiSecret := fs.String("api-secret", "", "Last.fm API secret")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*apiKey) == "" || strings.TrimSpace(*apiSecret) == "" {
			return fmt.Errorf("api-key and api-secret are required")
		}

		cfg.LastFMAPIKey = strings.TrimSpace(*apiKey)
		cfg.LastFMAPISecret = strings.TrimSpace(*apiSecret)
		if err := config.Save(cfg); err != nil {
			return err
		}

		fmt.Fprintln(out, "Saved Last.fm app credentials.")
		return nil
	case "lastfm":
		fs := flag.NewFlagSet("auth lastfm", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		sessionKey := fs.String("session-key", "", "Last.fm session key")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		if cfg.MissingLastFMAppCredentials() {
			return fmt.Errorf("missing Last.fm app credentials; run `auth lastfm-app --api-key <key> --api-secret <secret>` or set SCROBBLER_LASTFM_API_KEY / SCROBBLER_LASTFM_API_SECRET")
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
	query := strings.TrimSpace(strings.Join(args, " "))
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

	if cfg.MissingLastFMAppCredentials() {
		return fmt.Errorf("missing Last.fm app credentials; run `auth lastfm-app --api-key ... --api-secret ...` or set SCROBBLER_LASTFM_API_KEY / SCROBBLER_LASTFM_API_SECRET")
	}
	if cfg.MissingLastFMSession() {
		return fmt.Errorf("missing Last.fm session; run `auth lastfm` to complete the browser auth flow or set SCROBBLER_LASTFM_SESSION_KEY")
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

	return scrobbleRelease(context.Background(), cfg, release, startedAt, reader, out)
}

func runScrobble(args []string, in io.Reader, out io.Writer) error {
	fs := flag.NewFlagSet("scrobble", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	startedAtValue := fs.String("started-at", "", "Album start time (RFC3339 or YYYY-MM-DD HH:MM[:SS])")
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
	if cfg.MissingLastFMAppCredentials() {
		return fmt.Errorf("missing Last.fm app credentials; run `auth lastfm-app --api-key ... --api-secret ...` or set SCROBBLER_LASTFM_API_KEY / SCROBBLER_LASTFM_API_SECRET")
	}
	if cfg.MissingLastFMSession() {
		return fmt.Errorf("missing Last.fm session; run `auth lastfm` to complete the browser auth flow or set SCROBBLER_LASTFM_SESSION_KEY")
	}

	reader := bufio.NewReader(in)

	release, err := resolveRelease(context.Background(), cfg, query, reader, out)
	if err != nil {
		return err
	}

	return scrobbleRelease(context.Background(), cfg, release, startedAt, reader, out)
}

func scrobbleRelease(ctx context.Context, cfg config.Config, release model.Album, startedAt time.Time, reader *bufio.Reader, out io.Writer) error {
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
	if err := client.Scrobble(ctx, timeline, release.Title); err != nil {
		return err
	}

	fmt.Fprintf(out, "Scrobbled %d tracks from %s - %s starting at %s.\n", len(timeline), release.Artist, release.Title, startedAt.Format(time.RFC3339))
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

	if len(results) == 1 {
		return client.GetRelease(ctx, results[0].ReleaseID)
	}

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

	return client.GetRelease(ctx, results[selection].ReleaseID)
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

func promptIndex(reader *bufio.Reader, out io.Writer, max int) (int, error) {
	for {
		fmt.Fprintf(out, "Enter selection [1-%d]: ", max)
		line, err := reader.ReadString('\n')
		if err != nil {
			return 0, fmt.Errorf("read selection: %w", err)
		}

		value, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil || value < 1 || value > max {
			fmt.Fprintln(out, "Please enter a valid selection.")
			continue
		}

		return value - 1, nil
	}
}

func promptDuration(reader *bufio.Reader, out io.Writer, track model.Track) (time.Duration, error) {
	for {
		fmt.Fprintf(out, "Enter duration for %s %q (mm:ss, hh:mm:ss, or seconds): ", track.Position, track.Title)
		line, err := reader.ReadString('\n')
		if err != nil {
			return 0, fmt.Errorf("read duration: %w", err)
		}

		duration, err := parsePromptDuration(strings.TrimSpace(line))
		if err != nil {
			fmt.Fprintf(out, "Invalid duration: %v\n", err)
			continue
		}
		return duration, nil
	}
}

func parsePromptDuration(value string) (time.Duration, error) {
	if value == "" {
		return 0, errors.New("duration is required")
	}

	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0, errors.New("duration must be positive")
		}
		return time.Duration(seconds) * time.Second, nil
	}

	return discogs.ParseDuration(value)
}

func parseStartedAt(value string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
	}

	for _, format := range formats {
		var (
			parsed time.Time
			err    error
		)

		if format == time.RFC3339 {
			parsed, err = time.Parse(format, value)
		} else {
			parsed, err = time.ParseInLocation(format, value, time.Local)
		}
		if err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported --started-at value %q", value)
}

func printUsage(out io.Writer) {
	fmt.Fprintln(out, "cli-scrobbler")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Run with no arguments to start the interactive app.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Commands:")
	fmt.Fprintln(out, "  auth discogs --token <token> [--username <name>] [--user-agent <ua>]")
	fmt.Fprintln(out, "  auth lastfm-app --api-key <key> --api-secret <secret>")
	fmt.Fprintln(out, "  auth lastfm [--session-key <key>]")
	fmt.Fprintln(out, "  search <album query>          # search, select an album, and scrobble with prompted start time")
	fmt.Fprintln(out, "  scrobble --started-at <time> <album query>")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Environment overrides:")
	fmt.Fprintln(out, "  SCROBBLER_DISCOGS_TOKEN")
	fmt.Fprintln(out, "  SCROBBLER_DISCOGS_USERNAME")
	fmt.Fprintln(out, "  SCROBBLER_LASTFM_API_KEY")
	fmt.Fprintln(out, "  SCROBBLER_LASTFM_API_SECRET")
	fmt.Fprintln(out, "  SCROBBLER_LASTFM_SESSION_KEY")
}

func runInteractive(in io.Reader, out io.Writer) error {
	reader := bufio.NewReader(in)

	fmt.Fprintln(out, "Welcome to cli-scrobbler.")
	fmt.Fprintln(out, "This interactive mode will help you connect Discogs and Last.fm, then search and scrobble albums.")
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
		fmt.Fprintln(out, "1. Search my Discogs collection")
		fmt.Fprintln(out, "2. Scrobble an album")
		fmt.Fprintln(out, "3. Update Discogs / Last.fm connection settings")
		fmt.Fprintln(out, "4. Exit")

		selection, err := promptIndex(reader, out, 4)
		if err != nil {
			return err
		}

		switch selection {
		case 0:
			if err := interactiveSearch(reader, out, cfg); err != nil {
				return err
			}
		case 1:
			cfg, err = interactiveScrobble(reader, out, cfg)
			if err != nil {
				return err
			}
		case 2:
			cfg, err = runConnectionWizard(reader, out, cfg, true)
			if err != nil {
				return err
			}
		case 3:
			fmt.Fprintln(out, "Goodbye.")
			return nil
		}
	}
}

func ensureConnections(reader *bufio.Reader, out io.Writer, cfg config.Config) (config.Config, error) {
	if !cfg.MissingDiscogs() && !cfg.MissingLastFM() {
		fmt.Fprintln(out, "Discogs and Last.fm are already configured.")
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
	fmt.Fprintln(out, "Configure Last.fm app credentials once for this installation, then connect the current user's Last.fm account.")

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
		fmt.Fprintln(out, "Last.fm app credentials are not configured yet. They are stored locally in your user config directory, not in the repository.")
		if err := promptLastFMAppCredentials(reader, out, cfg); err != nil {
			return err
		}
	} else {
		updateAppCredentials, err := promptYesNo(reader, out, "Update the stored Last.fm app credentials?", false)
		if err != nil {
			return err
		}
		if updateAppCredentials {
			if err := promptLastFMAppCredentials(reader, out, cfg); err != nil {
				return err
			}
		} else {
			fmt.Fprintln(out, "Using the stored Last.fm app credentials.")
		}
	}

	sessionKey, err := promptLastFMSessionKey(reader, out, cfg.LastFMAPIKey, cfg.LastFMAPISecret, cfg.LastFMSessionKey)
	if err != nil {
		return err
	}
	cfg.LastFMSessionKey = sessionKey
	return nil
}

func promptLastFMAppCredentials(reader *bufio.Reader, out io.Writer, cfg *config.Config) error {
	apiKey, err := promptSecretValue(reader, out, "Last.fm API key", cfg.LastFMAPIKey)
	if err != nil {
		return err
	}

	apiSecret, err := promptSecretValue(reader, out, "Last.fm API secret", cfg.LastFMAPISecret)
	if err != nil {
		return err
	}

	cfg.LastFMAPIKey = apiKey
	cfg.LastFMAPISecret = apiSecret
	return nil
}

func interactiveSearch(reader *bufio.Reader, out io.Writer, cfg config.Config) error {
	query, err := promptRequiredValue(reader, out, "Search your collection for", "")
	if err != nil {
		return err
	}

	results, err := searchCollection(context.Background(), cfg, query)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		fmt.Fprintln(out, "No matching albums found in your Discogs collection.")
		return nil
	}

	fmt.Fprintln(out)
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

	return nil
}

func interactiveScrobble(reader *bufio.Reader, out io.Writer, cfg config.Config) (config.Config, error) {
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
	if err := scrobbleRelease(context.Background(), cfg, release, startedAt, reader, out); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func promptStartedAt(reader *bufio.Reader, out io.Writer) (time.Time, error) {
	for {
		now := time.Now().Format("2006-01-02 15:04")
		value, err := promptOptionalValue(reader, out, fmt.Sprintf("When did you start playing it? (blank uses now: %s)", now), "")
		if err != nil {
			return time.Time{}, err
		}

		if value == "" {
			return time.Now(), nil
		}

		startedAt, err := parseStartedAt(value)
		if err != nil {
			fmt.Fprintf(out, "Invalid start time: %v\n", err)
			continue
		}
		return startedAt, nil
	}
}

func promptRequiredValue(reader *bufio.Reader, out io.Writer, label, defaultValue string) (string, error) {
	for {
		value, err := promptValue(reader, out, label, defaultValue)
		if err != nil {
			return "", err
		}
		if value != "" {
			return value, nil
		}
		fmt.Fprintln(out, "A value is required.")
	}
}

func promptSecretValue(reader *bufio.Reader, out io.Writer, label, currentValue string) (string, error) {
	for {
		if strings.TrimSpace(currentValue) != "" {
			fmt.Fprintf(out, "%s [stored]: ", label)
		} else {
			fmt.Fprintf(out, "%s: ", label)
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("read input: %w", err)
		}

		value := strings.TrimSpace(line)
		if value == "" {
			if strings.TrimSpace(currentValue) != "" {
				return strings.TrimSpace(currentValue), nil
			}
			fmt.Fprintln(out, "A value is required.")
			continue
		}

		return value, nil
	}
}

func promptLastFMSessionKey(reader *bufio.Reader, out io.Writer, apiKey, apiSecret, currentValue string) (string, error) {
	useGuidedFlow := true
	if strings.TrimSpace(currentValue) != "" {
		keepCurrent, err := promptYesNo(reader, out, "Keep the currently stored Last.fm session key?", true)
		if err != nil {
			return "", err
		}
		if keepCurrent {
			return strings.TrimSpace(currentValue), nil
		}

		useGuidedFlow, err = promptYesNo(reader, out, "Generate a new Last.fm session key through the browser-based auth flow?", true)
		if err != nil {
			return "", err
		}
	} else {
		var err error
		useGuidedFlow, err = promptYesNo(reader, out, "Generate a Last.fm session key now through the browser-based auth flow?", true)
		if err != nil {
			return "", err
		}
	}

	if useGuidedFlow {
		return guideLastFMSessionKey(reader, out, apiKey, apiSecret)
	}

	return promptSecretValue(reader, out, "Last.fm session key", currentValue)
}

func guideLastFMSessionKey(reader *bufio.Reader, out io.Writer, apiKey, apiSecret string) (string, error) {
	client := lastfm.NewClient(apiKey, apiSecret, "")
	token, err := client.GetAuthToken(context.Background())
	if err != nil {
		return "", err
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "Open this URL in your browser and approve access for the application:")
	fmt.Fprintln(out, client.AuthURL(token))
	fmt.Fprintln(out)

	if _, err := promptRequiredValue(reader, out, "Press Enter after approving access in Last.fm", "ready"); err != nil {
		return "", err
	}

	sessionKey, err := client.GetSessionKey(context.Background(), token)
	if err != nil {
		return "", err
	}

	fmt.Fprintln(out, "Last.fm session key obtained successfully.")
	return sessionKey, nil
}

func promptOptionalValue(reader *bufio.Reader, out io.Writer, label, defaultValue string) (string, error) {
	return promptValue(reader, out, label, defaultValue)
}

func promptValue(reader *bufio.Reader, out io.Writer, label, defaultValue string) (string, error) {
	if defaultValue != "" {
		fmt.Fprintf(out, "%s [%s]: ", label, defaultValue)
	} else {
		fmt.Fprintf(out, "%s: ", label)
	}

	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read input: %w", err)
	}

	value := strings.TrimSpace(line)
	if value == "" {
		return strings.TrimSpace(defaultValue), nil
	}

	return value, nil
}

func promptYesNo(reader *bufio.Reader, out io.Writer, label string, defaultYes bool) (bool, error) {
	suffix := "[y/N]"
	if defaultYes {
		suffix = "[Y/n]"
	}

	for {
		fmt.Fprintf(out, "%s %s: ", label, suffix)
		line, err := reader.ReadString('\n')
		if err != nil {
			return false, fmt.Errorf("read input: %w", err)
		}

		value := strings.TrimSpace(strings.ToLower(line))
		if value == "" {
			return defaultYes, nil
		}
		if value == "y" || value == "yes" {
			return true, nil
		}
		if value == "n" || value == "no" {
			return false, nil
		}

		fmt.Fprintln(out, "Please answer yes or no.")
	}
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
