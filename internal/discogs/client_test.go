package discogs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestParseDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  time.Duration
	}{
		{input: "", want: 0},
		{input: "04:32", want: 4*time.Minute + 32*time.Second},
		{input: "1:02:03", want: time.Hour + 2*time.Minute + 3*time.Second},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			got, err := ParseDuration(tc.input)
			if err != nil {
				t.Fatalf("ParseDuration() error = %v", err)
			}

			if got != tc.want {
				t.Fatalf("ParseDuration() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDiscogsIntUnmarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  int
	}{
		{
			name:  "string year",
			input: `{"id":1,"title":"Album","year":"2024","country":"US","format":["LP"],"type":"release"}`,
			want:  2024,
		},
		{
			name:  "numeric year",
			input: `{"id":1,"title":"Album","year":1991,"country":"US","format":["LP"],"type":"release"}`,
			want:  1991,
		},
		{
			name:  "empty year",
			input: `{"id":1,"title":"Album","year":"","country":"US","format":["LP"],"type":"release"}`,
			want:  0,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got struct {
				Year discogsInt `json:"year"`
			}
			if err := json.Unmarshal([]byte(tc.input), &got); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}

			if int(got.Year) != tc.want {
				t.Fatalf("year = %d, want %d", got.Year, tc.want)
			}
		})
	}
}

func TestSearchCollection(t *testing.T) {
	t.Parallel()

	releases := []CollectionRelease{
		{ReleaseID: 1, Artist: "Blood Incantation", Title: "Absolute Elsewhere", Year: 2024},
		{ReleaseID: 2, Artist: "Incantation", Title: "Onward to Golgotha", Year: 1992},
		{ReleaseID: 3, Artist: "Ulcerate", Title: "Stare Into Death and Be Still", Year: 2020},
	}

	got := SearchCollection("blood inc absolute", releases)
	if len(got) == 0 {
		t.Fatal("SearchCollection() returned no matches")
	}

	if got[0].ReleaseID != 1 {
		t.Fatalf("top result release ID = %d, want 1", got[0].ReleaseID)
	}
}

func TestIdentitySetsHeadersAndReturnsUsername(t *testing.T) {
	t.Parallel()

	client := NewClient("discogs-token", "agent/1.0")
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/oauth/identity" {
				t.Fatalf("request path = %q, want %q", req.URL.Path, "/oauth/identity")
			}
			if req.Header.Get("Authorization") != "Discogs token=discogs-token" {
				t.Fatalf("Authorization header = %q", req.Header.Get("Authorization"))
			}
			if req.Header.Get("User-Agent") != "agent/1.0" {
				t.Fatalf("User-Agent header = %q", req.Header.Get("User-Agent"))
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader(`{"username":"discogs-user"}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	got, err := client.Identity(context.Background())
	if err != nil {
		t.Fatalf("Identity() error = %v", err)
	}
	if got != "discogs-user" {
		t.Fatalf("Identity() = %q, want %q", got, "discogs-user")
	}
}

func TestIdentityRequiresUsernameInResponse(t *testing.T) {
	t.Parallel()

	client := NewClient("discogs-token", "agent/1.0")
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader(`{"username":"   "}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	_, err := client.Identity(context.Background())
	if err == nil {
		t.Fatal("Identity() error = nil, want error")
	}
	if err.Error() != "Discogs identity response did not include a username" {
		t.Fatalf("Identity() error = %q, want username error", err.Error())
	}
}

func TestCollectionReleasesPaginatesAndNormalizesValues(t *testing.T) {
	t.Parallel()

	pageRequests := 0
	client := NewClient("discogs-token", "agent/1.0")
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			pageRequests++
			values, err := url.ParseQuery(req.URL.RawQuery)
			if err != nil {
				t.Fatalf("ParseQuery() error = %v", err)
			}
			switch values.Get("page") {
			case "1":
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body: io.NopCloser(strings.NewReader(`{
						"pagination":{"page":1,"pages":2},
						"releases":[
							{"basic_information":{"id":1,"title":"Dummy","year":"1994","artists":[{"name":"Portishead (2)"}],"formats":[{"name":"LP"},{"name":"  "}]}}
						]
					}`)),
					Header: make(http.Header),
				}, nil
			case "2":
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body: io.NopCloser(strings.NewReader(`{
						"pagination":{"page":2,"pages":2},
						"releases":[
							{"basic_information":{"id":2,"title":"Third","year":2008,"artists":[{"name":"Portishead"}],"formats":[{"name":"CD"}]}}
						]
					}`)),
					Header: make(http.Header),
				}, nil
			default:
				t.Fatalf("unexpected page %q", values.Get("page"))
				return nil, nil
			}
		}),
	}

	got, err := client.CollectionReleases(context.Background(), "discogs-user")
	if err != nil {
		t.Fatalf("CollectionReleases() error = %v", err)
	}
	if pageRequests != 2 {
		t.Fatalf("CollectionReleases() request count = %d, want %d", pageRequests, 2)
	}
	if len(got) != 2 {
		t.Fatalf("CollectionReleases() len = %d, want %d", len(got), 2)
	}
	if got[0].Artist != "Portishead" {
		t.Fatalf("CollectionReleases()[0].Artist = %q, want %q", got[0].Artist, "Portishead")
	}
	if len(got[0].Formats) != 1 || got[0].Formats[0] != "LP" {
		t.Fatalf("CollectionReleases()[0].Formats = %#v, want %#v", got[0].Formats, []string{"LP"})
	}
}

