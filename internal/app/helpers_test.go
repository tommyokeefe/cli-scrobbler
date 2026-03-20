package app

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cli-scrobbler/internal/cache"
	"cli-scrobbler/internal/config"
	"cli-scrobbler/internal/discogs"
	"cli-scrobbler/internal/model"
)

type appRoundTripFunc func(*http.Request) (*http.Response, error)

func (f appRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestPromptIndexRetriesUntilValid(t *testing.T) {
	t.Parallel()

	reader := bufio.NewReader(strings.NewReader("0\nabc\n2\n"))
	var out bytes.Buffer

	got, err := promptIndex(reader, &out, 3)
	if err != nil {
		t.Fatalf("promptIndex() error = %v", err)
	}
	if got != 1 {
		t.Fatalf("promptIndex() = %d, want %d", got, 1)
	}
	if strings.Count(out.String(), "Please enter a valid selection.") != 2 {
		t.Fatalf("promptIndex() output = %q, want two validation messages", out.String())
	}
}

func TestPromptDurationRetriesUntilValid(t *testing.T) {
	t.Parallel()

	reader := bufio.NewReader(strings.NewReader("-1\n4:30\n"))
	var out bytes.Buffer

	got, err := promptDuration(reader, &out, model.Track{Position: "A1", Title: "Intro"})
	if err != nil {
		t.Fatalf("promptDuration() error = %v", err)
	}
	if got != 4*time.Minute+30*time.Second {
		t.Fatalf("promptDuration() = %v, want %v", got, 4*time.Minute+30*time.Second)
	}
	if !strings.Contains(out.String(), "Invalid duration:") {
		t.Fatalf("promptDuration() output missing validation message: %q", out.String())
	}
}

func TestPromptSecretValueKeepsStoredValue(t *testing.T) {
	t.Parallel()

	reader := bufio.NewReader(strings.NewReader("\n"))
	var out bytes.Buffer

	got, err := promptSecretValue(reader, &out, "Discogs token", "stored-token")
	if err != nil {
		t.Fatalf("promptSecretValue() error = %v", err)
	}
	if got != "stored-token" {
		t.Fatalf("promptSecretValue() = %q, want %q", got, "stored-token")
	}
}

func TestPromptYesNoRetriesUntilValid(t *testing.T) {
	t.Parallel()

	reader := bufio.NewReader(strings.NewReader("maybe\nn\n"))
	var out bytes.Buffer

	got, err := promptYesNo(reader, &out, "Continue?", true)
	if err != nil {
		t.Fatalf("promptYesNo() error = %v", err)
	}
	if got {
		t.Fatalf("promptYesNo() = %v, want false", got)
	}
	if !strings.Contains(out.String(), "Please answer yes or no.") {
		t.Fatalf("promptYesNo() output missing retry prompt: %q", out.String())
	}
}

func TestEnsureConnectionsReportsConfiguredState(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		DiscogsToken:     "discogs-token",
		DiscogsUserAgent: "agent/1.0",
		LastFMAPIKey:     "api-key",
		LastFMAPISecret:  "api-secret",
		LastFMSessionKey: "session-key",
	}
	var out bytes.Buffer

	got, err := ensureConnections(bufio.NewReader(strings.NewReader("")), &out, cfg)
	if err != nil {
		t.Fatalf("ensureConnections() error = %v", err)
	}
	if got != cfg {
		t.Fatalf("ensureConnections() = %#v, want %#v", got, cfg)
	}
	if !strings.Contains(out.String(), "configured") {
		t.Fatalf("ensureConnections() output missing confirmation: %q", out.String())
	}
}

func TestPromptDiscogsConfigUpdatesFields(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		DiscogsToken:     "old-token",
		DiscogsUsername:  "old-user",
		DiscogsUserAgent: "old-agent",
	}
	reader := bufio.NewReader(strings.NewReader("new-token\nnew-user\nnew-agent\n"))

	if err := promptDiscogsConfig(reader, io.Discard, &cfg); err != nil {
		t.Fatalf("promptDiscogsConfig() error = %v", err)
	}
	if cfg.DiscogsToken != "new-token" || cfg.DiscogsUsername != "new-user" || cfg.DiscogsUserAgent != "new-agent" {
		t.Fatalf("promptDiscogsConfig() updated cfg = %#v", cfg)
	}
}

func TestPromptLastFMConfigRequiresCredentials(t *testing.T) {
	t.Parallel()

	cfg := config.Config{}
	err := promptLastFMConfig(bufio.NewReader(strings.NewReader("")), io.Discard, &cfg)
	if err == nil {
		t.Fatal("promptLastFMConfig() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "missing Last.fm app credentials") {
		t.Fatalf("promptLastFMConfig() error = %q, want missing credentials", err.Error())
	}
}

func TestPromptLastFMConfigKeepsStoredSessionKey(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		LastFMAPIKey:     "api-key",
		LastFMAPISecret:  "api-secret",
		LastFMSessionKey: "stored-session",
	}

	err := promptLastFMConfig(bufio.NewReader(strings.NewReader("\n")), io.Discard, &cfg)
	if err != nil {
		t.Fatalf("promptLastFMConfig() error = %v", err)
	}
	if cfg.LastFMSessionKey != "stored-session" {
		t.Fatalf("promptLastFMConfig() session key = %q, want %q", cfg.LastFMSessionKey, "stored-session")
	}
}

