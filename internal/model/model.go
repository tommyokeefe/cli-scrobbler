package model

import "time"

type Album struct {
	ReleaseID int
	Title     string
	Artist    string
	Year      int
	Tracks    []Track
}

type Track struct {
	Position string
	Title    string
	Artist   string
	Duration time.Duration
}
