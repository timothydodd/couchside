# Server stories

Work on the server and web app, planned from the second code audit
(3 October 2026). Four lists:

- **Fixes** (`B1` to `B23`): bugs, security gaps, packaging and docs drift.
- **Cleanup** (`C1` to `C6`): dead code, duplication, files to split.
- **Features** (`F1` to `F10`): what people coming from Plex and Jellyfin ask
  for most, sized for this codebase.
- **Carried over** (`S5`, `S7`, `S8`): what's left of store readiness.

The Roku app's share is in
[couchside-roku](https://github.com/timothydodd/couchside-roku)
`docs/stories.md`, as `R12` to `R18`. `R-n` here refers to a Roku story.

Finished stories are removed from this file; git history and the pull requests
have them (the first audit's fixes, A1 to A17, are PRs #8 to #26).

Suggested order: **B1 → B5** (security, and metadata being wiped), **B6 → B12**
(live TV, DVR, transcode), **B13 → B18** (web), then **F1 → F3** (small
features with the most demand), **B19 → B23** and **C1 → C6** as filler
between features, then **F4 → F10**. `R12` (certification) can run alongside
from the start.

| ID  | Story                                                  | Kind      | Finding   | Done |
| --- | ------------------------------------------------------ | --------- | --------- | ---- |
| B1  | Turning passwords on ends passwordless sessions        | Security  | Confirmed | Done |
| B2  | Sign-in hardening, second pass                         | Security  | Reported  | Done |
| B3  | Remote image cache can't be filled between prunes      | Security  | Reported  | Done |
| B4  | Internal error text stays on the server                | Security  | Reported  | Done |
| B5  | A provider outage doesn't wipe cast and backdrops      | Data loss | Confirmed | Done |
| B6  | Virtual channels stay on their schedule                | Bug       | Confirmed | Done |
| B7  | Virtual and tuner streams use the right picture path   | Bug       | Reported  | Done |
| B8  | A recording watched from the start outlives recording  | Bug       | Confirmed | Done |
| B9  | Commercial detection: custom ini and the job lookup    | Bug       | Confirmed | Done |
| B10 | DVR: stale series matches and failed joins             | Bug       | Reported  | Done |
| B11 | Transcode: false GPU fallback, copied HEVC             | Bug       | Reported  | Done |
| B12 | Server odds and ends                                   | Bug       | Mixed     | Done |
| B13 | Player: subtitles, audio track, retry                  | Bug       | Confirmed | Done |
| B14 | Live player: failed Record, phone controls             | Bug       | Confirmed | Done |
| B15 | `useApi`: late answers, stale errors, cache size       | Bug       | Reported  | Done |
| B16 | Console polling and token renewal                      | Bug       | Confirmed | Done |
| B17 | Load failures and failed actions are shown             | Bug       | Reported  | Done |
| B18 | Keyboard and screen-reader gaps                        | A11y      | Reported  | Done |
| B19 | Docker and compose                                     | Packaging | Reported  | Done |
| B20 | Helm chart hardening                                   | Packaging | Reported  | Done |
| B21 | CI and release                                         | Packaging | Reported  | Done |
| B22 | Docs and site catch up                                 | Docs      | Reported  | Done |
| B23 | Demo media script                                      | Hygiene   | Reported  | Done |
| C1  | Server dead code and stale comments                    | Cleanup   |           | Done |
| C2  | One builder for ffmpeg's HLS arguments                 | Cleanup   |           | Done |
| C3  | Split `api/auth.go` and `api/handlers.go`              | Cleanup   |           | Done |
| C4  | One HLS hook for the three players                     | Cleanup   |           | Done |
| C5  | Web duplication, dead CSS, player colour tokens        | Cleanup   |           | Done |
| C6  | Load pages on demand                                   | Cleanup   |           | Done |
| F1  | Next Up, and remove from Continue Watching             | Feature   | S         | Done |
| F2  | Watchlist                                              | Feature   | S         | Done |
| F3  | Database backup and restore                            | Feature   | S         | Done |
| F4  | Seek-bar preview thumbnails                            | Feature   | M         | Done |
| F5  | Skip intro and credits                                 | Feature   | M to L    | Done |
| F6  | Per-profile libraries and rating limit                 | Feature   | M         | Done |
| F7  | Versions and editions                                  | Feature   | S to M    | Done |
| F8  | Surround sound passthrough                             | Feature   | S to M    | Done |
| F9  | Sign in with OIDC                                      | Feature   | M         | Done |
| F10 | Two-factor sign-in                                     | Feature   | S to M    | Done |

**How to read these.** "Confirmed" means the defect was read in the code and
checked by a second pass. "Reported" means one reviewer traced it by reading
and nobody re-checked: **confirm it in the code before fixing, and drop the
item (noting why in the PR) if it doesn't hold.** Nothing was reproduced at
runtime. Line numbers are from commit `1acefb8` and will drift; the function
names are the stable reference.

**Rules for every story**
- All of this goes on one branch, `audit-2`, with one commit per story
  (`B4: …`), and becomes a single PR when the release is decided. Don't fold
  neighbouring cleanups into a story's commit.
- Write the failing test first where the story lists one. Tests for `livetv`
  and `db` run on a temp SQLite DB (see `livetv/rules_test.go`).
- Verify with the commands in `CLAUDE.md`: `go vet ./... && go test -race
  ./...` in `server/`, and `npx tsc --noEmit && npx vite build` for `web/`
  (copy `web/` to a scratch folder first; npm on `/mnt/f` is slow).
- If behaviour described in `CLAUDE.md` or `docs/` changes, update it in the
  same PR.
- A story that changes what the Roku sees names the Roku story that follows it.
- Paths below are under `server/internal/` unless they start with `web/`,
  `deploy/` or `.github/`.

---

# Fixes

## B1 · Turning passwords on ends passwordless sessions

**Done.**

**Why.** `Server.Run` (`api/api.go:74`) calls `s.pruneSessions(ctx)`, which
loops on a 6-hour ticker until shutdown. The `if s.cfg.Auth {
endPasswordlessSessions }` block after it only runs when the server is
stopping. So setting `COUCHSIDE_AUTH=true` on a server that was passwordless
leaves every profile without a password signed in, on a session that renews
for a year. This was A7's item 4.

**Scope**
- Run `go s.pruneSessions(ctx)`, or move the `cfg.Auth` block above it.
- Test: start a server with `cfg.Auth` and a session for a profile with no
  password; the session is gone once `Run` has started.

**Done when** the test passes.

## B2 · Sign-in hardening, second pass

**Done.**

**Scope**
- **The throttle is check-then-act.** `throttled` (`api/auth.go:389`) reads
  `Limiter.Wait` before the Argon2 verify and `loginFailed` (`:412`) records
  after it, so N parallel requests for one name each get a real check. Reserve
  the attempt in one locked call before verifying and clear it on success.
  Same in `changePassword` and `setup`. Test: 50 concurrent wrong logins make
  at most the allowed number of `VerifyPassword` calls.
- **Login CSRF.** `login` (`:366`) and `setup` (`:594`) skip the `sameOrigin`
  check that `pick` and `refresh` have, and `decode` ignores Content-Type, so
  a cross-site form post can sign a browser into someone else's profile.
  Apply `sameOrigin` to web sign-ins (TV clients send no Origin and get JSON
  tokens).
- **TV clients and a lost refresh answer.** If the server rotates a refresh
  token and the answer never arrives, the Roku holds the old token: 409
  `refresh_stale` for 60s, then the session is deleted (`RotateRefresh`,
  `db/accounts.go:181`). The grace assumes a browser's cookie jar already has
  the new token. For JSON-token clients, keep the previous hash valid until
  the new one is first used (then retire it). Pairs with R13.
- **Typed names in the log.** "sign-in failed" (`:448`) logs the name as
  typed, and a password typed into the name box shows in Settings → Console.
  Log the name only when it matches a profile.

**Done when** the concurrency test passes, a cross-origin login is refused,
and a TV client that repeats a refresh with the previous token gets a session.

## B3 · Remote image cache can't be filled between prunes

**Done.**

**Why.** `GET /api/artwork/remote` needs no sign-in, and the 2 GB
`remoteMaxTotal` is only enforced by `pruneRemote` on the daily ticker
(`api/remote.go`, `api/api.go:77`). Someone walking distinct
`image.tmdb.org/t/p/original/...` paths (up to 10 MB each) can fill the cache
volume in between. A6 left this.

**Scope**
- Keep a running total of the folder (set at start-up and by `pruneRemote`,
  added to by `fetchRemote`). Over the cap, evict least recently used before
  storing, or refuse new fetches for signed-out callers.
- Test: with a small cap, fetching past it never leaves the folder over it.

**Done when** the test passes.

## B4 · Internal error text stays on the server

**Done.**

**Why.** A7 made 500s say "internal error" unless the message is a `usererr`,
but several handlers still wrap anything.

**Scope**
- `hlsSegment` (`api/encoding.go`, the `case err != nil` branch) answers with
  `"transcode failed: " + tail(ffmpeg stderr)`, which has server paths.
- `dvrDelete` (`api/livetv.go`) wraps `os.Remove`'s `PathError` in
  `badRequest`.
- `dvrRecord`, `dvrWatch`, `ruleCreate`, `ruleUpdate` turn DB errors into
  400s; `fileStreams` (`api/subtitles.go`) sends ffprobe's error as a 502.
- `GET /healthz` (`api/api.go:299`) returns `"db: " + err.Error()` to anyone.
- `commercials` (GET) returns the failed job's raw `job.Error` to users.
- `status` (`api/handlers.go:23`) gives every user `mediaRoot`; `tvStatus`
  already hides folders from non-admins.
- In each, send only `usererr` messages and route the rest through
  `writeErr`'s default; give admin-only fields to admins.

**Done when** none of those answers contains a filesystem path or driver
error for a non-admin, with a test for `hlsSegment` and `dvrDelete`.

## B5 · A provider outage doesn't wipe cast and backdrops

**Done.**

**Why.** `Chain.Lookup` (`metadata/metadata.go:77`) moves on to the next
provider when one errors and returns that answer with a nil error. When TMDB
is rate-limiting and OMDb answers, `match` (`worker/match.go:43`) writes an
empty `backdrop_url` and `SetCredits` deletes every credit. The item is
`matched`, so `retryMatches` never comes back. "Re-match library" during a
TMDB 429 degrades the whole library.

**Scope**
- Have `Lookup` report that an earlier provider failed (a flag on the result,
  or a wrapped error beside the details).
- In `match`, when that's set: keep the existing credits and backdrop, and
  leave the item `pending` so the next scan retries it.
- Test in `metadata`: first provider errors, second matches. Test in `worker`:
  an item with credits keeps them through such a match.

**Done when** both tests pass.

## B6 · Virtual channels stay on their schedule

**Done.**

**Scope**
- **A failed piece puts the stream ahead of the guide.** `playVirtual`
  (`livetv/virtual_stream.go:106`) sleeps 1s after a failed `runPiece` and
  sets `pos = end`, so the next piece starts at once and the stream runs ahead
  by the rest of the failed piece for the life of the shared session (the
  resync only handles being behind). A file shorter than its stored duration
  does the same; an unmounted share burns through 36h at a piece a second.
  After a failed or short piece, wait until the schedule reaches `end`.
- **A missing file that's "on now" makes the channel untunable.** On a first
  run (`pl.count() == 0`) the failure is returned as "couldn't play X". Try
  the next piece before giving up.
- **Saving a channel races the schedule builder.** `SaveVirtual`
  (`livetv/virtual.go:606`) runs `UpdateVirtualChannel` outside `virtualMu`.
  An `extendVirtual` already running inserts 36h of old-config pieces, and
  the save's own extend then sees enough scheduled and returns. Take
  `virtualMu` around the update in `SaveVirtual` and `DeleteVirtual`.
- **Guide rows are never pruned without a tuner.** `PrunePrograms` is only
  called from `refreshGuide` (`livetv/service.go:287`). Call it from
  `extendAllVirtual`, and drop `filler_clips` rows not seen in the walk.
- **A tuner channel with the same number takes the guide.** `refreshGuide`
  writes that number's programs and its page-window delete removes the
  virtual channel's rows. Skip guide entries whose channel is virtual.
- **Renumbering can hit a favourites key.** `UpdateVirtualChannel`
  (`db/virtual.go:112`) moves `profile_channels` rows to the new number; a
  stale favourite already there breaks the primary key. Delete it first.
- Tests: `playVirtual` with a failing piece stays on schedule; save versus a
  running extend; `mergedPlaylist.follow` (window slide, `discSeq`).

**Done when** the tests pass and a channel whose current file is deleted
plays the next programme at its scheduled time.

## B7 · Virtual and tuner streams use the right picture path

**Done**, in part. A virtual channel now probes each file once per session
and tone maps HDR films, and decodes on the GPU when the start-up test passed
(falling back to the CPU for the rest of the session if a GPU run fails).

**Dropped, and why**
- *SD files are upscaled to the session height.* Kept on purpose: the pieces
  are joined into one stream, and the picture size changing at every join is
  worse than the upscale.
- *Tuner streams pass no `SrcHeight`.* The lineup only says HD or not, never
  a height, so there's nothing to pass.

## B8 · A recording watched from the start outlives recording

**Done.**

**Scope**
- **Cut off 30s after the recording ends.** The follow-ffmpeg
  (`startRecordingPlayback`, `livetv/live.go:231`) has `-rw_timeout 30s` and
  exits once the part file stops growing. The reaper (`liveManager.run`,
  `:438`) stops any session with `!s.running()` and removes its folder, so a
  viewer 20 minutes behind loses the rest. For `rec:` sessions, keep an exited
  session (its EVENT playlist is complete) until it's idle.
- **Wrong error for the second viewer.** `launch` (`:356`) returns "the tuner
  didn't deliver video in time" when the first caller's context ended, so
  `claimLocked`'s "first caller gave up, try again" branch (`:152`) never
  fires. Return `ctx.Err()` there, as `launchVirtual` does.
- Tests: `liveManager.run` with an exited `rec:` session; `claimLocked` when
  the first caller cancels.

**Done when** the tests pass.

## B9 · Commercial detection: custom ini and the job lookup

**Done.**

**Scope**
- With `COUCHSIDE_COMSKIP_INI` set, `comskipINI` (`worker/commercials.go:182`)
  returns before creating `$CACHE/comskip`, and `os.MkdirTemp` in
  `commercials` (`:124`) fails on every job. `MkdirAll` the folder first.
- `LatestJob` (`db/commercials.go`), polled by the player, filters on `kind`
  and `ref_id` with no index. Add `CREATE INDEX jobs_ref ON jobs(kind,
  ref_id, id)` in a new migration.
- Activity shows `commercials` jobs with the raw kind
  (`web/src/pages/ActivityPage.tsx:9`, `KINDS`). Add the entry.

**Done when** a job runs with a custom ini on an empty cache (test), and the
migration applies.

## B10 · DVR: stale series matches and failed joins

**Done.**

**Scope**
- `showYear` (`livetv/identify.go`): once a stored match is past its TTL, a
  failed lookup returns 0 instead of the stored year, so "MacGyver (2016)"
  records into plain "MacGyver" while the provider is down. Return the stale
  year on error. Test it.
- On a full disk each retry leaves a small `.partN.ts`; `joinParts` fails, the
  row goes to `failed`, and once the recording isn't active `rePartFile`
  (`worker/scan.go:153`) stops hiding the parts, so a scan indexes them as
  episodes. `recoverInterrupted` (`livetv/recorder.go:395`) also stores an
  empty path on a failed join. Keep the row's path on failure, and have the
  scan skip `.partN.ts` whenever a recording row owns that base path.
- Offer "retry join" on a failed recording that still has parts.

**Done when** the `showYear` test passes and parts of a failed recording
don't appear in the library.

## B11 · Transcode: false GPU fallback, copied HEVC

**Done.**

- The stale waiter was real: a restart lets go of the session lock while the
  old ffmpeg dies, and a request waiting on a segment could then turn GPU
  decoding off for good. `gpuFallback` now ignores a run that has since been
  replaced, and `Segment` checks the request is still wanted first.
- The log no longer says "GPU decoding failed" for a failure that may not be
  the GPU's.
- The Windows comment and `docs/install.md` say that ffmpeg isn't paused
  there.

- Copied video (`Request.VideoCodecs`): the client's codec names are
  normalised (`codecName`), and Dolby Vision profile 5 (`probe.Info.DVProfile`)
  is never copied.

**Left for a device:** check on a Roku that HEVC with 10s keyframe spacing
plays against a playlist listing 4s segments (R15).

## B12 · Server odds and ends

**Done.**

**Scope**
- `createAccount` (`api/accounts.go:94`) drops the errors from
  `SetPasswordLocked` and `SetPassword` and still answers 201. Pass `locked`
  into `db.CreateAccount` so it's one INSERT.
- `uploadArtwork` (`api/manage.go`) writes a fixed `upload.orig`, so a poster
  and a backdrop upload for one item collide, and it removes `<kind>.src`
  before the upload succeeds. Use `os.CreateTemp`; remove `.src` after
  `SetCustomArtwork`.
- The per-key lock maps (`subLocks` in `api/subtitles.go`, `remoteLocks` in
  `api/remote.go`, `photoLocks` in `worker/match.go:253`) delete the entry
  after unlock, so a third caller can run beside a waiter. Write one
  refcounted (or singleflight-style) helper and use it in all three.
- `poster()` and `backdrop()` leave a partial `.orig` when `download` fails
  mid-body (the `defer os.Remove` comes after the error return).
- `search` (`api/search.go`) has no cap on `q`; clip it to about 100 runes.
- The SSDP responder (`discovery/ssdp.go`, `Run` → `reply`) starts a goroutine
  and a settings query per packet. Cache the sign-in mode for a few seconds
  and limit replies per source.
- `people` rows and `$CACHE/people/*.webp` are never pruned. Drop people with
  no credits in `pruneTables`.
- `setFileRole` and `enqueueOptimize` call `s.db.Enqueue`; the rest use
  `s.worker.Enqueue`, and `setFileRole` never wakes the worker. Use the
  worker's.

**Done when** each has a test or a noted reason it can't have one.

## B13 · Player: subtitles, audio track, retry

**Done.** Typechecked and built; not yet tried in a browser.

- Subtitle chunks: a failed chunk is retried after 5s, 10s and 20s, never
  after a 404 or 422, and chunks past the end aren't asked for.
- A server stream for "the default audio track" waits for the track list.
- `lib/hls.ts` loads hls.js for all three players and forgets a failed load.
- "Try again" reloads the file info when that's what failed.
- The recording player recovers from a decode error twice, then offers
  "Try again".
- The page's own shortcuts run before the modifier check, so Ctrl+Up/Down
  change channel.

- On an iPhone, `full` follows `webkitbeginfullscreen` /
  `webkitendfullscreen` (its own full-screen player fires no
  `fullscreenchange`). Not tried on an iPhone.

## B14 · Live player: failed Record, phone controls

**Done.** Typechecked and built; check the top bar on a phone.

- A failed Record or Stop shows as a notice over the stream, which keeps
  playing.
- "Start over" and "Record"/"Stop" lose their text below `md` (the buttons
  keep an `aria-label`), so the title has room at 360px.

- A recording's "Start over" button in the bottom row is icon-only below
  `sm`, so the row fits at 360px.

## B15 · `useApi`: late answers, stale errors, cache size

**Done.** Typechecked and built; not yet tried in a browser.

- An answer is dropped when a newer request for the same URL has been sent.
- Data, error and loading always belong to the URL passed in: no previous
  page's error or data after the URL changes (or becomes null).
- The cache keeps the 300 most recently used URLs.
- New option `keep`, used by the guide, the folder picker and the history
  charts: they page by changing the URL and relied on the old data staying
  up until the new answer arrives.

**Not done:** the unit test. The web app has no test runner; adding one
(Vitest) belongs with the lint step in B21.

## B16 · Console polling and token renewal

**Done.**

**Scope**
- `web/src/components/settings/Console.tsx:32`: `setInterval` starts a poll
  while the last is in flight; both use the same `after=` and both append,
  giving duplicate lines and keys. Chain polls with `setTimeout`, and skip
  when the tab is hidden.
- `web/src/stores/auth.ts:177` (`schedule`) compares the server's absolute
  expiry with the client clock. A clock 13 minutes fast refreshes every 5s; a
  clock 2 minutes slow renews after the cookie expired, and `<video>` and
  `<img>` can't retry. Have sign-in and refresh return `expiresIn` (seconds)
  and schedule from that. The Roku can use the same field (R13).
- `PlayerFrame`'s keyboard effect and `BreakMenu`'s listeners re-subscribe on
  every render. Add the dependency arrays.

**Done when** a slow `/api/system/logs` shows no duplicates, and renewal
doesn't depend on the client clock.

## B17 · Load failures and failed actions are shown

**Done.** Typechecked and built; not yet tried in a browser with the server
stopped.

- An error note instead of a blank card or endless spinner: System (load and
  history), Your channels, the channel editor's options, a title's files in
  Manage, and Streaming now on Activity.
- A title or person page says "Couldn't load" with Try again unless the
  server answered not found.
- Sign-out failures and unsaved settings are reported; a setting that didn't
  save goes back to what it was.
- A new artwork preview is shown after an upload to an item that had none.
- The channel editor suggests a number once, from a fresh request, and asks
  for the commercials folder when commercials are on.

## B18 · Keyboard and screen-reader gaps

**Done**, except virtualising. Typechecked and built; try each with the
keyboard in a browser.

- Manage: each title is a button that opens its panel; sort headers carry
  `aria-sort`.
- The Filters panel and the More sheet use `useDialog` (focus in, Tab kept
  inside, Escape, focus back).
- Menus (`MenuButton`, the player's settings): focus moves in on open, the
  arrow keys, Home and End move between items (`menuKeys` in `lib/dialog.ts`),
  and Escape closes only the menu.

**Not done:** virtualising the Manage table. Rows are memoised on their
content instead, so the 3-second refetch during a re-match only renders rows
that changed. Virtualising a `<table>` needs trying in a browser.

## B19 · Docker and compose

**Done**, not built: Docker runs on Windows here. Build the image and run
`docker compose up -d` from a release zip before the release.

- `docker-compose.yml` runs the published image; `docker-compose.dev.yml`
  adds the local build, tagged `couchside:dev`.
- The image has a `HEALTHCHECK` on `/healthz`.
- `.dockerignore` leaves out `.env*`, `site`, `branding`, `docs`, `scripts`
  and `.github`.
- CI, release and notices workflows use Node 26 and Go 1.27, as the
  Dockerfile does; `docker.yml` also runs when the lockfiles change.
- `.env.example` has `COUCHSIDE_HWACCEL`.

## B20 · Helm chart hardening

**Done**, not rendered: `helm` isn't installed in WSL. Run `helm lint` and
`helm template` on Windows, and start a pod with the new security context,
before the release.

- `fsGroupChangePolicy: OnRootMismatch`, `seccompProfile: RuntimeDefault`,
  `automountServiceAccountToken: false`.
- A GPU through a device plugin (not privileged) keeps `runAsNonRoot` and
  drops every capability, like the no-GPU case.
- A `startupProbe` (5 minutes by default); liveness loses its initial delay.
- `readOnlyRootFilesystem` (off by default) with emptyDir volumes for `/tmp`
  and an unpersisted `/recordings`.
- `extraVolumes` / `extraVolumeMounts`, and `hwaccel.device` for
  `COUCHSIDE_VAAPI_DEVICE`.

## B21 · CI and release

**Done**, except the web lint. The workflows can only be proven by running
them: watch the first CI run and the next tag.

- `go test -race` in CI and in the release's `verify` job.
- `verify` also runs `third-party-notices.py --check`.
- A tag must match the chart's `version` as well as `appVersion`, and must be
  on `main`.
- Every `actions/*` step is pinned by commit SHA (Dependabot keeps them
  current).

**Not done:** a web lint step. `typescript-eslint` doesn't install against
TypeScript 7 yet (the peer range stops below it), so ESLint with the React
hooks rules, and Vitest for B15's hook test, wait for that.

## B22 · Docs and site catch up

**Done.** Look at the site in a browser before publishing: its colours
changed to the current tokens and haven't been seen rendered.

- `docs/configuration.md` describes Settings by section, with the Console,
  the history charts, search and the merge gap; the three wrong menu paths in
  `live-tv.md` and `playback.md` are fixed; "Not a commercial" is documented.
- `SECURITY.md` and `docs/accounts.md` list everything open without a
  session.
- The data folder's `auth.key` and `server.id` are named for backups.
- The privacy page mentions the in-memory log shown in the Console.
- "7 MB" is gone from the README and the site.
- The site, `theme-color` and the web manifest use the tokens in
  `docs/style.md`; `color-scheme` is `dark`.
- The site has canonical and Open Graph tags per page, and a `404.html`.
- `CLAUDE.md`: Alpine version, `SHA256SUMS.txt`, next milestones.

**Not done:** the privacy page still carries the landing page's unused CSS.

## B23 · Demo media script

**Done.** The script compiles and its `--help` runs; nothing was downloaded.

- Removed "Any Bonds Today?" (an `.ogv`, which isn't scanned), the six shorts
  built on racial or wartime caricature, and the copies taken from a TV
  broadcast, PeerTube, Dailymotion and YouTube.
- Cartoons aren't in the default run (`--only cartoons` fetches them).
- The closing message lists only what was fetched.
- `.gitignore` has `__pycache__/` and `*.pyc`.

**Yours to judge:** many remaining cartoons come from Archive items named
"restored". A restoration can carry its own copyright, which the script's
header says it avoids.

---

# Cleanup

## C1 · Server dead code and stale comments

**Done.**

- DVR handlers ask `s.tv.HasTuner()` (now nil-safe) instead of `s.tv == nil`,
  so a server with only virtual channels answers "Live TV isn't set up" for
  recordings rather than failing further in. The nil checks on the routes
  virtual channels also use stay: tests build a server with no service.
- Removed `db.SetMatchStatus`, `worker.abs`, `idleFor` (both idle times were
  the same), `truncate` (now `clip`, which cuts at a character boundary), the
  second `tail` (now `transcode.Tail`) and `livetv`'s own list of video
  extensions (now `parse.IsVideo`, which also knows `.m2ts`).
- Stale comments fixed; the whole server is gofmt-clean.

**Left as it is:** `db.CreateProfile` is only called from tests, but from
three packages' tests, so it stays in `db`.

## C2 · One builder for ffmpeg's HLS arguments

**Done.** `transcode.HLSOutput` builds the HLS output arguments and
`transcode.ForceKeyFrames` the keyframe grid, for a file's session, live TV
(tuner streams and recordings watched from the start) and a virtual
channel's pieces. Argument tests cover the three shapes; the live TV test
that runs real ffmpeg still passes.

## C3 · Split `api/auth.go` and `api/handlers.go`

**Done.** Moves only, no behaviour change.

- `auth.go` keeps the state, the session cache and the middleware;
  `signin.go` has sign-in, refresh, setup and sign-out; `password.go` the
  password change; `sessions.go` the session list and pruning.
- `handlers.go` keeps status and Home; `libraries.go`, `items.go`,
  `playback.go`, `artwork.go` and `jobs.go` have the rest.

## C4 · One HLS hook for the three players

**Done**, as shared pieces rather than one hook: the three players differ in
their hls.js settings and error recovery, so a single `useHlsSession` would
have been mostly options. Checked in a browser: a film and a virtual channel
both play.

- `lib/hls.ts` `hlsEngine(what)` decides hls.js, the browser's own HLS, or
  an error, for all three players.
- `PlayerPage.tsx` is 581 lines (from 674): text subtitles are
  `useTextSubtitles` and progress reports `useProgressReports`, both in
  `components/player/`.

## C5 · Web duplication, dead CSS, player colour tokens

**Done.** Checked in a browser: the live player's controls look as before.

- Red fills (RECORDING, LIVE, Stop) use `text-on-accent`.
- No colour is written in a component any more: the player uses
  `player-fg` / `player-bg`, backdrops behind dialogs use `backdrop`, and the
  scrim, channel-logo plate, subtitle cue and seek-bar hatching read tokens
  in `index.css`.
- Removed `.toolbar-field` and the unused `streamUrl`.
- `errText(e)` (`lib/errors.ts`) replaces the error-message expression in 18
  files; `fmtVersion` the version label in three.

**Left as they are:** the quality options (the three lists differ in what
they offer) and the buffered-range loops (each wants a different number).
The large files named in the story are split when next touched.

## C6 · Load pages on demand

**Done.** The three players, Live TV, Settings, Activity, Libraries and
Manage load when first opened (`lib/lazyPage.tsx`, which shows a Reload
message if a page's file can't be fetched). The entry file went from 525 kB
to 339 kB (155 kB to 103 kB gzipped), under Vite's warning. Checked in a
browser: Settings and the player open.

---

# Features

Demand figures are vote counts read from Jellyfin's feature board and the
Plex forum on 3 October 2026 (approximate); "judgement" means no count.
Each feature names its Roku follow-up; those are collected in R18.

## F1 · Next Up, and remove from Continue Watching

**Done.** Server tested; the web is typechecked and built, not yet seen in a
browser. The Roku gets the row from the same `/api/home`; its side is in R18.

- Home's Continue Watching row now also holds, for each series whose last
  finished episode has an unwatched one after it, that next episode
  (`nextUp` on the entry, a "Next up" badge on the card). A series with an
  episode in progress shows that one only. Most recent activity first, up to
  20.
- `DELETE /api/home/continue/{itemId}` (the X on a card) takes a title off
  the row until the profile watches it again. The resume point is kept
  (table `home_hidden`, migration 0023).

## F2 · Watchlist

**Done.** Server tested; the web is typechecked and built, not yet seen in a
browser. The Roku still writes its `favoriteShows` pref until R18 moves it
to this.

- Table `profile_items` (migration 0024, which also copies each profile's
  `favoriteShows` in). `PUT` / `DELETE /api/items/{id}/watchlist`;
  `inWatchlist` on every title summary; `watchlist` in `/api/home`.
- Web: a "My list" button on the title page, a "My list" row on Home, and
  "My list" in the Movies and TV filters.

## F3 · Database backup and restore

**Done.** Server tested (backup, restore, pruning, the pre-upgrade copy, the
routes); the Settings card is typechecked and built, not yet seen in a
browser. The restore command hasn't been run against a real data folder.

- `internal/backup`: a zip of a `VACUUM INTO` copy of the database with
  `auth.key` and `server.id`, in `$DATA/backups`. Daily by default, the last
  7 kept (`backup.daily`, `backup.keep`); manual ones stay until deleted.
- `db.Open` copies an existing database to `backups/` before applying a new
  migration (the last 3 kept).
- Settings → Advanced → Backups: list, Back up now, download, delete, and the
  two settings. Admin only.
- `couchside restore <file>` refuses while the server answers on its port,
  checks the file is a database, and keeps what it replaces.

## F4 · Seek-bar preview thumbnails

**Done.** Tested with real ffmpeg, and seen working in a browser. The Roku's
use of the BIF file is in R18.

- A `trickplay` job in the encode pool, after everything else there: one
  frame every 10 seconds at 320 wide, decoded from keyframes only, tiled 10
  by 10 into JPEG sheets, with an `index.json` and an `index.bif`, in
  `$CACHE/files/<id>/trick/`. Thumbnails made from a file that has since
  changed aren't used.
- Off by default; a checkbox on a library (Libraries → Edit) turns it on,
  queues the files already there, and the scan queues new ones.
- `GET /api/files/{id}/trickplay` (the index) and `/trickplay/{n}.jpg` or
  `/trickplay/index.bif`.
- The web player shows the frame above the seek bar's time label.

## F5 · Skip intro and credits

**Done**, both steps. Step 1 was seen working in a browser (a file with
"Intro" and "End Credits" chapters). Step 2 is tested on synthetic episodes
through real ffmpeg; it hasn't met a real TV season yet, so expect to tune
its thresholds (`audiofp`'s `maxDiff`, `minAgree`; `introMinSec` /
`introMaxSec`).

- **Marks** (`file_segments`): an intro and end credits per file, from the
  file's chapter names (read the first time it's played), from detection, or
  from an admin. A manual mark is never replaced; chapters beat detection.
- **Player:** "Skip intro" while in the intro (or S), and during the credits
  "Next episode" or "Skip credits". Profile setting Intros: skip button
  (default), auto-skip, or off. Admins mark the intro and credits at the
  playhead from Settings → Intro and credits.
- **Detection** (`internal/audiofp`, `worker/intros.go`): a job per series
  fingerprints the first 10 minutes of each episode's sound and finds the
  stretch neighbouring episodes share (15 to 180 seconds). On for a TV
  library with "Find intros" ticked, or for one show from its "⋯" menu.
- `GET /api/files/{id}/segments`; admins: `PUT` / `DELETE
  /api/files/{id}/segments/{intro|credits}`, `POST /api/items/{id}/intros`.

**Not done:** detecting credits (only chapters and admins mark them), and
starting the next-episode countdown at the credits.

## F6 · Per-profile libraries and rating limit

**Done.** Server tested on every kind of route; the account form is
typechecked and built, not yet seen in a browser.

- `profile_libraries` (none = all) and `profiles.max_rating` (G, PG, PG-13 or
  R, with TV ratings mapped on; with a limit, unrated and unmatched titles
  are hidden). Admins are never limited.
- Applied in the queries themselves (`db/access.go`: `visible`,
  `visibleFileJoin`, from `db.WithAccess` set by `authenticate`), so lists,
  Home, My list, search, people, a title's page, a file's info, streams,
  tracks, subtitles and new HLS sessions all answer 404 or leave it out.
- Settings → Accounts: tick libraries and pick "Up to…" per account.

**Decided / known gaps**
- Artwork routes stay open without a token (TV image nodes need that), so a
  poster can still be fetched by someone who guesses a title's id.
- Live TV isn't limited: a virtual channel can play a title the profile
  couldn't open, and the sidebar's counts are for the whole server.

## F7 · Versions and editions

**Done.** Tested, and seen in a browser with two cuts of one film.

- `parse.Edition`: Plex's `{edition-…}` tag (on the file or its folder), or
  a cut named after the year (Extended, Director's Cut, Final Cut, Unrated,
  Uncut, Theatrical, Remastered, Special Edition, IMAX). Stored in
  `files.edition`, and kept up to date for unchanged files on each scan.
- A film with more than one copy gets a version chooser beside Play
  ("4K · HEVC · Extended · 54 GB"). The choice is remembered per profile and
  title (`profile_versions`, `PUT /api/items/{id}/version`).
- Copies that are different cuts aren't duplicates in the Manage view.

**Not done:** HDR isn't in the chooser's label (it isn't stored for files),
and episodes with two copies still play the larger.

## F8 · Surround sound passthrough

**Done** on the server, and run for real: a 5.1 FLAC film asked for with
`audioCodecs: ["ac3","eac3"]` came out as 6-channel E-AC-3 in the HLS
segments. The Roku sending the list is R18; browsers don't send one and keep
stereo AAC.

- `POST /api/files/{id}/hls` takes `audioCodecs` (`ac3`, `eac3`, in any
  spelling). An AC-3 or E-AC-3 track the client lists is copied; any other
  track with more than two channels is converted to 5.1 E-AC-3 (or AC-3 when
  that's all the client plays) at 640 kbit/s. The answer's `audioOut` says
  which: `copy`, `aac`, `eac3` or `ac3`.

## F9 · Sign in with OIDC

**Done.** The whole flow is tested against a fake provider (sign-in,
unknown people, auto-created profiles, the admin group, a replayed token, a
made-up callback), and the TV code flow was run for real through the `/link`
page. It hasn't met a real provider (Authelia, Authentik, Keycloak) yet.

- **OIDC for the web** (`api/oidc.go`): authorization-code flow with PKCE,
  state and nonce. Settings → Accounts → Single sign-on holds the provider's
  address, client id and secret, the claim that names the profile, whether
  to make profiles for new people, an admin group, and the button's text.
  The sign-in page gets the button; a failure comes back there as a message.
  Local passwords and `reset-password` keep working.
- **TVs** (`api/device.go`): `POST /api/auth/device` gives a TV a code to
  show; a signed-in person enters it at `/link` (also under Your account →
  Sign in a TV); `POST /api/auth/device/token` then gives the TV its tokens.
  That signs a TV in to any account without typing a password, including
  ones that sign in through the provider. The Roku side is R18.

**Know before turning it on**
- A profile made by single sign-on has no password and is locked from
  setting one. With passwordless sign-in on, anyone could pick it without
  going through the provider, so use single sign-on with passwordless off.
- The ID token's signature isn't checked: it's read from the token endpoint
  over TLS with the client secret, which the spec allows. Issuer, audience,
  expiry and nonce are checked.

## F10 · Two-factor sign-in

**Done.** Tested (the RFC's code vectors, enrolment, sign-in, replay,
recovery codes, admin reset), and enrolment was run for real in a browser
with a computed code.

- TOTP (`auth/totp.go`: SHA-1, six digits, 30 seconds, the step either side
  allowed, each code usable once). Your account → Two-step sign-in shows a
  QR code and the key, checks a code, then shows eight recovery codes once.
- Signing in with the password then answers 401 `totp_required` until a
  `code` is sent with it: the app's code, or a recovery code (spent on use).
- Turning it off needs a code. An admin can turn it off for someone from
  Settings → Accounts, and `couchside reset-password` clears it.
- New web dependency: `qrcode-generator` (MIT), for the QR code; the
  third-party notices are regenerated.

**By design:** it applies to signing in with a password. A profile picked
without one isn't asked, sign-in through a provider relies on the
provider's own second step, and a TV is best signed in with a code from
`/link` (the Roku's own code prompt is R18).

## Not planned

- A second guide source (XMLTV, M3U): what's there covers it.
- Music, audiobooks and photos; offline downloads; apps for other TV
  platforms; watch together; DLNA; plugins. Each is a separate product or
  works against the one-process, one-SQLite-file design.
- Asked for, not yet planned: subtitle download and styling, watch history
  and stats, collections, trailers, API keys and webhooks, Trakt, NFO files,
  multi-episode files, metadata language, an NVENC/QSV image, missing
  episodes, recording priorities, SAP audio, AirPlay and Chromecast.

---

# Carried over

| ID  | Story                                 | Needed by | State                    |
| --- | ------------------------------------- | --------- | ------------------------ |
| S5  | Demo server for Roku reviewers        | R7        | DNS record left; see B23 |
| S7  | Subtitle and audio track info for TVs | R4        | Done                     |
| S8  | Server discovery on the LAN           | R10       | Device test left         |

## S5 · Demo server for Roku reviewers

Built and tested locally. Left: the `demo.couchside.app` DNS record, the
cross-network check from a Roku, and B23 (what the demo library holds).

## S7 · Subtitle and audio track info for TVs

**Done** on the server. The Roku asking this way is R18.

`GET /api/files/{id}/subtitles/s<N>.vtt?async=1` converts the whole embedded
track in the background (up to 30 minutes, for a large remux on a share)
and answers 202 with `Retry-After` until it's cached, then the track. A
conversion that fails answers 422 for the next ten minutes instead of
starting again on every poll. Without `async` the URL behaves as before, so
a player can be given it once the poll says it's ready.

## S8 · Server discovery on the LAN

Done (PR #28) for the same subnet. Left: try it on a real Roku with
`discovery.hostNetwork`.
