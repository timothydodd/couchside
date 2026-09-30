# CLAUDE.md

Couchside is a self-hosted media server for k3s. Go backend in `server/`, React frontend in `web/`, Helm chart in `deploy/helm/couchside`. The UI deliberately matches `../portside-lite`: same tokens, component classes and shell layout.

## Build & verify (WSL)

- Go 1.26 is required by `modernc.org/sqlite`. `~/.local/go/bin/go` is 1.23, but GOTOOLCHAIN=auto downloads 1.26 on first use.
  `cd server && ~/.local/go/bin/go vet ./... && ~/.local/go/bin/go test ./...`
- npm on `/mnt/f` is slow. Copy `web/` (without node_modules) to the session scratchpad, then run `npm install && npx tsc --noEmit && npx vite build` there. Copy `package-lock.json` back if it changed.
- Full-stack smoke test: build the binary, then run it with `COUCHSIDE_DATA_DIR`, `COUCHSIDE_MEDIA_ROOT` and `COUCHSIDE_WEB_DIR=<scratch>/web/dist`. Generate sample media with `ffmpeg -f lavfi -i testsrc2=...`.
- Docker builds and the real deploy run from Windows.
- **Embedded UI**: `server/internal/webui` embeds `dist/`, which is only a `.gitkeep` in git. The Dockerfile and release workflow copy `web/dist` there before `go build`. `COUCHSIDE_WEB_DIR` overrides it with a folder on disk.
- **Releases**: push a `v*` tag. `.github/workflows/release.yml` builds zips (linux/darwin amd64 and arm64, windows amd64), a multi-arch ghcr image, and a GitHub Release with SHA256SUMS. `ci.yml` runs vet, test and the web build on pushes and PRs. Icons live in `web/public/icons/logo-<size>.png`, plus `favicon.ico`, `apple-touch-icon.png` and `site.webmanifest`.

## Architecture

- **One process, one pod.** The HTTP API and worker pool share a SQLite database in WAL mode. The Deployment uses `strategy: Recreate` because the DB sits on a ReadWriteOnce PVC. Don't scale replicas.
- **Job queue** is the `jobs` table. A partial unique index dedupes active jobs, and `ClaimJob` runs `UPDATE ... RETURNING` in priority order: scan, match, artwork, still. Kinds are defined in `internal/worker/worker.go`.
- **Scan flow:** walk, then `parse.Movie`/`parse.Episode`, then `probe.Probe`, then upsert. A newly created item enqueues `match`, which enqueues `artwork`. TV files enqueue `still`. Pruning uses `files.last_seen < scanStart` and is skipped after a partial walk.
- **Parsing** (`internal/parse`) covers S01E02 with seasons up to 4 digits, 1x02, Plex-DVR date names (`Show - 2024-11-10 03 30 00 - Title.ts`), and bare E01 files. Date names become episode `YYMMDDHHMM` plus `episodes.air_date`. Unchanged files are re-parsed every scan (no probe), and re-indexed if the parse changed, so parser fixes reach existing libraries.
- **Matching** never trusts OMDb's fuzzy title lookup. `metadata.pickBest` scores candidates from exact lookup with and without year, plus search with and without year, on normalized title and year. Title variants ("3.10" to "3:10", " - " to ": ", "&" and "and") are tried when the result is missing or only partial. `imdb_pinned` marks "Fix match" choices, which automatic re-matches keep.
- **Unplayable files** get `files.problem` (`unreadable` or `no-video`, usually DRM iTunes files). They get no stills, optimize or HLS, and the UI explains why.
- **Grouping key** is `(library_id, kind, parsed_title, parsed_year)`, where `parsed_year` is 0 when unknown. Metadata overwrites `title`, but grouping always uses the parsed fields.
- **Metadata providers** implement `metadata.Provider` and run in order through `metadata.Chain`. OMDb responses are cached in `provider_cache`. Not-found answers are cached; bad-key and quota errors are not.
- **Artwork** lives in `$CACHE/items/<id>/{poster,poster-thumb,backdrop}.webp` and `$CACHE/files/<id>/still.webp`. URLs carry `?v=updatedAt` for cache busting.
- **Frontend** uses a tiny history-API router (`stores/router.ts`), `useApi` for stale-while-revalidate fetches, and a zustand status store that polls `/api/status`. Grids are virtualised with @tanstack/react-virtual and manage their own scroll.

