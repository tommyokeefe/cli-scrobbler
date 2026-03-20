package lastfm

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"cli-scrobbler/internal/scrobble"
)

const apiEndpoint = "https://ws.audioscrobbler.com/2.0/"

const maxBatchSize = 50

type Client struct {
	httpClient *http.Client
	apiKey     string
	apiSecret  string
	sessionKey string
}

type response struct {
	Error   int    `json:"error"`
	Message string `json:"message"`
}

type tokenResponse struct {
	response
	Token string `json:"token"`
}

type sessionResponse struct {
	response
	Session sessionPayload `json:"session"`
}

type sessionPayload struct {
	Key string `json:"key"`
}

func NewClient(apiKey, apiSecret, sessionKey string) *Client {
	return NewClientWithHTTPClient(apiKey, apiSecret, sessionKey, &http.Client{Timeout: 20 * time.Second})
}

func NewClientWithHTTPClient(apiKey, apiSecret, sessionKey string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}

	return &Client{
		httpClient: httpClient,
		apiKey:     apiKey,
		apiSecret:  apiSecret,
		sessionKey: sessionKey,
	}
}

func (c *Client) Scrobble(ctx context.Context, tracks []scrobble.ScheduledTrack, albumTitle string) error {
	if len(tracks) == 0 {
		return fmt.Errorf("no tracks to scrobble")
	}

	for start := 0; start < len(tracks); start += maxBatchSize {
		end := start + maxBatchSize
		if end > len(tracks) {
			end = len(tracks)
		}

		if err := c.scrobbleBatch(ctx, tracks[start:end], albumTitle); err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) GetAuthToken(ctx context.Context) (string, error) {
	values := url.Values{}
	values.Set("method", "auth.getToken")
	values.Set("api_key", c.apiKey)
	values.Set("api_sig", signature(map[string]string{
		"method":  "auth.getToken",
		"api_key": c.apiKey,
	}, c.apiSecret))
	values.Set("format", "json")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiEndpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return "", fmt.Errorf("create Last.fm token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var body tokenResponse
	if err := c.do(req, &body); err != nil {
		return "", err
	}
	if body.Error != 0 {
		return "", fmt.Errorf("Last.fm API error %d: %s", body.Error, body.Message)
	}
	if strings.TrimSpace(body.Token) == "" {
		return "", fmt.Errorf("Last.fm token response did not include a token")
	}

	return body.Token, nil
}

func (c *Client) AuthURL(token string) string {
	values := url.Values{}
	values.Set("api_key", c.apiKey)
	values.Set("token", token)
	return "https://www.last.fm/api/auth/?" + values.Encode()
}

func (c *Client) GetSessionKey(ctx context.Context, token string) (string, error) {
	values := url.Values{}
	values.Set("method", "auth.getSession")
	values.Set("api_key", c.apiKey)
	values.Set("token", token)
	values.Set("api_sig", signature(map[string]string{
		"method":  "auth.getSession",
		"api_key": c.apiKey,
		"token":   token,
	}, c.apiSecret))
	values.Set("format", "json")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiEndpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return "", fmt.Errorf("create Last.fm session request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var body sessionResponse
	if err := c.do(req, &body); err != nil {
		return "", err
	}
	if body.Error != 0 {
		return "", fmt.Errorf("Last.fm API error %d: %s", body.Error, body.Message)
	}
	if strings.TrimSpace(body.Session.Key) == "" {
		return "", fmt.Errorf("Last.fm session response did not include a session key")
	}

	return body.Session.Key, nil
}

func (c *Client) scrobbleBatch(ctx context.Context, tracks []scrobble.ScheduledTrack, albumTitle string) error {
	values := url.Values{}
	values.Set("method", "track.scrobble")
	values.Set("api_key", c.apiKey)
	values.Set("sk", c.sessionKey)

	signatureParams := map[string]string{
		"method":  "track.scrobble",
		"api_key": c.apiKey,
		"sk":      c.sessionKey,
	}

	for i, track := range tracks {
		idx := strconv.Itoa(i)
		values.Set("artist["+idx+"]", track.Track.Artist)
		values.Set("track["+idx+"]", track.Track.Title)
		values.Set("timestamp["+idx+"]", strconv.FormatInt(track.StartedAt.Unix(), 10))
		values.Set("album["+idx+"]", albumTitle)

		signatureParams["artist["+idx+"]"] = track.Track.Artist
		signatureParams["track["+idx+"]"] = track.Track.Title
		signatureParams["timestamp["+idx+"]"] = strconv.FormatInt(track.StartedAt.Unix(), 10)
		signatureParams["album["+idx+"]"] = albumTitle

		if track.Track.Duration > 0 {
			seconds := strconv.FormatInt(int64(track.Track.Duration/time.Second), 10)
			values.Set("duration["+idx+"]", seconds)
			signatureParams["duration["+idx+"]"] = seconds
		}
	}

	values.Set("api_sig", signature(signatureParams, c.apiSecret))
	values.Set("format", "json")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiEndpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return fmt.Errorf("create Last.fm scrobble request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var body response
	if err := c.do(req, &body); err != nil {
		return err
	}
	if body.Error != 0 {
		return fmt.Errorf("Last.fm API error %d: %s", body.Error, body.Message)
	}

	return nil
}

func decodeResponse(resp *http.Response, dest any) error {
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode Last.fm response: %w", err)
	}

	return nil
}

func (c *Client) do(req *http.Request, dest any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("perform Last.fm request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Last.fm request failed: %s", resp.Status)
	}

	return decodeResponse(resp, dest)
}

func signature(params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	h := md5.New()
	for _, key := range keys {
		h.Write([]byte(key))
		h.Write([]byte(params[key]))
	}
	h.Write([]byte(secret))

	return hex.EncodeToString(h.Sum(nil))
}
