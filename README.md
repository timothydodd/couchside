<p align="center"><img src="web/public/icons/logo-256.png" alt="Couchside" width="128" height="128"></p>

# Couchside

A lightweight self-hosted media server for a home k3s cluster. It scans your movie and TV folders, matches them against the Open Movie Database (OMDb), and shows them in a poster-wall UI styled like Portside Lite.

- **Backend:** one static Go binary with pure-Go SQLite (no CGO) and ffmpeg for thumbnails.
- **Frontend:** React 19, Vite, and Tailwind v4, with the same design tokens as Portside Lite.
- **Deploy:** Helm chart for k3s: Traefik ingress, an NFS, hostPath or PVC media mount, and a SQLite PVC.

## What works today (milestone 1)

- Library scanning with filename parsing, for example `The.Matrix.1999.1080p.mkv` or `Show/Season 1/Show S01E02.mkv`. Unchanged files are skipped and deleted files are pruned.
- ffprobe stream info: duration, codecs, resolution, and track counts.
- OMDb matching: title, year, plot, genres, IMDb rating, posters, and per-season episode titles. Responses are cached for 30 days, and "Fix match" pins an IMDb id.
- Artwork: posters resized to WebP, backdrops grabbed from the video itself, and episode stills.
- Profiles: each person gets their own watch progress, Continue Watching, favourite channels and settings (theme, autoplay of the next episode, preferred subtitle language, commercial skipping, live TV quality). With more than one profile, each browser asks "Who's watching?" once and remembers the answer; switch from the sidebar. There are no passwords.
- Browsing: Home with a hero and rows, virtualised Movies and TV grids with search, genre, watched filters and sort, plus detail pages with seasons.
- Direct play with byte-range streaming, resume position, watched state, continue-watching, and next-episode autoplay.
- An Activity page for background jobs, with retry.
- Library management: rename a library or move it to another folder (watch history follows), and a **Manage** view per library. Sort by quality, size or date added; filter to duplicates, unmatched or SD titles; fix a match by searching OMDb under any name; upload your own poster or backdrop; delete extra copies or whole movies and series from disk.

## Encoding

- **Automatic playback choice.** The player checks what the browser can decode. It plays the original file when it can, an optimized copy if one exists, and otherwise a live HLS stream.
- **Live HLS.** H.264 video with incompatible audio (DTS, AC3, TrueHD) or an MKV container is remuxed: the video is copied and only the audio is converted. Everything else is transcoded to H.264/AAC. HDR is tone-mapped to SDR.
- **Seeking.** Keyframes are forced every 4 seconds, so the full timeline is seekable immediately. Jumping ahead restarts ffmpeg at that point.
- **Throttling.** ffmpeg pauses once it's two minutes ahead of the player and stops when nobody is watching.
- **Buffering fallback.** In Auto quality, three stalls within a minute step playback down, from direct to 1080p, then 720p, then 480p. The gear menu picks a quality manually.
- **Optimize.** Background encodes make browser-friendly MP4 copies per title or per library. They run one at a time, show progress on the Activity page, and can be cancelled.
- **Player.** Movies, episodes, live TV and in-progress recordings share one player with custom controls:
  - A seek bar with buffered and recorded ranges, and hover times.
  - Skip back 10 seconds and forward 30, volume, and full screen.
  - Keyboard shortcuts: Space or K to play and pause, arrows to seek and change volume, M to mute, F for full screen.
  - A settings menu with Quality, Audio, Subtitles, Playback speed, and Playback info. The info page shows direct play versus transcode, source and output codecs, the transcoder, buffer, dropped frames and bandwidth.
- **Audio tracks.** Picking a non-default audio track switches to a server stream that carries it, because browsers can't switch tracks in a direct-played file.
- **Subtitles.** Embedded text subtitles (SRT, ASS, mov_text) and sidecar files next to the video (`Movie.en.srt`, `.vtt`, `.ass`) are converted to WebVTT and cached. Forced text tracks turn on automatically. Picture subtitles (Blu-ray PGS, DVD) are burned in by the server, scaled to the video, and read from 10 seconds before each seek point so captions survive seeking.
- **Live timeline.** Live TV shows a LIVE / GO LIVE button and can rewind within the session. A show that's being recorded offers **Start over**. In-progress recordings show the whole program with the unrecorded part hatched, and jump to the recorded edge without a second tuner.
- **Hardware.** Intel/AMD iGPUs work through VAAPI. The Intel drivers are in the image, and `/dev/dri` must be passed in. An unusable GPU falls back to software with a log warning. QSV and NVENC need an ffmpeg built with them. Alpine's lacks NVENC and the newer Intel QSV runtime.

