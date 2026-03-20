package lastfm

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"cli-scrobbler/internal/model"
	"cli-scrobbler/internal/scrobble"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestAuthURL(t *testing.T) {
	t.Parallel()

	client := NewClient("api-key", "api-secret", "")
	got := client.AuthURL("token-123")
	want := "https://www.last.fm/api/auth/?api_key=api-key&token=token-123"
	if got != want {
		t.Fatalf("AuthURL() = %q, want %q", got, want)
	}
}

func TestSignatureDeterministic(t *testing.T) {
	t.Parallel()

	params := map[string]string{
		"method":  "auth.getToken",
		"api_key": "abc",
	}

	got := signature(params, "xyz")
	want := "2a379a844d6cae900cab08529c2a183c"
	if got != want {
		t.Fatalf("signature() = %q, want %q", got, want)
	}
}

func TestScrobbleBatchesRequests(t *testing.T) {
	t.Parallel()

	client := NewClient("api-key", "api-secret", "session-key")

	requestCount := 0
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestCount++

			if req.Method != http.MethodPost {
				t.Fatalf("request method = %q, want %q", req.Method, http.MethodPost)
			}

			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("ReadAll(request body) error = %v", err)
			}

			values, err := url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("ParseQuery(request body) error = %v", err)
			}

			if values.Get("method") != "track.scrobble" {
				t.Fatalf("method param = %q, want %q", values.Get("method"), "track.scrobble")
			}
			if values.Get("api_key") != "api-key" {
				t.Fatalf("api_key param = %q, want %q", values.Get("api_key"), "api-key")
			}
			if values.Get("sk") != "session-key" {
				t.Fatalf("sk param = %q, want %q", values.Get("sk"), "session-key")
			}

			trackFieldCount := 0
			for key := range values {
				if len(key) >= 6 && key[:6] == "track[" {
					trackFieldCount++
				}
			}
			if trackFieldCount == 0 {
				t.Fatalf("request contained no track[*] fields")
			}
			if trackFieldCount > maxBatchSize {
				t.Fatalf("request contained %d tracks, exceeds maxBatchSize %d", trackFieldCount, maxBatchSize)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"error":0}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	startedAt := time.Date(2026, 3, 20, 20, 0, 0, 0, time.UTC)
	tracks := make([]scrobble.ScheduledTrack, 0, 120)
	for i := 0; i < 120; i++ {
		tracks = append(tracks, scrobble.ScheduledTrack{
			Track: model.Track{
				Artist:   "Artist",
				Title:    "Track " + strconv.Itoa(i+1),
				Duration: 3 * time.Minute,
			},
			StartedAt: startedAt.Add(time.Duration(i) * 3 * time.Minute),
		})
	}

	err := client.Scrobble(context.Background(), tracks, "Album")
	if err != nil {
		t.Fatalf("Scrobble() error = %v", err)
	}

	if requestCount != 3 {
		t.Fatalf("Scrobble() request count = %d, want %d", requestCount, 3)
	}
}

func TestScrobbleRequiresTracks(t *testing.T) {
	t.Parallel()

	client := NewClient("api-key", "api-secret", "session-key")
	err := client.Scrobble(context.Background(), nil, "Album")
	if err == nil {
		t.Fatalf("Scrobble() error = nil, want non-nil")
	}
	if err.Error() != "no tracks to scrobble" {
		t.Fatalf("Scrobble() error = %q, want %q", err.Error(), "no tracks to scrobble")
	}
}
