package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePathsOutsideRepo(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	workingDir := t.TempDir()
	restore := chdirForTest(t, workingDir)
	defer restore()

	paths, err := ResolvePaths()
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}

	baseDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir() error = %v", err)
	}

	wantDir := filepath.Join(baseDir, AppName)
	if paths.Dir != wantDir {
		t.Fatalf("ResolvePaths().Dir = %q, want %q", paths.Dir, wantDir)
	}
	if paths.ConfigFile != filepath.Join(wantDir, configFileName) {
		t.Fatalf("ResolvePaths().ConfigFile = %q, want %q", paths.ConfigFile, filepath.Join(wantDir, configFileName))
	}
	if paths.DurationCache != filepath.Join(wantDir, durationCacheFileName) {
		t.Fatalf("ResolvePaths().DurationCache = %q, want %q", paths.DurationCache, filepath.Join(wantDir, durationCacheFileName))
	}
	if paths.BuildConfigFile != "" {
		t.Fatalf("ResolvePaths().BuildConfigFile = %q, want empty outside repo", paths.BuildConfigFile)
	}
}

func TestLoadOutsideRepoUsesUserConfigAndEnv(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv(envLastFMAPIKey, "env-key")
	t.Setenv(envLastFMAPISecret, "env-secret")

	workingDir := t.TempDir()
	restore := chdirForTest(t, workingDir)
	defer restore()

	baseDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir() error = %v", err)
	}

	userDir := filepath.Join(baseDir, AppName)
	if err := os.MkdirAll(userDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	userConfig := []byte("{\n  \"discogs_token\": \"discogs-token\",\n  \"lastfm_session_key\": \"session-key\"\n}\n")
	if err := os.WriteFile(filepath.Join(userDir, configFileName), userConfig, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.DiscogsToken != "discogs-token" {
		t.Fatalf("Load().DiscogsToken = %q, want %q", cfg.DiscogsToken, "discogs-token")
	}
	if cfg.LastFMSessionKey != "session-key" {
		t.Fatalf("Load().LastFMSessionKey = %q, want %q", cfg.LastFMSessionKey, "session-key")
	}
	if cfg.LastFMAPIKey != "env-key" {
		t.Fatalf("Load().LastFMAPIKey = %q, want %q", cfg.LastFMAPIKey, "env-key")
	}
	if cfg.LastFMAPISecret != "env-secret" {
		t.Fatalf("Load().LastFMAPISecret = %q, want %q", cfg.LastFMAPISecret, "env-secret")
	}
}

func chdirForTest(t *testing.T, dir string) func() {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%q) error = %v", dir, err)
	}

	return func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore working directory to %q: %v", wd, err)
		}
	}
}
