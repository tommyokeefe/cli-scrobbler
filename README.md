# Discogs CLI Scrobbler

[![CI](https://github.com/tommyokeefe/cli-scrobbler/actions/workflows/ci.yml/badge.svg)](https://github.com/tommyokeefe/cli-scrobbler/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/tommyokeefe/cli-scrobbler/branch/main/graph/badge.svg?token=46THHHZGCO)](https://codecov.io/gh/tommyokeefe/cli-scrobbler)
[![GitHub release](https://img.shields.io/github/v/release/tommyokeefe/cli-scrobbler)](https://github.com/tommyokeefe/cli-scrobbler/releases)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)
[![Go Report Card](https://goreportcard.com/badge/github.com/tommyokeefe/cli-scrobbler)](https://goreportcard.com/report/github.com/tommyokeefe/cli-scrobbler)

Scrobble albums from your Discogs collection to Last.fm from the command line.

The app helps you:

- connect Discogs + Last.fm from an interactive CLI
- search your own Discogs collection
- select an album and scrobble its full track list with correct timestamps
- cache missing track durations so you only enter them once

## App Installation

- Homebrew: `brew tap tommyokeefe/tap && brew install scrobble`
- Scoop: `scoop bucket add tommyokeefe https://github.com/tommyokeefe/scoop-bucket && scoop install scrobble`

## Running locally

Without installing:

```bash
go run ./cmd/scrobble
```

Build a local binary:

```bash
go build -o scrobble ./cmd/scrobble
./scrobble
```

## First-time setup

Interactive mode (recommended):

```bash
scrobble
```

Or explicit auth commands:

```bash
scrobble auth discogs --token <token> [--username <name>] [--user-agent <ua>]
scrobble auth lastfm
```

Common commands:

```bash
scrobble search <album query>
scrobble scrobble --started-at "2026-03-13 20:15" <album query>
```

## Configuration paths

The app uses separate build-time and user-time configuration:

- Build config (`config.json`) is loaded from the repository root when you run the app from inside the working tree.
  - Intended as an optional development override for Last.fm app credentials (`lastfm_api_key`, `lastfm_api_secret`).
- User config (`config.json`) is loaded from the user config directory under `cli-scrobbler`.
  - Stores user-entered values from the interactive CLI (Discogs settings and Last.fm session key).
- `durations.json` is loaded from the user config directory under `cli-scrobbler`.
  - On macOS, this is typically `~/Library/Application Support/cli-scrobbler/durations.json`.

Environment variables override config values:

- `SCROBBLER_DISCOGS_TOKEN`
- `SCROBBLER_DISCOGS_USERNAME`
- `SCROBBLER_LASTFM_API_KEY`
- `SCROBBLER_LASTFM_API_SECRET`
- `SCROBBLER_LASTFM_SESSION_KEY`

## Building

Last.fm app credentials can be baked into the binary at link time:

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

On every published release, CI uploads archives for:

- macOS: amd64, arm64
- Linux: amd64, arm64
- Windows: amd64

Required repository secrets:

- `LASTFM_API_KEY`
- `LASTFM_API_SECRET`



## Local development

To avoid passing `-ldflags` every build during development, create a repo-root `config.json`:

```json
{
  "lastfm_api_key": "your-lastfm-api-key",
  "lastfm_api_secret": "your-lastfm-api-secret"
}
```

## License

This project is licensed under the GNU General Public License v3.0. You can use, modify, and redistribute it, but if you distribute modified versions you must make the corresponding source available under the same license terms.
