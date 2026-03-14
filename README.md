# Discogs CLI Scrobbler

## Configuration paths

The app uses two different locations for persisted data:

- Build config (`config.json`) is loaded from the repository root (the directory containing `go.mod`).
  - This file is intended for build-time Last.fm app credentials only.
- User config (`config.json`) is loaded from the user config directory under `cli-scrobbler`.
  - This file stores user-entered values from the interactive CLI (Discogs settings and Last.fm session key).
- `durations.json` is loaded from the user config directory under `cli-scrobbler`.
  - On macOS, this is typically `~/Library/Application Support/cli-scrobbler/durations.json`.

Environment variables still override values from `config.json`:

- `SCROBBLER_DISCOGS_TOKEN`
- `SCROBBLER_DISCOGS_USERNAME`
- `SCROBBLER_LASTFM_API_KEY`
- `SCROBBLER_LASTFM_API_SECRET`
- `SCROBBLER_LASTFM_SESSION_KEY`

## Building

Last.fm app credentials are baked into the binary at link time. Build with:

```bash
go build \
  -ldflags "-s -w \
    -X 'cli-scrobbler/internal/config.BuildLastFMAPIKey=YOUR_KEY' \
    -X 'cli-scrobbler/internal/config.BuildLastFMAPISecret=YOUR_SECRET'" \
  -o scrobble ./cmd/scrobble
```

Or install directly to `~/go/bin/`:

```bash
go install \
  -ldflags "-s -w \
    -X 'cli-scrobbler/internal/config.BuildLastFMAPIKey=YOUR_KEY' \
    -X 'cli-scrobbler/internal/config.BuildLastFMAPISecret=YOUR_SECRET'" \
  ./cmd/scrobble
```

## Releases

GitHub releases are built and published by GoReleaser via `.github/workflows/release.yml`.

On every published GitHub release, CI builds and uploads:

- macOS: amd64, arm64
- Linux: amd64, arm64
- Windows: amd64

Release binaries are packaged as archives (`.tar.gz` for macOS/Linux, `.zip` for Windows) plus `checksums.txt`.

Required repository secrets:

- `LASTFM_API_KEY`
- `LASTFM_API_SECRET`

To publish package-manager installers, also create:

- `TAP_GITHUB_TOKEN` (a Personal Access Token with write access to the tap/bucket repos)

And create these repositories under your GitHub account:

- `tommyokeefe/homebrew-tap`
- `tommyokeefe/scoop-bucket`

Then users can install with:

- Homebrew: `brew tap tommyokeefe/tap && brew install scrobble`
- Scoop: `scoop bucket add tommyokeefe https://github.com/tommyokeefe/scoop-bucket && scoop install scrobble`

## Local development

To avoid passing `-ldflags` every time during development, create a `config.json` in the repository root to override the baked-in credentials:

```json
{
  "lastfm_api_key": "your-lastfm-api-key",
  "lastfm_api_secret": "your-lastfm-api-secret"
}
```


