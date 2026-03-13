package discogs

import (
	"encoding/json"
	"testing"
	"time"
)

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
