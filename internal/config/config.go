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
	Dir             string
	ConfigFile      string
	BuildConfigFile string
	DurationCache   string
}

func ResolvePaths() (Paths, error) {
	repoRoot, err := resolveRepoRoot()
	if err != nil {
		return Paths{}, err
	}

	baseDir, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve user config dir: %w", err)
	}

	userDir := filepath.Join(baseDir, AppName)

	return Paths{
		Dir:             userDir,
		ConfigFile:      filepath.Join(userDir, configFileName),
		BuildConfigFile: filepath.Join(repoRoot, configFileName),
		DurationCache:   filepath.Join(userDir, durationCacheFileName),
	}, nil
}

func resolveRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}

	start := dir
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}

		dir = parent
	}

	return "", fmt.Errorf("resolve repository root: go.mod not found from working directory %q", start)
}

func Load() (Config, error) {
	paths, err := ResolvePaths()
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		DiscogsUserAgent: defaultDiscogsAgent,
	}

	userCfg, userCfgExists, err := readConfig(paths.ConfigFile)
	if err != nil {
		return Config{}, fmt.Errorf("read user config: %w", err)
	}
	if userCfgExists {
		cfg.DiscogsToken = userCfg.DiscogsToken
		cfg.DiscogsUsername = userCfg.DiscogsUsername
		if userCfg.DiscogsUserAgent != "" {
			cfg.DiscogsUserAgent = userCfg.DiscogsUserAgent
		}
		cfg.LastFMSessionKey = userCfg.LastFMSessionKey
	}

	buildCfg, buildCfgExists, err := readConfig(paths.BuildConfigFile)
	if err != nil {
		return Config{}, fmt.Errorf("read build config: %w", err)
	}
	if buildCfgExists {
		cfg.LastFMAPIKey = buildCfg.LastFMAPIKey
		cfg.LastFMAPISecret = buildCfg.LastFMAPISecret
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

	userCfg := Config{
		DiscogsToken:     cfg.DiscogsToken,
		DiscogsUsername:  cfg.DiscogsUsername,
		DiscogsUserAgent: cfg.DiscogsUserAgent,
		LastFMSessionKey: cfg.LastFMSessionKey,
	}

	data, err := json.MarshalIndent(userCfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(paths.ConfigFile, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

func readConfig(path string) (Config, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, false, nil
		}
		return Config{}, false, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, false, fmt.Errorf("parse config at %s: %w", path, err)
	}

	return cfg, true, nil
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
