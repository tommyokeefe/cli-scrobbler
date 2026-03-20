package app

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"cli-scrobbler/internal/config"
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