func TestGetReleaseSkipsHeadingsAndFallsBackToAlbumArtist(t *testing.T) {
	t.Parallel()

	client := NewClient("discogs-token", "agent/1.0")
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/releases/7" {
				t.Fatalf("request path = %q, want %q", req.URL.Path, "/releases/7")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body: io.NopCloser(strings.NewReader(`{
					"id":7,
					"title":"Dummy",
					"year":"1994",
					"artists":[{"name":"Portishead (2)"}],
					"tracklist":[
						{"position":"","title":"Side A","duration":"","type_":"heading"},
						{"position":"A1","title":"Mysterons","duration":"5:02","type_":"track","artists":[]},
						{"position":"A2","title":"Sour Times","duration":"4:11","type_":"track","artists":[{"name":"Beth Gibbons (2)"}]}
					]
				}`)),
				Header: make(http.Header),
			}, nil
		}),
	}

	got, err := client.GetRelease(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetRelease() error = %v", err)
	}
	if got.Artist != "Portishead" {
		t.Fatalf("GetRelease().Artist = %q, want %q", got.Artist, "Portishead")
	}
	if len(got.Tracks) != 2 {
		t.Fatalf("GetRelease().Tracks len = %d, want %d", len(got.Tracks), 2)
	}
	if got.Tracks[0].Artist != "Portishead" {
		t.Fatalf("GetRelease().Tracks[0].Artist = %q, want album artist fallback", got.Tracks[0].Artist)
	}
	if got.Tracks[1].Artist != "Beth Gibbons" {
		t.Fatalf("GetRelease().Tracks[1].Artist = %q, want stripped artist suffix", got.Tracks[1].Artist)
	}
}

func TestGetReleaseReturnsDurationParseError(t *testing.T) {
	t.Parallel()

	client := NewClient("discogs-token", "agent/1.0")
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body: io.NopCloser(strings.NewReader(`{
					"id":7,
					"title":"Dummy",
					"year":1994,
					"artists":[{"name":"Portishead"}],
					"tracklist":[
						{"position":"A1","title":"Mysterons","duration":"bad","type_":"track"}
					]
				}`)),
				Header: make(http.Header),
			}, nil
		}),
	}

	_, err := client.GetRelease(context.Background(), 7)
	if err == nil {
		t.Fatal("GetRelease() error = nil, want error")
	}
	if !strings.Contains(err.Error(), `parse track duration for "Mysterons"`) {
		t.Fatalf("GetRelease() error = %q, want wrapped duration error", err.Error())
	}
}

func TestDoWrapsTransportAndDecodeErrors(t *testing.T) {
	t.Parallel()

	transportErr := errors.New("boom")
	client := NewClient("discogs-token", "agent/1.0")
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, transportErr
		}),
	}

	req, err := http.NewRequest(http.MethodGet, baseURL+"/oauth/identity", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	if err := client.do(req, &identityResponse{}); !errors.Is(err, transportErr) {
		t.Fatalf("client.do() error = %v, want wrapped transport error", err)
	}

	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader("{")),
				Header:     make(http.Header),
			}, nil
		}),
	}
	if err := client.do(req, &identityResponse{}); err == nil || !strings.Contains(err.Error(), "decode Discogs response") {
		t.Fatalf("client.do() decode error = %v, want decode context", err)
	}
}
