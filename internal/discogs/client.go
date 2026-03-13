package discogs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"cli-scrobbler/internal/model"
)

const baseURL = "https://api.discogs.com"

var discogsArtistSuffix = regexp.MustCompile(` \(\d+\)$`)

type Client struct {
	httpClient *http.Client
	token      string
	userAgent  string
}

type CollectionRelease struct {
	ReleaseID int
	Title     string
	Artist    string
	Year      int
	Formats   []string
}

type releaseResponse struct {
	ID        int              `json:"id"`
	Title     string           `json:"title"`
	Year      discogsInt       `json:"year"`
	Artists   []artistResponse `json:"artists"`
	Tracklist []trackResponse  `json:"tracklist"`
}

type discogsInt int

type artistResponse struct {
	Name string `json:"name"`
}

type trackResponse struct {
	Position string           `json:"position"`
	Title    string           `json:"title"`
	Duration string           `json:"duration"`
	Type     string           `json:"type_"`
	Artists  []artistResponse `json:"artists"`
}

type identityResponse struct {
	Username string `json:"username"`
}

type collectionResponse struct {
	Pagination collectionPagination `json:"pagination"`
	Releases   []collectionItem     `json:"releases"`
}

type collectionPagination struct {
	Page  int `json:"page"`
	Pages int `json:"pages"`
}

type collectionItem struct {
	BasicInformation collectionBasicInformation `json:"basic_information"`
}

type collectionBasicInformation struct {
	ID      int              `json:"id"`
	Title   string           `json:"title"`
	Year    discogsInt       `json:"year"`
	Artists []artistResponse `json:"artists"`
	Formats []formatResponse `json:"formats"`
}

type formatResponse struct {
	Name string `json:"name"`
}

func NewClient(token, userAgent string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		token:      token,
		userAgent:  userAgent,
	}
}

func (c *Client) Identity(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/oauth/identity", nil)
	if err != nil {
		return "", fmt.Errorf("create Discogs identity request: %w", err)
	}

	var response identityResponse
	if err := c.do(req, &response); err != nil {
		return "", err
	}

	if strings.TrimSpace(response.Username) == "" {
		return "", fmt.Errorf("Discogs identity response did not include a username")
	}

	return response.Username, nil
}

func (c *Client) CollectionReleases(ctx context.Context, username string) ([]CollectionRelease, error) {
	const perPage = 100

	page := 1
	var releases []CollectionRelease
	for {
		values := url.Values{}
		values.Set("page", strconv.Itoa(page))
		values.Set("per_page", strconv.Itoa(perPage))

		endpoint := fmt.Sprintf("%s/users/%s/collection/folders/0/releases?%s", baseURL, url.PathEscape(username), values.Encode())
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("create Discogs collection request: %w", err)
		}

		var response collectionResponse
		if err := c.do(req, &response); err != nil {
			return nil, err
		}

		for _, item := range response.Releases {
			releases = append(releases, CollectionRelease{
				ReleaseID: item.BasicInformation.ID,
				Title:     item.BasicInformation.Title,
				Artist:    joinArtists(item.BasicInformation.Artists),
				Year:      int(item.BasicInformation.Year),
				Formats:   joinFormats(item.BasicInformation.Formats),
			})
		}

		if response.Pagination.Pages <= page {
			break
		}
		page++
	}

	return releases, nil
}

func SearchCollection(query string, releases []CollectionRelease) []CollectionRelease {
	normalizedQuery := normalizeSearchText(query)
	if normalizedQuery == "" {
		return nil
	}

	type scoredRelease struct {
		release CollectionRelease
		score   int
	}

	terms := strings.Fields(normalizedQuery)
	scored := make([]scoredRelease, 0, len(releases))
	for _, release := range releases {
		score := scoreCollectionRelease(normalizedQuery, terms, release)
		if score == 0 {
			continue
		}

		scored = append(scored, scoredRelease{
			release: release,
			score:   score,
		})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		if scored[i].release.Year != scored[j].release.Year {
			return scored[i].release.Year > scored[j].release.Year
		}
		return scored[i].release.Title < scored[j].release.Title
	})

	results := make([]CollectionRelease, len(scored))
	for i, item := range scored {
		results[i] = item.release
	}

	return results
}

