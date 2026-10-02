# Configuration

Couchside is configured with environment variables, plus settings in the web
UI that are stored in its database. Release builds work with none of them set.

## Environment variables

| Variable | Default | Purpose |
| --- | --- | --- |
| `COUCHSIDE_MEDIA_ROOT` | none | Libraries must live under it; enables the folder picker |
| `COUCHSIDE_ADDR` | `:8080` | Listen address |
| `COUCHSIDE_DATA_DIR` | `./data` | SQLite database |
| `COUCHSIDE_CACHE_DIR` | `$DATA_DIR/cache` | Artwork, stills, subtitles, optimized copies |
| `COUCHSIDE_WEB_DIR` | none | Serve the UI from this folder instead of the one built into release binaries |
| `COUCHSIDE_WORKERS` | `2` | Background jobs at once (scans, matching, artwork) |
| `COUCHSIDE_SCAN_INTERVAL` | `6h` | Periodic rescan; `0` disables it |
| `COUCHSIDE_DEBUG` | none | Debug logging |
| `TZ` | UTC | Time zone for guide times and recording names |
| **Metadata** | | |
| `TMDB_API_KEY` | built in | Your own TMDB key (v3 key or v4 read token) instead of Couchside's; `off` disables TMDB |
| `OMDB_API_KEY` | none | Optional fallback metadata source; free keys at omdbapi.com |
| **Accounts** | | |
| `COUCHSIDE_AUTH` | `false` | Require passwords: no passwordless sign-in. Set it for a server on the internet |
| `COUCHSIDE_TRUSTED_PROXIES` | none | Reverse proxies (CIDRs or addresses, comma-separated) whose `X-Forwarded-For` and `X-Forwarded-Proto` are believed. See [install.md](install.md#putting-it-on-the-internet) |
| **Transcoding** | | |
| `COUCHSIDE_FFMPEG` / `COUCHSIDE_FFPROBE` | `ffmpeg` / `ffprobe` | Paths to ffmpeg and ffprobe |
| `COUCHSIDE_HWACCEL` | `none` | `vaapi`, `qsv` or `nvenc`; falls back to software if unusable |
| `COUCHSIDE_VAAPI_DEVICE` | `/dev/dri/renderD128` | VAAPI render node |
| `COUCHSIDE_MAX_TRANSCODES` | `2` | Live transcode sessions at once; idle ones are evicted |
| `COUCHSIDE_ENCODE_WORKERS` | `1` | Background optimize encodes at once |
| `COUCHSIDE_OPTIMIZE_HEIGHT` | `1080` | Height cap for optimized copies |
| **Live TV & DVR** | | |
| `COUCHSIDE_HDHOMERUN` | none | HDHomeRun IP or host; enables Live TV and the DVR |
| `COUCHSIDE_RECORDINGS_DIR` | `$DATA_DIR/recordings` | Default folder for recordings (Settings can pick another) |
| `COUCHSIDE_DVR_PAD_BEFORE` / `_AFTER` | `10s` / `10s` | Default recording padding (Settings → Advanced overrides) |
| `COUCHSIDE_COMSKIP` | `comskip` | Comskip binary; commercial detection is off when it isn't found |
| `COUCHSIDE_COMSKIP_INI` | built-in defaults | Your own `comskip.ini`; it must keep `output_edl=1` |

## Settings in the UI

Admins see everything on the Settings page; other accounts see only their own
preferences.

- **Server load** (top of the page): CPU and memory, ffmpeg load, and
  **Connected**: who has the app open and what each person is watching.
- **Your settings** (per profile): next-episode autoplay, subtitle language,
  commercial skipping, live TV quality, and passthrough on TVs.
- **Your account:** change your password.
- **Accounts:** passwordless sign-in, and the account manager
  ([accounts.md](accounts.md)).
- **Metadata:** whether TMDB and OMDb are set up.
- **Live TV & DVR:** refresh the guide, and where recordings are saved.
- **Transcoding:** which encoder is in use and whether GPU decoding works.
- **Advanced** (collapsed): recording padding, and how far into and before the
  end of a commercial break skipping starts and stops (1 second each by
  default, so a skip never cuts into the show).
