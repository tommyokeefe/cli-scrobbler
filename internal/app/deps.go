package app

import (
	"cli-scrobbler/internal/discogs"
	"cli-scrobbler/internal/lastfm"
)

var (
	newDiscogsClient = discogs.NewClient
	newLastFMClient  = lastfm.NewClient
)