## Live TV & DVR

- **Tuner.** Set `COUCHSIDE_HDHOMERUN` to the HDHomeRun's IP address. The channel lineup refreshes hourly.
- **Guide.** Listings come from SiliconDust's free guide service, about a day ahead, refreshed every 3 hours.
- **Finding channels.** The Guide and Channels tabs share a filter bar:
  - Search by channel name, number or show title; matching shows stay bright and the rest dim.
  - A genre list built from the guide's categories, and a **New** toggle for first airings.
  - HD-only or SD-only.
  - **Hide weak** drops channels whose signal quality was under 60% at the tuner's last scan. Each channel shows signal bars.
  - **Hide locked** hides the DRM channels.
  - The star on a channel pins it to the top, and **Favorites** shows only pinned channels. Pins are stored on the server; filter choices are remembered per browser.
- **Live TV.** The tuner stream is deinterlaced and transcoded to HLS at 1080p, 720p or 480p. Viewers of the same channel share one tuner, and the tuner is released about 20 seconds after the last viewer leaves. Pause and rewind work within the session. Page Up and Page Down change channel.
- **Recording.** Record any program from the guide, the channel list or the live player. Recordings copy the tuner stream with no transcoding, padded by 1 minute before and 2 minutes after. They're named Plex-style (`Show/Season 3/Show - S03E15 - Title.ts`, or by air date) and land in an automatic "DVR Recordings" library.
- **Same-titled shows.** The guide doesn't say which *MacGyver* is airing (1985 or 2016). When a recording is scheduled, the episode's original air date, season and episode are checked against OMDb, and a show whose title is shared gets the year in its folder (`MacGyver (2016)/Season 2/…`), so it's matched to the right series and kept apart from the other. This needs `OMDB_API_KEY`. Shows with a unique title keep plain folder names.
- **Where recordings go.** Choose in Settings under Live TV & DVR. The options are Couchside's own storage, any TV library folder such as TV Shows, or another folder under the media root. Recordings reuse an existing matching show and season folder (`The Simpsons (1989)/Season 07`), so they join that show instead of duplicating it. Existing recordings can be moved along. Recording into a library folder needs the media share mounted writable, which the compose file does.
- **Watching a recording in progress.** Use **From start** on the Recordings tab, or **Watch from start** in a program's details, to play a recording that's still in progress from its beginning, with a jump to the live broadcast.
- **Robustness.** A dropped signal or busy tuner is retried and the parts are joined. A server restart mid-recording resumes into the same file. Overlapping recordings beyond the tuner count are flagged as a conflict.
- **Series recordings.** Choose **Record series** on any program and pick a mode:
  - **Episodes I don't have.** Skips episodes already in the linked library show or already recorded, cancelled airings, and repeat airings. Airings with no episode info are recorded only when they're new.
  - **New episodes only.** First airings only.
  - **Every airing.** Everything, repeats included.
  - **Options.** Limit to one channel, and keep only the last N recordings.
  - **Linking.** The rule links to the same-named library show whose episode titles match the upcoming airings, and you can change it.
  - **When it runs.** Rules re-evaluate after every guide refresh and hourly. When all tuners are booked, a later airing of the same episode is used instead.