func (c *Client) GetRelease(ctx context.Context, releaseID int) (model.Album, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/releases/"+strconv.Itoa(releaseID), nil)
	if err != nil {
		return model.Album{}, fmt.Errorf("create Discogs release request: %w", err)
	}

	var response releaseResponse
	if err := c.do(req, &response); err != nil {
		return model.Album{}, err
	}

	albumArtist := joinArtists(response.Artists)
	album := model.Album{
		ReleaseID: response.ID,
		Title:     response.Title,
		Artist:    albumArtist,
		Year:      int(response.Year),
		Tracks:    make([]model.Track, 0, len(response.Tracklist)),
	}

	for _, track := range response.Tracklist {
		if track.Type != "" && track.Type != "track" {
			continue
		}

		duration, err := ParseDuration(track.Duration)
		if err != nil {
			return model.Album{}, fmt.Errorf("parse track duration for %q: %w", track.Title, err)
		}

		artist := joinArtists(track.Artists)
		if artist == "" {
			artist = albumArtist
		}

		album.Tracks = append(album.Tracks, model.Track{
			Position: track.Position,
			Title:    track.Title,
			Artist:   artist,
			Duration: duration,
		})
	}

	return album, nil
}

func ParseDuration(value string) (time.Duration, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}

	parts := strings.Split(trimmed, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("unsupported duration format %q", value)
	}

	multiplier := 1
	totalSeconds := 0
	for i := len(parts) - 1; i >= 0; i-- {
		component, err := strconv.Atoi(parts[i])
		if err != nil {
			return 0, fmt.Errorf("invalid duration component %q", parts[i])
		}
		totalSeconds += component * multiplier
		multiplier *= 60
	}

	return time.Duration(totalSeconds) * time.Second, nil
}

func (d *discogsInt) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*d = 0
		return nil
	}

	var asInt int
	if err := json.Unmarshal(data, &asInt); err == nil {
		*d = discogsInt(asInt)
		return nil
	}

	var asString string
	if err := json.Unmarshal(data, &asString); err != nil {
		return fmt.Errorf("parse int value: %w", err)
	}

	asString = strings.TrimSpace(asString)
	if asString == "" {
		*d = 0
		return nil
	}

	parsed, err := strconv.Atoi(asString)
	if err != nil {
		return fmt.Errorf("parse int string %q: %w", asString, err)
	}

	*d = discogsInt(parsed)
	return nil
}

func joinArtists(artists []artistResponse) string {
	if len(artists) == 0 {
		return ""
	}

	names := make([]string, 0, len(artists))
	for _, artist := range artists {
		name := strings.TrimSpace(artist.Name)
		name = discogsArtistSuffix.ReplaceAllString(name, "")
		names = append(names, name)
	}

	return strings.Join(names, ", ")
}

func joinFormats(formats []formatResponse) []string {
	if len(formats) == 0 {
		return nil
	}

	names := make([]string, 0, len(formats))
	for _, format := range formats {
		name := strings.TrimSpace(format.Name)
		if name == "" {
			continue
		}
		names = append(names, name)
	}

	return names
}

func scoreCollectionRelease(query string, terms []string, release CollectionRelease) int {
	title := normalizeSearchText(release.Title)
	artist := normalizeSearchText(release.Artist)
	combined := strings.TrimSpace(artist + " " + title)

	score := 0
	switch {
	case combined == query:
		score += 1000
	case title == query:
		score += 950
	case artist == query:
		score += 925
	}

	if strings.Contains(combined, query) {
		score += 500
	}
	if strings.Contains(title, query) {
		score += 300
	}
	if strings.Contains(artist, query) {
		score += 250
	}

	for _, term := range terms {
		switch {
		case strings.Contains(title, term):
			score += 120
		case strings.Contains(artist, term):
			score += 110
		case strings.Contains(combined, term):
			score += 80
		}
	}

	if isSubsequence(query, combined) {
		score += 80
	}
	if isSubsequence(query, title) {
		score += 60
	}
	if isSubsequence(query, artist) {
		score += 50
	}

	return score
}

func normalizeSearchText(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	b.Grow(len(value))

	lastSpace := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastSpace = false
			continue
		}

		if !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}

	return strings.Join(strings.Fields(b.String()), " ")
}

func isSubsequence(query, value string) bool {
	if query == "" {
		return false
	}

	idx := 0
	for i := 0; i < len(value) && idx < len(query); i++ {
		if value[i] == query[idx] {
			idx++
		}
	}

	return idx == len(query)
}

func (c *Client) do(req *http.Request, dest any) error {
	req.Header.Set("Authorization", "Discogs token="+c.token)
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("perform Discogs request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Discogs request failed: %s", resp.Status)
	}

	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode Discogs response: %w", err)
	}

	return nil
}
