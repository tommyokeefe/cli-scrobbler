package app

import (
	"bufio"
	"fmt"
	"io"

	"cli-scrobbler/internal/config"
)

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