## Encoding (internal/transcode, worker/optimize.go, web PlayerPage)

- **Decision** happens client-side in `web/src/lib/playback.ts`. It uses canPlayType for direct play, then the optimized copy, then HLS. The server then decides copy vs encode per stream in `transcode.Manager.Create`. Video is copied only for 8-bit H.264 that isn't HDR and doesn't need downscaling.
- **HLS sessions**: the playlist is synthesized up front, with `SegDur` = 4s and every segment listed. Segments are generated on request. A request outside the current ffmpeg run restarts ffmpeg with `-ss N*4 -copyts -start_at_zero` and `-force_key_frames expr:gte(t,n_forced*4)`. `t` is relative to the run's first frame, so don't add the start offset. Remuxed segments follow source keyframes, which hls.js tolerates.
- ffmpeg is SIGSTOPped when more than 30 segments ahead and killed after 90s idle. Sessions expire after 3h and are wiped on start. At the limit, a session idle 20s or more is evicted.
- **Optimize** jobs (`kind=optimize`) run in their own worker pool (`ClaimJob(ctx, encode=true)`), so scans are never blocked. Progress comes from `ffmpeg -progress pipe:1`. Cancel deletes the job row. Output goes to `$CACHE/optimized/<fileId>.mp4`. A changed file drops its optimized copy, and orphans are cleaned at startup.
- **Hardware**: `transcode.Detect` runs a one-frame test encode and falls back to software. On Intel iGPUs use VAAPI (the iHD driver is in the image) with `/dev/dri` mounted, a privileged container (or a GPU device plugin), and `supplementalGroups` set to the host gids of `/dev/dri/renderD128` and `card*`.
- **Player UI** is `web/src/components/player`. `PlayerFrame` owns the `<video>`, controls, keyboard, auto-hide and full screen (on its root, which is `data-theme="dark"`). `SettingsMenu` takes `SettingSection[]`, and `useMediaState` snapshots the video. Pages (`PlayerPage`, `LivePlayerPage`, `RecordingPlayerPage`) only own sources and settings. Timelines are `vod`, `live` (seekable range, GO LIVE via `hls.liveSyncPosition`), or `recording` (wall-clock start/end, with a hatched unrecorded part).
- **Tracks**: `GET /api/files/{id}/streams` lists audio and subtitles, including sidecars (`x<N>` keys). `GET /api/files/{id}/subtitles/{s|x}<N>.vtt` converts to WebVTT, cached as `$CACHE/subs/<file>-<key>-<mtime>.vtt`. Embedded tracks are fetched by the player in 90s chunks (`s<N>.c<K>.vtt`), because a whole track means reading the entire file: 7 minutes for a 30 GB remux over SMB, versus about 7s per chunk. Chunks use `-ss from -copyts -start_at_zero … -to <absolute end>`, since an input `-t` is ignored with copyts. The player dedupes cues across chunk overlaps. HLS sessions take `audioIndex` (`-map 0:a:N`) and `burnSubtitle`. Burn-in uses `-filter_complex [1:s:N][0:v:0]scale2ref=w=main_w:h=main_h…overlay` with a **second input seeked 10s earlier**, because PGS captions need earlier data. ffmpeg 6.1's scale2ref uses `main_w`/`main_h`, not `rw`. `Encoder.VideoParts` returns the bare filter chain for this.
- Test media for the tricky paths: HEVC 10-bit + AC3 in MKV (transcode), H.264 in MKV (remux), and HEVC tagged smpte2084 (tone map). Generate them with ffmpeg lavfi.

## Live TV & DVR (internal/livetv, web components/livetv, LivePlayerPage)

