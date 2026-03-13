package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	AppName               = "cli-scrobbler"
	envDiscogsToken       = "SCROBBLER_DISCOGS_TOKEN"
	envDiscogsUsername    = "SCROBBLER_DISCOGS_USERNAME"
	envLastFMAPIKey       = "SCROBBLER_LASTFM_API_KEY"
	envLastFMAPISecret    = "SCROBBLER_LASTFM_API_SECRET"
	envLastFMSessionKey   = "SCROBBLER_LASTFM_SESSION_KEY"
	defaultDiscogsAgent   = "cli-scrobbler/0.1"
	configFileName        = "config.json"
	durationCacheFileName = "durations.json"
)

type Config struct {
	DiscogsToken     string `json:"discogs_token,omitempty"`
	DiscogsUsername  string `json:"discogs_username,omitempty"`
	DiscogsUserAgent string `json:"discogs_user_agent,omitempty"`
	LastFMAPIKey     string `json:"lastfm_api_key,omitempty"`
	LastFMAPISecret  string `json:"lastfm_api_secret,omitempty"`
	LastFMSessionKey string `json:"lastfm_session_key,omitempty"`
}

type Paths struct {
	Dir           string
	ConfigFile    string
	DurationCache string
}

func ResolvePaths() (Paths, error) {
	baseDir, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve user config dir: %w", err)
	}

	dir := filepath.Join(baseDir, AppName)
	return Paths{
		Dir:           dir,
		ConfigFile:    filepath.Join(dir, configFileName),
		DurationCache: filepath.Join(dir, durationCacheFileName),
	}, nil
}

func Load() (Config, error) {
	paths, err := ResolvePaths()
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		DiscogsUserAgent: defaultDiscogsAgent,
	}

	data, err := os.ReadFile(paths.ConfigFile)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Config{}, fmt.Errorf("read config: %w", err)
		}
		cfg.applyEnv()
		return cfg, nil
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}

	if cfg.DiscogsUserAgent == "" {
		cfg.DiscogsUserAgent = defaultDiscogsAgent
	}

	cfg.applyEnv()
	return cfg, nil
}

func Save(cfg Config) error {
	paths, err := ResolvePaths()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	if cfg.DiscogsUserAgent == "" {
		cfg.DiscogsUserAgent = defaultDiscogsAgent
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(paths.ConfigFile, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

func (c Config) MissingDiscogs() bool {
	return c.DiscogsToken == ""
}

func (c Config) MissingLastFMAppCredentials() bool {
	return c.LastFMAPIKey == "" || c.LastFMAPISecret == ""
}

func (c Config) MissingLastFMSession() bool {
	return c.LastFMSessionKey == ""
}

func (c Config) MissingLastFM() bool {
	return c.MissingLastFMAppCredentials() || c.MissingLastFMSession()
}

func (c *Config) applyEnv() {
	if value := os.Getenv(envDiscogsToken); value != "" {
		c.DiscogsToken = value
	}
	if value := os.Getenv(envDiscogsUsername); value != "" {
		c.DiscogsUsername = value
	}
	if value := os.Getenv(envLastFMAPIKey); value != "" {
		c.LastFMAPIKey = value
	}
	if value := os.Getenv(envLastFMAPISecret); value != "" {
		c.LastFMAPISecret = value
	}
	if value := os.Getenv(envLastFMSessionKey); value != "" {
		c.LastFMSessionKey = value
	}
}
