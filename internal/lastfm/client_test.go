package lastfm

import "testing"

func TestAuthURL(t *testing.T) {
	t.Parallel()

	client := NewClient("api-key", "api-secret", "")
	got := client.AuthURL("token-123")
	want := "https://www.last.fm/api/auth/?api_key=api-key&token=token-123"
	if got != want {
		t.Fatalf("AuthURL() = %q, want %q", got, want)
	}
}
