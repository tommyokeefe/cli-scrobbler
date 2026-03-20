package app

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"cli-scrobbler/internal/config"
	"cli-scrobbler/internal/discogs"
	"cli-scrobbler/internal/lastfm"
)

func TestRunAuthDiscogsSavesConfig(t *testing.T) {
	isolateAppConfigHome(t)
	restore := chdirForAppTest(t, t.TempDir())
	defer restore()

	var out bytes.Buffer
	err := runAuth([]string{"discogs", "--token", "discogs-token", "--username", "discogs-user", "--user-agent", "agent/2.0"}, strings.NewReader(""), &out)
	if err != nil {
		t.Fatalf("runAuth(discogs) error = %v", err)
	}
	if !strings.Contains(out.String(), "Saved Discogs credentials.") {
		t.Fatalf("runAuth(discogs) output = %q, want saved message", out.String())
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.DiscogsToken != "discogs-token" || cfg.DiscogsUsername != "discogs-user" || cfg.DiscogsUserAgent != "agent/2.0" {
		t.Fatalf("saved config = %#v", cfg)
	}
}

func TestRunAuthLastFMSavesProvidedSessionKey(t *testing.T) {
	isolateAppConfigHome(t)
	t.Setenv("SCROBBLER_LASTFM_API_KEY", "api-key")
	t.Setenv("SCROBBLER_LASTFM_API_SECRET", "api-secret")
	restore := chdirForAppTest(t, t.TempDir())
	defer restore()

	var out bytes.Buffer
	err := runAuth([]string{"lastfm", "--session-key", "session-123"}, strings.NewReader(""), &out)
	if err != nil {
		t.Fatalf("runAuth(lastfm) error = %v", err)
	}
	if !strings.Contains(out.String(), "Saved Last.fm session.") {
		t.Fatalf("runAuth(lastfm) output = %q, want saved message", out.String())
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.LastFMSessionKey != "session-123" {
		t.Fatalf("saved session key = %q, want %q", cfg.LastFMSessionKey, "session-123")
	}
}

func TestRunAuthLastFMRequiresCredentials(t *testing.T) {
	isolateAppConfigHome(t)
	restore := chdirForAppTest(t, t.TempDir())
	defer restore()

	err := runAuth([]string{"lastfm", "--session-key", "session-123"}, strings.NewReader(""), io.Discard)
	if err == nil {
		t.Fatal("runAuth(lastfm) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "missing Last.fm app credentials") {
		t.Fatalf("runAuth(lastfm) error = %q, want missing credentials", err.Error())
	}
}

func TestRunSearchDryRunUsesDiscogsResults(t *testing.T) {
	isolateAppConfigHome(t)
	restore := chdirForAppTest(t, t.TempDir())
	defer restore()
	writeUserConfig(t, "{\n  \"discogs_token\": \"discogs-token\",\n  \"discogs_username\": \"discogs-user\",\n  \"discogs_user_agent\": \"agent/1.0\"\n}\n")

	restoreClients := stubAppClients(t, func(token, userAgent string) *discogs.Client {
		return discogs.NewClientWithHTTPClient(token, userAgent, &http.Client{
			Transport: appRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch {
				case strings.HasPrefix(req.URL.Path, "/users/discogs-user/collection/folders/0/releases"):
					return jsonResponse(`{
						"pagination":{"page":1,"pages":1},
						"releases":[
							{"basic_information":{"id":1,"title":"Dummy","year":1994,"artists":[{"name":"Portishead"}],"formats":[{"name":"LP"}]}},
							{"basic_information":{"id":2,"title":"Dummy","year":2017,"artists":[{"name":"Portishead"}],"formats":[{"name":"LP"},{"name":"Reissue"}]}}
						]
					}`), nil
				case req.URL.Path == "/releases/2":
					return jsonResponse(`{
						"id":2,
						"title":"Dummy",
						"year":2017,
						"artists":[{"name":"Portishead"}],
						"tracklist":[
							{"position":"1","title":"Mysterons","duration":"5:02","type_":"track"},
							{"position":"2","title":"Sour Times","duration":"4:11","type_":"track"}
						]
					}`), nil
				default:
					t.Fatalf("unexpected Discogs request: %s", req.URL.String())
					return nil, nil
				}
			}),
		})
	}, nil)
	defer restoreClients()

	var out bytes.Buffer
	err := runSearch([]string{"--no-scrobble", "dummy"}, strings.NewReader("1\n2026-03-20 20:00\n"), &out)
	if err != nil {
		t.Fatalf("runSearch() error = %v", err)
	}
	if !strings.Contains(out.String(), "Select an album from your Discogs collection:") {
		t.Fatalf("runSearch() output missing selection prompt: %q", out.String())
	}
	if !strings.Contains(out.String(), "Dry run") {
		t.Fatalf("runSearch() output missing dry-run message: %q", out.String())
	}
}

func TestRunSearchNoMatchesPrintsFriendlyMessage(t *testing.T) {
	isolateAppConfigHome(t)
	restore := chdirForAppTest(t, t.TempDir())
	defer restore()
	writeUserConfig(t, "{\n  \"discogs_token\": \"discogs-token\",\n  \"discogs_username\": \"discogs-user\",\n  \"discogs_user_agent\": \"agent/1.0\"\n}\n")

	restoreClients := stubAppClients(t, func(token, userAgent string) *discogs.Client {
		return discogs.NewClientWithHTTPClient(token, userAgent, &http.Client{
			Transport: appRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if strings.HasPrefix(req.URL.Path, "/users/discogs-user/collection/folders/0/releases") {
					return jsonResponse(`{"pagination":{"page":1,"pages":1},"releases":[]}`), nil
				}
				t.Fatalf("unexpected Discogs request: %s", req.URL.String())
				return nil, nil
			}),
		})
	}, nil)
	defer restoreClients()

	var out bytes.Buffer
	err := runSearch([]string{"--no-scrobble", "missing"}, strings.NewReader(""), &out)
	if err != nil {
		t.Fatalf("runSearch() error = %v", err)
	}
	if !strings.Contains(out.String(), "No matching albums found") {
		t.Fatalf("runSearch() output = %q, want no-match message", out.String())
	}
}