- It's off unless `COUCHSIDE_HDHOMERUN` is set. The HDHomeRun HTTP API supplies `discover.json`, `lineup.json` and `status.json`. Stream URLs come from the lineup (`:5004/auto/v<ch>`), and HTTP 503 means every tuner is busy.
- **Guide**: `api.hdhomerun.com/api/guide.php?DeviceAuth=...&Start=<unix>`, about 4h per page, paged to about 26h. It **rejects Go's default User-Agent with 403**, so keep the explicit UA. DeviceAuth rotates, so re-read `discover.json` before each fetch and never expose it.
- **Channel prefs**: `channels.pinned` (via `PUT /api/livetv/channels/{n}/pin`) sorts first in `Channels()`. `signal_strength`/`signal_quality` come from the lineup's scan readings on each refresh. `ReplaceChannels` upserts, so pins survive lineup refreshes. Filtering is client-side, in `components/livetv/filters.ts` and `FilterBar.tsx`, with state shared by the Guide and Channels tabs and kept in localStorage.
- **Live**: one ffmpeg per channel and height, shared by viewers. It writes an HLS EVENT playlist with 2s segments, deinterlaces with yadif (interlaced frames only), and waits for the first playlist before returning. It's reaped 20s after the last request.
- **Recordings folder**: the `settings` table key `dvr.recordings_dir` overrides `COUCHSIDE_RECORDINGS_DIR`; always call `Service.RecordingsDir(ctx)`, never `cfg.RecordingsDir`. `pathInDir` reuses a matching show folder (by `parse.Name` plus `Normalize`) and season folder. `ensureLibrary` uses the library that *contains* the folder, and only creates "DVR Recordings" when none does. Delete only removes the row's own `.ts` file, because the folder may be a shared TV library. The media mount is read-write for this; nothing else writes there.
- **Shutdown during a recording** leaves the `.partN.ts` files and the `recording` status alone. `recoverInterrupted` resumes into the same path, via `existingParts`, which also picks up a finished file from older builds, and joins everything at the end. Never finalize on `parent.Err()`, or the file splits in two.
- **Watch from start**: `POST /api/dvr/recordings/{id}/watch` runs a live-style HLS session whose input is the growing part file (`-follow 1`). The player uses `startPosition: 0`.
- **Recording**: the scheduler ticks every 5s. It runs `ffmpeg -c copy -t <remaining>` into `.partN.ts`, retries on drops, and joins parts with `concat:`. Plex-style names are built in `recordingPath`. Restart recovery lives in `recoverInterrupted`. Each finished recording enqueues a scan of the auto-created "DVR Recordings" library, and `underRoot` allows the recordings dir outside `MEDIA_ROOT`.
- **Series rules** (`livetv/rules.go`, table `series_rules`) are keyed by the guide's SeriesID. `evaluateRule` walks upcoming airings and handles the cases in this order:
  - DRM and channel filter.
  - An existing recording at that airing: cancelled ones are never revived, and failed ones can be retried.
  - The mode.
  - The library check (`LibraryEpisodeKeys`) and the recorded check (`RecordedEpisodes`).
  - Same-episode repeats.
  - Tuner conflicts: skip this airing, and a later one can take the episode.

  Episode keys are `S11E4` (unpadded), or `t:<normalized show>|<normalized episode title>`. Rule-owned scheduled rows the rule no longer wants are deleted (`UnscheduleStale`), except anything starting within 30s. `LibraryMatchesFor` ranks same-titled shows by matching upcoming episode titles. Keep-last-N runs in `afterRecording`. Tests in `livetv/rules_test.go` run on a temp SQLite DB.
- Tuners can be shared with other DVRs (e.g. Plex). ATSC 3.0 channels with DRM (HEVC/AC-4) are listed but shown locked.

## Styling

- `web/src/index.css` starts from Portside Lite's tokens and component classes (`.card`, `.btn-*`, `.field`, `.navtab`, `.table`, `.tint-*`). Media additions follow the same naming: `.poster`, `.still`, `.row-title`, `.art-badge`, `.chip`, `.hero-fade`, `.poster-placeholder`.
- Put new reusable styles in `index.css` under `@layer components`. Don't hard-code colours; use the tokens so light mode keeps working.

## Next milestones

1. Audio track choice for live TV (secondary audio / SAP).
2. TMDB provider for backdrops, then auth (local users or OIDC).
3. An ffmpeg build with NVENC or QSV GPU runtime (for example jellyfin-ffmpeg) if those are needed.
