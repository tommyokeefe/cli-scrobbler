# cli-scrobbler

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

## Example config.json

Create `config.json` in the repository root with Last.fm app credentials:

```json
{
  "lastfm_api_key": "your-lastfm-api-key",
  "lastfm_api_secret": "your-lastfm-api-secret"
}
```

At minimum, Last.fm requires:

- `lastfm_api_key`
- `lastfm_api_secret`

User-specific values (`discogs_token`, `discogs_username`, `discogs_user_agent`, `lastfm_session_key`) are collected from the user via the interactive CLI and saved in the user config directory.
