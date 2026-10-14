# Changelog

Each release lists **Breaking changes and upgrade notes** first (when there
are any), then what's new and what's fixed. The GitHub release links here;
its own notes list every merged change.

## 0.19.11

### Changed

- Playback-test results from the LG app's debug build are kept in
  `playtest.jsonl` in the data folder as well as the log, so a run survives
  the server restarting or upgrading.

## 0.19.10

### Fixed

- The very end of a converted stream no longer fails with a server error.
  When a file's last moment was a fraction of a second of sound after the
  last picture, asking for it started a conversion with nothing to convert,
  which the GPU encoder refused; the stream now just ends there.

## 0.19.9

### Fixed

- A letterboxed film converted at Auto quality keeps its full picture. A
  1920×818 film was boxed by its own height and shrunk to 1455×620, at
  720p's bitrate; it now converts at 1080p (1920×818, 8 Mbps). Optimized
  copies of such films had the same fault.

## 0.19.8

### New

- Settings → System → over time has a **Network** chart: what the server
  sends to players and receives, and, when the library is on NFS, what's
  read from it. A stuttering stream can be traced to the storage, the server
  or the player's link. Measured on Linux (Docker and Kubernetes included);
  Windows shows 0.

### Fixed

- When a 4K film whose picture is being copied keeps buffering on Auto
  quality, the web player now steps down to a 1080p conversion first
  instead of going straight to 720p.

## 0.19.7

### Changed

- Opening a title no longer grows the clicked poster into the title page's.
  Pages still slide in going forward and back, and fade between sections.

## 0.19.6

### Fixed

- Opening a title from a list for the first time no longer stalls for about
  four seconds and logs "Transition was aborted because of timeout in DOM
  update". The page change waited for the title's poster by asking for an
  animation frame, which the browser doesn't give while a view transition
  is updating the page.

## 0.19.5

### New

- Ready for the LG webOS app (`couchside-webos`). Apps installed on the TV
  run from `file://`, so the API now answers CORS for that origin (`null`)
  only, never with credentials, so cookies and the web's sessions stay out
  of it. The stream and subtitle routes also take the access token as
  `?access_token=`, because a `<video>` or `<track>` element can't send a
  header.

## 0.19.4

0.19.3 was tagged but never released: its release was stopped to add the
diagnostics below. 0.19.4 has everything 0.19.3 would have.

### New

- A transcode that makes video slower than it plays is logged once per
  session ("transcode is slower than real time", with its speed), so constant
  buffering has an explanation in System → Console. A failed GPU run logs more
  of ffmpeg's output.

### Fixed

- 4K films whose keyframes are far apart no longer fail on the Roku when
  their picture is passed through: a copied picture can only be cut at the
  film's own keyframes, so a film with keyframes 10s apart made segments of
  about 50 MB, more than the Roku's player holds. A client can now send
  `maxSegmentBytes` with `POST /api/files/{id}/hls`; the server measures the
  copy's segments and converts the picture when they would be bigger.

## 0.19.2

### Fixed

- A wide film that has to be converted is now fitted inside 1920×1080 (or
  the 16:9 box for the quality picked) instead of being scaled by height
  alone: a 1.85:1 4K film came out 1998×1080, and a 2.39:1 one about
  2580×1080, wider than a TV's H.264 decoder may play. Optimized copies are
  fitted the same way.

### New

- TV apps can send problem reports (`POST /api/client/log`, feature
  `clientLog`): when playback fails on a Roku, what its player said and what
  it was playing appear in System → Console as `client report`, next to the
  server's own lines about the stream.

## 0.19.1

0.19.0 was tagged but never released: its build failed the Go vulnerability
check. 0.19.1 is the same release, built with Go 1.27.2.

### Breaking changes and upgrade notes

- **Helm:** `ingress.enabled=true` now needs `auth.enabled=true` (passwords
  required), or `auth.allowOpenIngress=true` for an ingress only your LAN can
  reach. The chart refuses to render otherwise.
- **Helm:** `hwaccel.dri.privileged` defaults to `false`. If you mount
  `/dev/dri` (`hwaccel.dri.enabled=true`), set `privileged=true` yourself, or
  move to a GPU device plugin with `hwaccel.dri.resource`. `hwaccel.mode`
  offers `none` and `vaapi` (the image's ffmpeg has no NVENC or Quick Sync).
- **Helm:** liveness now checks `/livez`; there's a 60s
  `terminationGracePeriodSeconds`, and `GOMEMLIMIT` is set (`goMemLimit`).
- **Windows:** the service runs as its own account, `NT SERVICE\Couchside`,
  instead of Local System. Type network-share passwords again once (Settings
  → Server → Media locations), and grant the account Modify on any local
  folder the DVR records into.
- **Docker:** `/recordings` is no longer a volume of its own: mount one
  (`-v couchside-recordings:/recordings`) for the DVR.
- **Compose:** `docker-compose.yml` now runs with media from a local folder;
  the NAS/SMB setup moved to `docker-compose.smb.yml`.
- **First run:** until setup is done, only the home network can claim a new
  server; from anywhere else it needs the setup code from the log, and a
  password.
- **Settings → Server:** the ffmpeg, ffprobe and comskip programs can no
  longer be chosen from the web; set them in the environment or the
  settings file. Values saved from the web are dropped, with a warning.
- **Going back:** an older version refuses to open a database this one has
  upgraded; restore `backups/couchside-upgrade-<time>.db` to go back.

### Added

- `GET /api/server`: version, API version and features, for apps to check
  before signing in (docs/api-compat.md).
- Diagnostics download for bug reports (Settings → System → Advanced).
- Renamed files keep their watch history, optimized copy, commercial
  breaks and intro marks.
- The Helm chart is published at `oci://ghcr.io/timothydodd/charts/couchside`.
- Releases attach the source of the ffmpeg in the Windows installer.

### Fixed

- Built with Go 1.27.2, which fixes security issues in Go's HTTP/2, HTTP,
  TLS and header parsing (GO-2026-6603 to GO-2026-6617).
- A link in a media folder could serve any file the server could read.
- Security headers: a full Content-Security-Policy for the web UI, HSTS
  over HTTPS, Referrer-Policy.
- Big libraries: the Movies and TV lists, status polls, scans, job claims,
  search and HLS disk use are much lighter (see the release notes).
