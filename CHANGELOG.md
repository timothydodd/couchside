# Changelog

Each release lists **Breaking changes and upgrade notes** first (when there
are any), then what's new and what's fixed. The GitHub release links here;
its own notes list every merged change.

## 0.20.3

### Fixed

- **Phone search box lost focus on the first keystroke.** On a phone the
  search box sits on the Search page, and the page swapped the box's
  wrapper when results arrived, which recreated the input and dropped the
  keyboard. The box now stays put while you type.

## 0.20.2

### Added

- **Hide a shared profile's devices.** Accounts → an account → "Can see and
  sign out their own devices", off for a profile several people share (a
  demo server's guest, a household's "Everyone"): that profile's Settings
  no longer lists its signed-in devices and addresses, and it can't sign
  the others out (`hideDevices` on the account; the sessions routes answer
  403). Admins still see and end them in Accounts.

### Fixed

- **Helm: every sign-in showed the node's address** (`10.42.0.1` on k3s)
  in Settings → Accounts → sessions and the log when the chart's Service
  was a NodePort or LoadBalancer. The chart now sets
  `externalTrafficPolicy: Local` on those (`service.externalTrafficPolicy`),
  which keeps the visitor's address. Existing installs pick it up on the
  next `helm upgrade`; a cluster with several nodes that wants every node
  to answer can set it back to `Cluster`.

## 0.20.1

### Changed

- **The Windows installer ships Couchside's own FFmpeg** instead of
  jellyfin-ffmpeg: upstream FFmpeg 8.1.3, built static from unmodified
  sources with only what Couchside uses (NVENC/NVDEC and the CUDA filters,
  Intel Quick Sync, OpenCL tone mapping, libx264, zimg, libwebp). NVENC
  still works on NVIDIA driver 531.61 or newer. HDR on NVIDIA is tone
  mapped by OpenCL (`tonemap_opencl`), which the server already preferred
  when `tonemap_cuda` wasn't there. The build script and pins are in
  `deploy/ffmpeg/`; the source tarball on each release is now
  `couchside-ffmpeg-<version>-src.tar.gz`.

### Added

- **A warning when a network share can't be signed in to.** Admins see a
  banner on every page naming the share and the reason, with a link to
  Settings → Server → Media locations, instead of finding out from failed
  jobs; `GET /api/status` carries it as `shareErrors` (admins only).

### Fixed

- **HDR tone mapping on NVIDIA with more than one OpenCL device.** On a
  machine with an Intel iGPU or a CPU OpenCL runtime beside the NVIDIA
  card, ffmpeg refused to pick an OpenCL device ("More than one matching
  device found") and HDR fell back to the CPU. The server now asks for the
  NVIDIA GPU by vendor and only then for whatever single device there is.

## 0.20.0

### Added

- **Box sets and bonus discs.** A video anywhere below a subfolder of a
  movie's folder (`Looney Tunes Platinum Collection (2011)/Disc 3 - Bonus
  Materials/Bonus Cartoons/05 - The Door (1968).mp4`) is now filed under
  that movie instead of appearing on the Movies page: a title of its own,
  matched on its own name (a leading track number is dropped), with its own
  poster, details and watched state, listed on the movie's page under a
  heading named after the subfolder. The movie folder becomes a title even
  with no video of its own; Play on it plays what's filed under it in order,
  and Mark watched covers them all. Disc folders (`Disc 1/`) and Plex extras
  folders (`Featurettes/`) are read as before. Settings → Server → **File
  nested folders under their movie** turns the rule off.
- Admins can file any movie under another from its page (**⋯ → File
  under…**, and **Unfile**), or by selecting titles on the Movies page and
  choosing **Merge → Titles of their own, filed under it**. Choices made by
  hand stick through rescans.
- `GET /api/server` lists the feature `childTitles`; `GET /api/items` leaves
  filed titles out unless `includeChildren=1` ([api-compat.md](docs/api-compat.md)).

## 0.19.14

### Fixed

- On a show with more seasons than fit across the page, the season tabs
  can now be paged with arrows beside them (with a mouse; fingers swipe),
  like a row of cards.
- Activity's Target column showed the job's verb for preview thumbnails,
  intro detection, commercial detection and HDR reads ("thumbnails
  Film.mkv"); it now names the file or title alone.

## 0.19.13

### Added

- In the web player, the title in the top bar opens the title's details
  over the picture: the poster and plot, a show's seasons and episodes
  (the one playing marked, its season open) and a movie's extras. Pick an
  episode to play it, or open the title's page from there.
- On phones the player's play/pause and previous/next (or back and forward
  ten seconds for a film) sit over the picture, and the bar below is the
  seek bar, the time and the queue, settings and full-screen buttons. A
  double tap on the left or right of the picture goes back or forward ten
  seconds (each further tap adds ten more); a double tap in the middle
  still toggles full screen.

## 0.19.12

### Changed

- The About block at the bottom of Settings is shown to every profile and
  now carries the TMDB and OMDb credits, a trademark line and a reminder
  that Couchside provides no content: use it with media you have the right
  to play.
- The full GPL-2.0 and GPL-3.0 texts ship with FFmpeg and Comskip, in the
  image (`/usr/share/licenses/couchside/`) and the Windows installer.
- The privacy policy now covers the TV apps' playback reports and the
  outside addresses an admin sets up (single sign-on, stream channels).
- The stream-channel help asks for streams you have the right to watch.

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
