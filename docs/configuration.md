# Configuration

Couchside is configured with environment variables, plus settings in the web
UI that are stored in its database. Release builds work with none of them set.

The same variables can go in a settings file, one `KEY=value` per line (`#`
starts a comment). Couchside reads `COUCHSIDE_CONFIG` if it's set, and on
Windows `%ProgramData%\Couchside\couchside.env`, which the installer writes.
A variable set in the environment wins over the file.

Most of them can also be set by an admin in **Settings → Server**: rescans and
background jobs, the hardware
encoder and how many streams and optimized copies run at once, the HDHomeRun, the metadata keys and the network settings. A value
saved there wins over the variable and the settings file, and takes effect
when the server restarts, which the page does in place (playback stops;
recordings carry on in the same file). Clearing a field goes back to the
variable. If the server can't start with the saved values, it starts without
them and the page says why. The listen address, the data and cache folders,
`COUCHSIDE_WEB_DIR`, `COUCHSIDE_AUTH` and the programs Couchside runs
(`COUCHSIDE_FFMPEG`, `COUCHSIDE_FFPROBE`, `COUCHSIDE_COMSKIP`,
`COUCHSIDE_COMSKIP_INI`) can only be set in the environment or the settings
file (`couchside.env` on Windows), so a browser session can't choose what the
server runs.

**Media locations** are where libraries can be: drives, folders and network
shares, added in the first-run setup or Settings → Server (with a folder
browser). On Windows a network share (`\\nas\media`) can have the NAS's user
name and password; Couchside signs in to it whenever it starts. In a
container, mount your media and set `COUCHSIDE_MEDIA_ROOT`: those folders are
locations too, fixed by the environment.

## Environment variables

| Variable | Default | Purpose |
| --- | --- | --- |
| `COUCHSIDE_MEDIA_ROOT` | none | Media locations from the environment (containers), one or more separated like `PATH` (`:`, or `;` on Windows); more can be added in the web app |
| `COUCHSIDE_ADDR` | `:8080` | Listen address |
| `COUCHSIDE_DATA_DIR` | `./data` | SQLite database, plus `auth.key` (signs sessions) and `server.id`: back up all three. The Windows service defaults to `%ProgramData%\Couchside\data` |
| `COUCHSIDE_CONFIG` | see above | Settings file to read |
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
| `COUCHSIDE_FFMPEG` / `COUCHSIDE_FFPROBE` | beside `couchside`, else on the PATH | Paths to ffmpeg and ffprobe |
| `COUCHSIDE_HWACCEL` | `auto` | `auto` tries the GPU encoders this machine might have (Windows: NVENC, then Quick Sync; Linux: VAAPI when `/dev/dri` is there, then NVENC, Quick Sync) and keeps the first that works. `nvenc`, `qsv`, `vaapi` or `none` picks one; an unusable choice falls back to software. The container and Helm chart set `none` unless you change it |
| `COUCHSIDE_VAAPI_DEVICE` | `/dev/dri/renderD128` | VAAPI render node |
| `COUCHSIDE_MAX_TRANSCODES` | `2` | Live transcode sessions at once; idle ones are evicted |
| `COUCHSIDE_ENCODE_WORKERS` | `1` | Background optimize encodes at once |
| `COUCHSIDE_OPTIMIZE_HEIGHT` | `1080` | Height cap for optimized copies |
| **Live TV & DVR** | | |
| `COUCHSIDE_HDHOMERUN` | none | HDHomeRun IP or host; enables Live TV and the DVR |
| `COUCHSIDE_RECORDINGS_DIR` | `$DATA_DIR/recordings` | Default folder for recordings (Settings can pick another) |
| `COUCHSIDE_DVR_PAD_BEFORE` / `_AFTER` | `10s` / `10s` | Default recording padding (Settings → Live TV → Advanced overrides) |
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
  as one (60 seconds). **Backups** is here too (below).

The search box in the sidebar (press `/`) finds movies, shows, episodes,
channels and guide listings.

## Backups

Couchside copies its database to `backups/` in the data folder once a day and
keeps the last 7 (Settings → System → Advanced changes both, makes one on demand, and
downloads or deletes them). Each is a zip of the database, `auth.key` and
`server.id`. Before a new version changes the database's tables it also saves
a copy there (`couchside-upgrade-….db`; the last 3 are kept).

A backup holds password hashes and the key that signs sessions: keep
downloaded ones somewhere private. Media, artwork and recordings aren't in
it; artwork is fetched again, and the rest are your files.

The backups folder is on the same disk as the database. For protection from
losing that disk, copy the newest backup somewhere else.

**Restoring.** Stop Couchside, then run `couchside restore <file>` with the
data folder mounted (a zip or a `.db`; a name without a folder is looked for
in `backups/`). The database it replaces is kept as
`couchside.db.before-restore`. In Kubernetes: scale the Deployment to 0, run a
one-off pod of the same image with the data volume and `restore` as its
argument, then scale back up.

**Going back to an older version.** An older Couchside won't open a database
a newer one has upgraded: it stops with a message naming the newer change.
To go back, stop Couchside, run `couchside restore couchside-upgrade-<time>.db`
(the copy saved just before the upgrade) with the older version, then start
it. Anything changed since that upgrade is lost.
