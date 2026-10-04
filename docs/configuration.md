# Configuration

Couchside is configured with environment variables, plus settings in the web
UI that are stored in its database. Release builds work with none of them set.

## Environment variables

| Variable | Default | Purpose |
| --- | --- | --- |
| `COUCHSIDE_MEDIA_ROOT` | none | Libraries must live under it; enables the folder picker |
| `COUCHSIDE_ADDR` | `:8080` | Listen address |
| `COUCHSIDE_DATA_DIR` | `./data` | SQLite database, plus `auth.key` (signs sessions) and `server.id`: back up all three |
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
| **LAN discovery** | | |
| `COUCHSIDE_DISCOVERY` | on | Answer SSDP searches so TV apps find the server; `false` turns it off |
| `COUCHSIDE_SERVER_NAME` | host name | The name TV apps list the server under |
| `COUCHSIDE_DISCOVERY_URL` | this machine's address and port | Base URL to advertise instead, when the LAN reaches Couchside on another port or name (Docker `-p 8095:8080`, a NodePort) |
| `COUCHSIDE_DISCOVERY_INTERFACE` | the default one | Network interface to listen on, on a machine with several |
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

Every profile has **Settings** for its own preferences. Admins also get the
server's pages, listed under Settings in the sidebar (tabs on a phone).

- **Your settings** (every profile): next-episode autoplay, subtitle
  language, commercial skipping, live TV quality, and passthrough on TVs.
  **Your account** below it changes your password and signs out devices.
- **System:** CPU and memory right now, ffmpeg's share, and **Connected**
  (who has the app open and what they're watching); the same **over time**
  (15 minutes to 24 hours, kept in memory since the server started); which
  encoder is in use and whether GPU decoding passed its start-up test; and
  the server's folders and version.
- **Console:** the server's log as it happens (the last 2000 lines), filtered
  by level or text. It includes sign-in names and addresses.
- **Accounts:** passwordless sign-in, and the account manager
  ([accounts.md](accounts.md)).
- **Metadata:** whether TMDB and OMDb are set up.
- **Live TV:** refresh the guide, where recordings are saved, and **Your
  channels** (channels made from the library).
- **Advanced:** recording padding, how far into and before the end of a
  commercial break skipping starts and stops (1 second each by default, so a
  skip never cuts into the show), and how close two breaks must be to count
  as one (60 seconds).

The search box in the sidebar (press `/`) finds movies, shows, episodes,
channels and guide listings.
