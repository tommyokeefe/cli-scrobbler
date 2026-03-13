package cache

import (
	"path/filepath"
	"testing"
	"time"

	"cli-scrobbler/internal/model"
)

func TestDurationStoreRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "durations.json")
	store, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	track := model.Track{
		Position: "A1",
		Title:    "Example Track",
		Artist:   "Example Artist",
	}

	store.Put(123, track, 245*time.Second)
	if err := store.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() after save error = %v", err)
	}

	duration, ok := reloaded.Lookup(123, track)
	if !ok {
		t.Fatal("Lookup() did not find cached duration")
	}

	if want := 245 * time.Second; duration != want {
		t.Fatalf("Lookup() duration = %v, want %v", duration, want)
	}
}
