# Project Overview

## What This Project Is

**cli-scrobbler** (`scrobble`) is a Go CLI tool that scrobbles albums from a user's [Discogs](https://www.discogs.com/) collection to [Last.fm](https://www.last.fm/). It is designed for Last.fm enthusiasts who listen to physical media (vinyl, CDs, etc.) and want their listens tracked accurately without a streaming service doing it for them.

## Goals and Objectives

- **Primary goal:** Provide a seamless way to scrobble full album listens to Last.fm when the source is physical media in a Discogs collection.
- **Near-term:** Evolve toward a richer TUI (terminal user interface) experience — e.g. partial-album scrobbling, interactive track selection.
- **Longer-term possibilities:**
  - Display Last.fm stats (listening history, top artists, etc.) directly in the CLI/TUI.
  - Support additional collection sources beyond Discogs.

## Users and Stakeholders

- **Primary user:** The author (personal tool with real daily use).
- **Growing user base:** A small number of other Last.fm enthusiasts have adopted it.
- **Target audience:** Last.fm users who listen to physical media and want accurate scrobble records.

## Distribution

- **Homebrew:** `brew tap tommyokeefe/tap && brew install scrobble`
- **Scoop:** `scoop bucket add tommyokeefe https://github.com/tommyokeefe/scoop-bucket && scoop install scrobble`
- **Releases:** Built and published via GoReleaser (`.github/workflows/release.yml`) for macOS (amd64/arm64), Linux (amd64/arm64), and Windows (amd64).

## Key Modules

| Package | Responsibility |
|---------|---------------|
| `cmd/scrobble` | Binary entry point |
| `internal/app` | Top-level app wiring, interactive wizard, command flows, prompts, formatting |
| `internal/config` | Build-time and user config loading; environment variable overrides |
| `internal/discogs` | Discogs API client (collection search, release lookup) |
| `internal/lastfm` | Last.fm API client (auth, scrobbling) |
| `internal/scrobble` | Timeline calculation — maps track list to accurate timestamps |
| `internal/cache` | Local duration cache (`durations.json`) so missing track lengths are only entered once |
| `internal/model` | Shared data types |

## Configuration

Two layers of config, both named `config.json`:

- **Build config** (repo root) — optional dev override for Last.fm API credentials.
- **User config** (`~/Library/Application Support/cli-scrobbler/` on macOS) — stores Discogs settings and Last.fm session key.

Environment variables (`SCROBBLER_*`) override both.

Last.fm API credentials can also be baked into the binary at link time via `-ldflags`.

## Known Constraints and Design Decisions

- The current scope is **strictly** Discogs → Last.fm scrobbling of full albums. Any expansion (stats, partial albums, other sources) is future work.
- The TUI direction is aspirational but not yet started — current UX is an interactive prompt-driven wizard.