func TestRunScrobbleSendsTracksToLastFM(t *testing.T) {
	isolateAppConfigHome(t)
	t.Setenv("SCROBBLER_LASTFM_API_KEY", "api-key")
	t.Setenv("SCROBBLER_LASTFM_API_SECRET", "api-secret")
	t.Setenv("SCROBBLER_LASTFM_SESSION_KEY", "session-key")
	restore := chdirForAppTest(t, t.TempDir())
	defer restore()
	writeUserConfig(t, "{\n  \"discogs_token\": \"discogs-token\",\n  \"discogs_username\": \"discogs-user\",\n  \"discogs_user_agent\": \"agent/1.0\"\n}\n")

	restoreClients := stubAppClients(t, func(token, userAgent string) *discogs.Client {
		return discogs.NewClientWithHTTPClient(token, userAgent, &http.Client{
			Transport: appRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch {
				case strings.HasPrefix(req.URL.Path, "/users/discogs-user/collection/folders/0/releases"):
					return jsonResponse(`{
						"pagination":{"page":1,"pages":1},
						"releases":[
							{"basic_information":{"id":7,"title":"Dummy","year":1994,"artists":[{"name":"Portishead"}],"formats":[{"name":"LP"}]}}
						]
					}`), nil
				case req.URL.Path == "/releases/7":
					return jsonResponse(`{
						"id":7,
						"title":"Dummy",
						"year":1994,
						"artists":[{"name":"Portishead"}],
						"tracklist":[
							{"position":"1","title":"Mysterons","duration":"5:02","type_":"track"},
							{"position":"2","title":"Sour Times","duration":"4:11","type_":"track"}
						]
					}`), nil
				default:
					t.Fatalf("unexpected Discogs request: %s", req.URL.String())
					return nil, nil
				}
			}),
		})
	}, func(apiKey, apiSecret, sessionKey string) *lastfm.Client {
		return lastfm.NewClientWithHTTPClient(apiKey, apiSecret, sessionKey, &http.Client{
			Transport: appRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("ReadAll(request body) error = %v", err)
				}
				values, err := url.ParseQuery(string(body))
				if err != nil {
					t.Fatalf("ParseQuery() error = %v", err)
				}
				if values.Get("method") != "track.scrobble" {
					t.Fatalf("method = %q, want %q", values.Get("method"), "track.scrobble")
				}
				if values.Get("track[0]") != "Mysterons" || values.Get("track[1]") != "Sour Times" {
					t.Fatalf("unexpected scrobble payload: %v", values)
				}
				return jsonResponse(`{"error":0}`), nil
			}),
		})
	})
	defer restoreClients()

	var out bytes.Buffer
	err := runScrobble([]string{"--started-at", "2026-03-20T20:00:00Z", "dummy"}, strings.NewReader(""), &out)
	if err != nil {
		t.Fatalf("runScrobble() error = %v", err)
	}
	if !strings.Contains(out.String(), "Scrobbled Portishead - Dummy") {
		t.Fatalf("runScrobble() output = %q, want timeline", out.String())
	}
}

func stubAppClients(t *testing.T, discogsCtor func(string, string) *discogs.Client, lastFMCtor func(string, string, string) *lastfm.Client) func() {
	t.Helper()

	oldDiscogs := newDiscogsClient
	oldLastFM := newLastFMClient
	if discogsCtor != nil {
		newDiscogsClient = discogsCtor
	}
	if lastFMCtor != nil {
		newLastFMClient = lastFMCtor
	}

	return func() {
		newDiscogsClient = oldDiscogs
		newLastFMClient = oldLastFM
	}
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}