- **Commercial skipping.** Each finished recording is checked for commercial breaks with [Comskip](https://github.com/erikkaashoek/Comskip), which is built into the container image. The breaks are marked on the timeline, and the player skips them automatically, with a **Watch it** link to go back. The gear menu's **Commercials** page switches to a skip button (or the S key), or turns skipping off. Other `.ts` files, such as older Plex DVR recordings, can be checked from the same page. The recording file itself is never cut. For zip installs, put `comskip` on the PATH or set `COUCHSIDE_COMSKIP`; `COUCHSIDE_COMSKIP_INI` points at a tuned `comskip.ini`, which must keep `output_edl=1`.
- **Limits.** ATSC 3.0 channels with DRM can't be watched or recorded outside SiliconDust's apps.

## Not built yet

- **Auth.** There is no login, and profiles have no passwords. Keep Couchside on your LAN, or put it behind an auth proxy such as Authelia or oauth2-proxy.
- A Schedules Direct guide source for two weeks of listings.
- A TMDB provider for real backdrops.

## Install

Every release on the [Releases page](../../releases) has zips for Linux, macOS and Windows, plus a multi-arch container (amd64 and arm64).

**Container**

```bash
docker run -d --name couchside -p 8080:8080 \
  -v couchside-data:/data -v couchside-cache:/cache \
  -v /path/to/media:/media \
  -e OMDB_API_KEY=xxxx \
  ghcr.io/timothydodd/couchside:latest
```

Mount the media read-only (`:ro`) unless you want the DVR to record into a library folder. Add `-e COUCHSIDE_HDHOMERUN=<tuner IP>` for Live TV.

**Zip**

Each zip holds a single `couchside` binary with the web UI built in. Install `ffmpeg` (which includes `ffprobe`), then run:

```bash
COUCHSIDE_MEDIA_ROOT=/path/to/media ./couchside
```

Open http://localhost:8080 and add libraries on the Libraries page.

**Docker Compose against a NAS share**

Copy `.env.example` to `.env`, fill it in, and run `docker compose up -d`. Docker Desktop can't see mapped network drives, so the compose file mounts the SMB share directly.

**k3s / Kubernetes (Helm)**

```bash
kubectl create namespace media
kubectl -n media create secret generic couchside-omdb --from-literal=omdb-api-key=xxxx

helm install couchside deploy/helm/couchside -n media \
  --set omdb.existingSecret=couchside-omdb \
  --set media.type=nfs --set media.nfs.server=192.168.1.10 --set media.nfs.path=/volume1/media \
  --set ingress.enabled=true --set ingress.host=couchside.home.lan
```

Then open the UI and add libraries under `/media` on the Libraries page.

## Development

```bash
# API on :8080 (Go 1.26; the go command fetches it automatically)
cd server
OMDB_API_KEY=xxxx COUCHSIDE_MEDIA_ROOT=/path/to/media go run ./cmd/couchside

# UI on :5173, proxied to the API
cd web
npm install
npm run dev
```

A plain `go build` serves the API only. The release build copies `web/dist` into `server/internal/webui/dist` first, which embeds the UI; `COUCHSIDE_WEB_DIR` points at a UI folder on disk instead.

## Releasing

Push a version tag. The release workflow builds the zips, publishes the container to `ghcr.io/<owner>/couchside` (`:<version>`, `:<major>.<minor>`, `:latest`), and creates a GitHub Release with checksums. Tags with a hyphen, such as `v0.2.0-rc1`, become pre-releases and don't move `:latest`.

```bash
git tag v0.2.0 && git push origin v0.2.0
```

## Configuration

| Env var | Default | Purpose |
|---|---|---|
| `OMDB_API_KEY` | none | Enables metadata matching. Free keys at omdbapi.com |
| `COUCHSIDE_ADDR` | `:8080` | Listen address |
| `COUCHSIDE_DATA_DIR` | `./data` | SQLite database |
| `COUCHSIDE_CACHE_DIR` | `$DATA_DIR/cache` | Artwork and stills |
| `COUCHSIDE_WEB_DIR` | none | Serve the UI from this folder instead of the one embedded in release builds |
| `COUCHSIDE_MEDIA_ROOT` | none | Libraries must live under it; enables the folder picker |
| `COUCHSIDE_WORKERS` | `2` | Concurrent background jobs |
| `COUCHSIDE_SCAN_INTERVAL` | `6h` | Periodic rescan; `0` disables it |
| `COUCHSIDE_DEBUG` | none | Debug logging |
| `COUCHSIDE_HWACCEL` | `none` | `vaapi`, `qsv` or `nvenc`; falls back to software if unusable |
| `COUCHSIDE_VAAPI_DEVICE` | `/dev/dri/renderD128` | VAAPI render node |
| `COUCHSIDE_MAX_TRANSCODES` | `2` | Live transcode sessions at once; idle ones are evicted |
| `COUCHSIDE_ENCODE_WORKERS` | `1` | Background optimize encodes at once |
| `COUCHSIDE_OPTIMIZE_HEIGHT` | `1080` | Height cap for optimized copies |
| `COUCHSIDE_HDHOMERUN` | none | HDHomeRun IP or host; enables Live TV and DVR |
| `COUCHSIDE_RECORDINGS_DIR` | `$DATA_DIR/recordings` | Writable folder for recordings |
| `COUCHSIDE_DVR_PAD_BEFORE` / `_AFTER` | `1m` / `2m` | Recording padding |
| `TZ` | UTC | Time zone for guide times and recording names |