func TestHydrateDurationsUsesCacheAndPromptsForMissingTracks(t *testing.T) {
	t.Parallel()

	store := &cache.DurationStore{Entries: map[string]int64{}}
	release := model.Album{
		ReleaseID: 42,
		Tracks: []model.Track{
			{Position: "A1", Title: "Cached", Artist: "Artist"},
			{Position: "A2", Title: "Prompted", Artist: "Artist"},
		},
	}
	store.Put(release.ReleaseID, release.Tracks[0], 90*time.Second)

	reader := bufio.NewReader(strings.NewReader("200\n"))
	var out bytes.Buffer

	got, asked, err := hydrateDurations(release, store, reader, &out)
	if err != nil {
		t.Fatalf("hydrateDurations() error = %v", err)
	}
	if !asked {
		t.Fatal("hydrateDurations() asked = false, want true")
	}
	if got.Tracks[0].Duration != 90*time.Second {
		t.Fatalf("hydrateDurations() cached duration = %v, want %v", got.Tracks[0].Duration, 90*time.Second)
	}
	if got.Tracks[1].Duration != 200*time.Second {
		t.Fatalf("hydrateDurations() prompted duration = %v, want %v", got.Tracks[1].Duration, 200*time.Second)
	}
	if cached, ok := store.Lookup(release.ReleaseID, got.Tracks[1]); !ok || cached != 200*time.Second {
		t.Fatalf("hydrateDurations() failed to persist prompted duration, got %v ok=%v", cached, ok)
	}
}

func TestResolveDiscogsUsernameUsesConfiguredValue(t *testing.T) {
	t.Parallel()

	client := discogs.NewClient("discogs-token", "agent/1.0")
	got, err := resolveDiscogsUsername(context.Background(), client, config.Config{DiscogsUsername: "configured-user"})
	if err != nil {
		t.Fatalf("resolveDiscogsUsername() error = %v", err)
	}
	if got != "configured-user" {
		t.Fatalf("resolveDiscogsUsername() = %q, want %q", got, "configured-user")
	}
}

func TestResolveDiscogsUsernameWrapsIdentityError(t *testing.T) {
	t.Parallel()

	client := discogs.NewClientWithHTTPClient("discogs-token", "agent/1.0", &http.Client{
		Transport: appRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Status:     "401 Unauthorized",
				Body:       io.NopCloser(strings.NewReader(`{"message":"nope"}`)),
				Header:     make(http.Header),
			}, nil
		}),
	})

	_, err := resolveDiscogsUsername(context.Background(), client, config.Config{})
	if err == nil {
		t.Fatal("resolveDiscogsUsername() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "resolve Discogs username:") {
		t.Fatalf("resolveDiscogsUsername() error = %q, want wrapped context", err.Error())
	}
}

func TestScrobbleReleaseDryRunSavesPromptedDurations(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	restore := chdirForAppTest(t, t.TempDir())
	defer restore()

	release := model.Album{
		ReleaseID: 99,
		Artist:    "Portishead",
		Title:     "Dummy",
		Year:      1994,
		Tracks: []model.Track{
			{Position: "1", Title: "Mysterons", Artist: "Portishead", Duration: 0},
		},
	}

	var out bytes.Buffer
	err := scrobbleRelease(
		context.Background(),
		config.Config{},
		release,
		time.Date(2026, 3, 20, 20, 0, 0, 0, time.UTC),
		bufio.NewReader(strings.NewReader("300\n")),
		&out,
		true,
	)
	if err != nil {
		t.Fatalf("scrobbleRelease() error = %v", err)
	}
	if !strings.Contains(out.String(), "Dry run") {
		t.Fatalf("scrobbleRelease() output missing dry-run note: %q", out.String())
	}

	paths, err := config.ResolvePaths()
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	data, err := os.ReadFile(paths.DurationCache)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", paths.DurationCache, err)
	}
	if !strings.Contains(string(data), `"99|1|mysterons|portishead": 300`) {
		t.Fatalf("duration cache missing prompted track duration: %s", string(data))
	}
}

func chdirForAppTest(t *testing.T, dir string) func() {
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

func writeUserConfig(t *testing.T, cfg string) {
	t.Helper()

	baseDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir() error = %v", err)
	}
	configDir := filepath.Join(baseDir, config.AppName)
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", configDir, err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("WriteFile(config.json) error = %v", err)
	}
}
