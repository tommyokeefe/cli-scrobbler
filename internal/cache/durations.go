package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cli-scrobbler/internal/model"
)

type DurationStore struct {
	path    string
	Entries map[string]int64 `json:"entries"`
}

func Load(path string) (*DurationStore, error) {
	store := &DurationStore{
		path:    path,
		Entries: map[string]int64{},
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return store, nil
		}
		return nil, fmt.Errorf("read duration cache: %w", err)
	}

	if err := json.Unmarshal(data, store); err != nil {
		return nil, fmt.Errorf("parse duration cache: %w", err)
	}

	if store.Entries == nil {
		store.Entries = map[string]int64{}
	}

	store.path = path
	return store, nil
}

func (s *DurationStore) Lookup(releaseID int, track model.Track) (time.Duration, bool) {
	seconds, ok := s.Entries[cacheKey(releaseID, track)]
	if !ok {
		return 0, false
	}
	return time.Duration(seconds) * time.Second, true
}

func (s *DurationStore) Put(releaseID int, track model.Track, duration time.Duration) {
	s.Entries[cacheKey(releaseID, track)] = int64(duration / time.Second)
}

func (s *DurationStore) Save() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal duration cache: %w", err)
	}

	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		return fmt.Errorf("write duration cache: %w", err)
	}

	return nil
}

func cacheKey(releaseID int, track model.Track) string {
	return strings.Join([]string{
		strconv.Itoa(releaseID),
		normalize(track.Position),
		normalize(track.Title),
		normalize(track.Artist),
	}, "|")
}

func normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
