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
| B1  | Turning passwords on ends passwordless sessions        | Security  | Confirmed | PR #45|
| B2  | Sign-in hardening, second pass                         | Security  | Reported  | PR #46|
| B3  | Remote image cache can't be filled between prunes      | Security  | Reported  | PR #47|
| B4  | Internal error text stays on the server                | Security  | Reported  | PR #48|
| B5  | A provider outage doesn't wipe cast and backdrops      | Data loss | Confirmed |      |
| B6  | Virtual channels stay on their schedule                | Bug       | Confirmed |      |
| B7  | Virtual and tuner streams use the right picture path   | Bug       | Reported  |      |
| B8  | A recording watched from the start outlives recording  | Bug       | Confirmed |      |
| B9  | Commercial detection: custom ini and the job lookup    | Bug       | Confirmed |      |
| B10 | DVR: stale series matches and failed joins             | Bug       | Reported  |      |
| B11 | Transcode: false GPU fallback, copied HEVC             | Bug       | Reported  |      |
| B12 | Server odds and ends                                   | Bug       | Mixed     |      |
| B13 | Player: subtitles, audio track, retry                  | Bug       | Confirmed |      |
| B14 | Live player: failed Record, phone controls             | Bug       | Confirmed |      |
| B15 | `useApi`: late answers, stale errors, cache size       | Bug       | Reported  |      |
| B16 | Console polling and token renewal                      | Bug       | Confirmed |      |
| B17 | Load failures and failed actions are shown             | Bug       | Reported  |      |
| B18 | Keyboard and screen-reader gaps                        | A11y      | Reported  |      |
| B19 | Docker and compose                                     | Packaging | Reported  |      |
| B20 | Helm chart hardening                                   | Packaging | Reported  |      |
| B21 | CI and release                                         | Packaging | Reported  |      |
| B22 | Docs and site catch up                                 | Docs      | Reported  |      |
| B23 | Demo media script                                      | Hygiene   | Reported  |      |
| C1  | Server dead code and stale comments                    | Cleanup   |           |      |
| C2  | One builder for ffmpeg's HLS arguments                 | Cleanup   |           |      |
| C3  | Split `api/auth.go` and `api/handlers.go`              | Cleanup   |           |      |
| C4  | One HLS hook for the three players                     | Cleanup   |           |      |
| C5  | Web duplication, dead CSS, player colour tokens        | Cleanup   |           |      |
| C6  | Load pages on demand                                   | Cleanup   |           |      |
| F1  | Next Up, and remove from Continue Watching             | Feature   | S         |      |
| F2  | Watchlist                                              | Feature   | S         |      |
| F3  | Database backup and restore                            | Feature   | S         |      |
| F4  | Seek-bar preview thumbnails                            | Feature   | M         |      |
| F5  | Skip intro and credits                                 | Feature   | M to L    |      |
| F6  | Per-profile libraries and rating limit                 | Feature   | M         |      |
| F7  | Versions and editions                                  | Feature   | S to M    |      |
| F8  | Surround sound passthrough                             | Feature   | S to M    |      |
| F9  | Sign in with OIDC                                      | Feature   | M         |      |
| F10 | Two-factor sign-in                                     | Feature   | S to M    |      |

**How to read these.** "Confirmed" means the defect was read in the code and
checked by a second pass. "Reported" means one reviewer traced it by reading
and nobody re-checked: **confirm it in the code before fixing, and drop the
item (noting why in the PR) if it doesn't hold.** Nothing was reproduced at
runtime. Line numbers are from commit `1acefb8` and will drift; the function
names are the stable reference.

**Rules for every story**
- One story per branch and PR. Don't fold neighbouring cleanups in.
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

**Done** (PR #45).

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

**Done** (PR #46).

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

**Done** (PR #47).

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

**Done** (PR #48).

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

**Why.** `virtualArgs` (`livetv/virtual_stream.go:158`) passes no `HDR`,
`SrcHeight` or `HWDecode` in `VideoOpts`, and `PlayoutPiece` doesn't carry
them. HDR films go out without tone mapping, SD files are upscaled to the
session height, and 4K HEVC is decoded on the CPU when the GPU could. Tuner
streams pass no `SrcHeight` either, so 480i channels are upscaled.

**Scope**
- Carry height and HDR on the piece (from `files`, when the playout is built)
  and pass them, with `Encoder.HWDecode`, to `VideoOpts`.
- Pass the lineup's height for tuner streams where it's known.

**Done when** an HDR film on a virtual channel is tone-mapped and an SD file
isn't upscaled (argument tests, as `session_test.go` does).

## B8 · A recording watched from the start outlives recording

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

**Scope**
- **Stale waiter.** In `Segment` (`transcode/session.go:338`), a waiter's tick
  can see "exited with error" for a run another request's seek just killed and
  call `gpuFallback` (`:424`), which turns GPU decoding off for good and
  restarts at the stale segment. Check `ctx.Err()` first and only fall back
  when the dead run is the one that covered `n`. Reported, not reproduced.
- Any failure (an unreadable source) is logged as "GPU decoding failed". Say
  that only when the retry on the CPU works.
- **Copied video** (the uncommitted `VideoCodecs` change): normalise codec
  names as `livetv.NormalizeCodec` does rather than matching ffprobe names by
  exact case; don't copy Dolby Vision profile 5 (it probes as `hevc`); check
  on a Roku that HEVC with 10s GOPs plays against a playlist that lists 4s
  segments, and list real keyframe-based durations if it doesn't. Pairs
  with R15.
- `transcode/proc_windows.go`: releases ship a Windows zip, so the "dev builds
  only" comment is wrong and ffmpeg is never paused there. Fix the comment
  and note it in `docs/install.md`.

**Done when** a seek during a GPU session can't flip it to CPU decoding
(test), and copied HEVC is confirmed on a device.

## B12 · Server odds and ends

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

**Scope** (`web/src/pages/PlayerPage.tsx` unless named)
- **Subtitle chunks are retried four times a second.** `load` (`:285`)
  removes a failed chunk from `loaded` and `around` asks again on the next
  `timeupdate`. In the last 90s of any file the next chunk is a 404 by
  design; a track ffmpeg can't convert (422) re-runs ffmpeg for the whole
  film. Never retry a 404, back off other failures, and don't ask for chunks
  at or past `duration`.
- **Wrong audio track.** `defaultAudio` is 0 until `/streams` loads and isn't
  in `key`, so an HLS session created first plays track 0 while the menu
  marks the default. Wait for `streams` before creating an HLS source.
- **A failed hls.js import is cached for good** (`hlsModule ??= import(...)`
  here, in `LivePlayerPage.tsx:16` and `RecordingPlayerPage.tsx:12`) and
  shown as "this browser can't play streaming video". One shared `loadHls()`
  in `lib/` that forgets a rejection and says "couldn't load the player".
- **"Try again" does nothing when the file info failed** (`:540`): it bumps
  `nonce` but `info` is still missing. Call `useApi`'s `reload`.
- `RecordingPlayerPage.tsx:80`: `recoverMediaError()` has no limit and the
  error card has no retry. Reuse the live player's bounded recovery.
- `PlayerFrame.tsx:197` returns on any `ctrlKey` before `p.onKey`, so the live
  player's Ctrl+Up/Down zapping never runs. Call `onKey` first.
- After `webkitEnterFullscreen` on an iPhone, `full` never updates. Listen for
  `webkitbeginfullscreen`/`webkitendfullscreen`.

**Done when** the last 90s of a file with subtitles makes no repeated
requests, and a file whose default audio isn't track 0 starts on it.

## B14 · Live player: failed Record, phone controls

**Scope**
- `toggleRecord` (`web/src/pages/LivePlayerPage.tsx:179`) puts a failed Record
  or Stop into the player's fatal `error`, so a tuner conflict replaces the
  stream with "Can't play". Use `attempt("Couldn't record", …)`.
- `TopButton` (`components/player/PlayerFrame.tsx:521`) keeps its text labels
  on phones; Start over, Record, channel up/down and Guide leave no room for
  the title at 360px, and a recording's bottom row overflows. Hide the labels
  below `md` (with `aria-label`) and move "Start over" into the menu there.
  Check on a phone; this one is reasoned from class widths.

**Done when** a failed Record shows a notice over the playing stream, and the
controls fit at 360px.

## B15 · `useApi`: late answers, stale errors, cache size

**Why.** `web/src/lib/api.ts:66`: no request sequencing, so a poll sent
before a mutation can land after the reload and repaint old data (Activity,
Libraries, Manage). `error` isn't reset when the URL changes. The module
`cache` never evicts, so every search keystroke stays for the tab's life.

**Scope**
- A per-hook request counter that drops out-of-date answers.
- Reset `error` (and `data` when the URL becomes null) on a URL change.
- Cap the cache (LRU, a few hundred entries).

**Done when** a slow poll can't overwrite a newer reload (unit test of the
hook's fetch logic, pulled out so it can be tested without a DOM).

## B16 · Console polling and token renewal

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

**Scope**
- These take only `data` from `useApi`, so a failed request is a blank card or
  an endless spinner: `ServerNow.tsx:13`, `SystemHistory.tsx:35`,
  `VirtualChannels.tsx:16`, `manage/ItemPanel.tsx:20` (feeding `FileList`),
  `ChannelEditor.tsx:51`, `ActivityPage.tsx:149`. `PersonPage.tsx:20` and
  `ItemPage.tsx:28` say "not found" for any error. Render `ErrorNote`.
- `void logout()` in `MobileNav.tsx:135`, `AccountSettings.tsx:60` and
  `ChangePasswordPage.tsx:47`: wrap in `attempt("Couldn't sign out", …)`.
- `setPrefs` (`stores/profile.ts:35`) swallows failures; roll back and notify.
- `ArtworkEditor.tsx:88` (`Slot`): `broken` is only cleared by `onLoad`, but
  the `<img>` isn't rendered while broken. Key the image on `preview`.
- `ChannelEditor.tsx:61`: the number field refills itself when emptied, and
  prefills a stale, taken number after creating a channel. Seed it once from a
  fresh request. The "Classic TV" preset ticks "Add commercials" with no
  folder, which saves as none; require the folder.

**Done when** each page shows an error state with the server stopped.

## B18 · Keyboard and screen-reader gaps

**Scope**
- `LibraryManagePage.tsx:173`: rows are `<tr onClick>` with no way in by
  keyboard. Make the title cell a button; add `aria-sort` to the headers.
- The same table renders the whole library and refetches every 3s while
  anything is `pending`. Virtualise it as `PosterGrid` does.
- `FilterMenu.tsx:43` and the More sheet (`MobileNav.tsx:109`, left open by
  A15) are modal sheets without `useDialog`.
- `MenuButton` (`components/ui.tsx:188`) and the player's `SettingsMenu` use
  `role="menu"` without moving focus in or arrow keys; `MenuButton`'s Escape
  also closes the `ItemPanel` behind it. Stop propagation; add arrow keys.

**Done when** the Manage view, both sheets and both menus work with the
keyboard alone.

## B19 · Docker and compose

**Scope**
- `docker-compose.yml` has `build: .` and `image: ghcr.io/…:latest`. That
  build has no TMDB key and tags a dev build as the release image, and the
  copy in release zips has no Dockerfile. Drop `build:` (keep it in a
  `docker-compose.dev.yml`) and make the header `docker compose up -d`.
- No `HEALTHCHECK`; add one on `/healthz` (busybox `wget` is in the image).
- `.dockerignore`: add `.env*`, `site/`, `branding/`, `docs/`, `scripts/`,
  `.github/`.
- CI uses Node 22 and Go 1.26; the Dockerfile uses `node:26` and
  `golang:1.27`. Align them. Add `web/package-lock.json` and `server/go.sum`
  to `docker.yml`'s paths.
- `.env.example` lacks `COUCHSIDE_HWACCEL`, which compose reads.

**Done when** `docker compose up -d` from a release zip starts the published
image, and the image reports healthy.

## B20 · Helm chart hardening

**Scope** (`deploy/helm/couchside`)
- `fsGroup: 1000` with no `fsGroupChangePolicy: OnRootMismatch`: a recursive
  chown of the cache and recordings volumes on every start.
- The "dri enabled, not privileged" branch keeps only
  `allowPrivilegeEscalation: false`; keep `runAsNonRoot` and
  `capabilities.drop: [ALL]` there too.
- Add `seccompProfile: RuntimeDefault` and
  `automountServiceAccountToken: false`.
- Add a `startupProbe` on `/healthz`: hardware detection and migrations run
  before the server listens, and liveness allows about 100s.
- Offer `readOnlyRootFilesystem` with an emptyDir at `/tmp`.
- `extraVolumes`/`extraVolumeMounts`, so `COUCHSIDE_COMSKIP_INI` can point at
  a mounted file; a `hwaccel.device` value for `COUCHSIDE_VAAPI_DEVICE`.
- The `extraEnv` comment shows `COUCHSIDE_TRUSTED_PROXIES`, which
  `auth.trustedProxies` already sets.

**Done when** `helm lint` and `helm template` pass (run on Windows) and the
pod starts with the new security context.

## B21 · CI and release

**Scope**
- `go test -race ./...` in `ci.yml` and the release `verify` job.
- Run `scripts/third-party-notices.py --check` in `verify`.
- `release.yml` checks `appVersion` but not the chart `version`, nor that the
  tag is on `main`.
- Pin `actions/*` by SHA in the workflows that can write (`notices.yml`,
  `pages.yml`, `release.yml`), as third-party actions already are.
- Add a web lint step (ESLint with the React hooks rules: B16's missing
  dependency arrays are what it would have caught).

**Done when** CI runs the race detector and the lint, and a tag whose chart
version is behind fails `verify`.

## B22 · Docs and site catch up

**Scope**
- `docs/configuration.md:45` describes the old single Settings page. Rewrite
  per section (Your settings, System, Console, Accounts, Metadata, Live TV,
  Advanced); fix "Settings → Live TV & DVR", "→ Your channels" and
  "→ Transcoding" in `docs/live-tv.md` and `docs/playback.md`. Cover the
  Console, the history charts, search, "Not a commercial" and the merge gap.
- `SECURITY.md:31` and `docs/accounts.md:63` list posters, backdrops and
  stills as open. Add `/api/artwork/people/{id}`, `/api/artwork/remote` and
  `/api/discovery`.
- `docs/configuration.md:13`: the data folder also holds `auth.key` and
  `server.id` (matters for backups, F3).
- `site/privacy.html`: the last 2000 log lines, with sign-in names and
  addresses, are held in memory and shown in Settings → Console.
- "One 7 MB binary" (`README.md:8`, `site/index.html:79`) is 13.7 MB before
  the UI. State the real size or drop the number.
- The site, `web/index.html` `theme-color` and `site.webmanifest` still use
  the previous palette (`#0D1418`, `#63D6BE`); `docs/style.md` has `#111315`,
  `#5AF6B9`, `#04261A`. `index.html` also declares `color-scheme: dark
  light`.
- Site meta: canonical, `og:url`, `og:type`, `twitter:card`; the privacy page
  gets its own og text and loses the landing page's CSS; add `404.html`.
- `CLAUDE.md`: Alpine 3.24, `SHA256SUMS.txt`, and a current "Next milestones".
  `docs/development.md:58`: the `:v<version>` tag. `docs/live-tv.md:21`: 360p.

**Done when** each doc matches the code it describes.

## B23 · Demo media script

**Scope** (`scripts/fetch-demo-media.py`, uncommitted changes)
- "Any Bonds Today?" downloads as `.ogv`, which `parse.videoExts` doesn't
  scan. Pick another copy or drop it.
- `--only` defaults to every section, so the documented run now pulls the
  cartoons too (about 38 GB). Leave cartoons out of the default.
- The closing message prints an empty list when only `commercials` was
  fetched, and mentions Cartoons when they weren't.
- Remove the Censored Eleven and wartime-caricature shorts ("Hittin' the
  Trail for Hallelujah Land", "Jungle Jitters", "All This and Rabbit Stew",
  "Tokio Jokio", "The Ducktators", "Inki and the Minah Bird") and the
  broadcast-capture and rip sources: this feeds the reviewers' demo (S5).
- Add `__pycache__/` and `*.pyc` to `.gitignore`.

**Done when** a default run fetches only what the demo needs and every file
it fetches is scanned.

---

# Cleanup

## C1 · Server dead code and stale comments

- `s.tv` is never nil now: remove the 19 `s.tv == nil` checks in
  `api/livetv.go`, `handlers.go`, `system.go`, `search.go`, and the constant
  `searchIndex.tv` / `withTV`. Use `HasTuner()` where a tuner is meant.
- Unused: `db.SetMatchStatus` and `worker.abs` (`worker/match.go:325`).
  `db.CreateProfile` is only called from tests: move it to a test helper.
  `webIdle == tvIdle` makes `idleFor` a constant.
- `truncate` (`api/handlers.go:457`) and `clip` (`api/auth.go:503`) are the
  same and both cut mid-rune; `tail` exists in `api` and `transcode`.
- `livetv/virtual.go:300` `videoExts` duplicates `parse.IsVideo` and lacks
  `.m2ts`.
- Stale comments: `api/api.go:43`; `api/auth.go:21,52,126,133`
  ("COUCHSIDE_AUTH", "accounts off"); `db/profiles.go` `Profile`;
  `db/watch.go:23`; `config/config.go:15`; the doubled doc comments on
  `Record` (`livetv/recorder.go:117`) and at `livetv/rules.go:389`.
- `livetv/service.go:4`: gofmt the imports.

## C2 · One builder for ffmpeg's HLS arguments

`session.start`, `liveArgs` and `virtualArgs` each build the HLS output
arguments, `-force_key_frames` and `temp_file` themselves. Move the shared
part into `transcode`, so B7-style gaps (an option one path forgot) can't
recur. Do this after B7 and B11.

## C3 · Split `api/auth.go` and `api/handlers.go`

`auth.go` (821 lines) mixes middleware, sign-in, sessions and password
change: move the session handlers and the password handlers out.
`handlers.go` (635) holds libraries, items, playback, artwork and jobs: one
file each. No behaviour change.

## C4 · One HLS hook for the three players

The session POST, `pagehide` DELETE, native fallback, attach and destroy are
copied across `PlayerPage`, `LivePlayerPage` and `RecordingPlayerPage`. Write
`useHlsSession` (with B13's `loadHls`), then split `PlayerPage.tsx` (674
lines) into source, subtitles, progress and settings parts. Do this after B13
and B14.

## C5 · Web duplication, dead CSS, player colour tokens

- `bg-critical text-white` (`PlayerFrame.tsx:453,528`,
  `RecordingPlayerPage.tsx:132`) breaks `docs/style.md`: red fills take
  `text-on-accent`.
- Hard-coded colours: `.scrim`, `.channel-logo`, `video::cue` in `index.css`,
  the hatch gradients in `SeekBar.tsx:114,121`, and `text-white` / `bg-black/x`
  across the player chrome. Give the player its own tokens beside
  `--player-bg`.
- Dead: `.toolbar-field`, the `streamUrl` export (`lib/api.ts:120`), the
  doubled comment at `components/ui.tsx:182`.
- Shared helpers for: `e instanceof Error ? e.message : String(e)` (24
  places, with a hand-rolled busy/error block in about ten components); the
  version label (three places); the 1080/720/480 options (three places); the
  buffered-range loop (three places).
- Other large files to split when next touched: `PlayerFrame.tsx` (560),
  `ChannelEditor.tsx` (449), `ItemPage.tsx` (448), `lib/types.ts` (681).

## C6 · Load pages on demand

`App.tsx` imports every page, so the 525 kB entry chunk carries the admin
sections, Manage and all three players for every profile. `React.lazy` the
players and the admin routes. Done when the entry chunk is under Vite's
500 kB warning.

---

# Features

Demand figures are vote counts read from Jellyfin's feature board and the
Plex forum on 3 October 2026 (approximate); "judgement" means no count.
Each feature names its Roku follow-up; those are collected in R18.

## F1 · Next Up, and remove from Continue Watching

**Why.** "Remove from Continue Watching" is the second most wanted item on
Jellyfin's board (about 1,700 votes). Home has Continue Watching and Recently
Added but no Next Up, so finishing an episode drops the series off Home.

**Scope**
- `db.ContinueWatching` (`db/watch.go`) is `watched=0 AND position>30`. Add
  Next Up: per series with a watched episode, the next unwatched episode
  that has a file (`nextFileId` already works this out for the player).
  Show the two as one row, most recent activity first.
- A hide action: `DELETE /api/home/continue/{fileId}` zeroes the position
  (or sets a hidden flag for Next Up, cleared when the series is watched
  again). A button on the tile, and in its context menu.
- `/api/home` carries it, so the Roku gets it from the same call.

**Done when** finishing an episode puts the next one on Home, and a tile can
be removed.

## F2 · Watchlist

**Why.** Jellyfin about 1,300 votes. The Roku already keeps `favoriteShows`
in prefs; the web has nothing.

**Scope**
- Table `profile_items` (`profile_id`, `item_id`, `added_at`), like
  `profile_channels`. `PUT`/`DELETE /api/items/{id}/watchlist`; `inWatchlist`
  on summaries through `summaryCols(ctx)`.
- A button on the title page, a Home row, and a filter in the grids.
- Migrate the Roku's `favoriteShows` pref into it once per profile.

**Done when** a title added on the web shows in "My list" on both clients.

## F3 · Database backup and restore

**Why.** One SQLite file holds everything; Jellyfin shipped this as a
headline feature of 10.11.

**Scope**
- `VACUUM INTO $DATA/backups/couchside-<date>.db` on a timer (daily, keep 7;
  both settable), and before each migration run.
- Settings → Advanced: list, "Back up now", download. The download includes
  `auth.key` and `server.id` (a zip), or the docs say to copy them.
- `couchside restore <file>` beside `reset-password`, and a doc section.

**Done when** a backup taken from the UI restores to a working server.

## F4 · Seek-bar preview thumbnails

**Why.** Standard in Plex, Emby and Jellyfin (judgement).

**Scope**
- A `trickplay` job in the encode pool, lowest priority: one frame every 10s
  at 320 wide, tiled into sprite sheets, with an index, in
  `$CACHE/files/<id>/trick/`. Off by default; on per library.
- It reads the whole file, which is slow over SMB (the subtitle work measured
  7 minutes for 30 GB): run one at a time, use GPU decode where there is one,
  and drop the result when the file changes, as optimized copies do.
- `GET /api/files/{id}/trickplay` (index) and the sheets. `SeekBar` already
  tracks hover time; draw the frame above it, and while scrubbing on touch.
- Roku: write a BIF file as well, which its Video node shows natively.

**Done when** hovering the seek bar shows the frame at that time.

## F5 · Skip intro and credits

**Why.** Expected by anyone coming from Plex (judgement). `useBreakSkip`, the
seek-bar markers and the `commercials` table already do this for adverts.

**Scope**
- **Step 1, chapters.** Stop discarding chapter info at probe time; read
  chapter names ("Intro", "Opening", "Credits", "OP", "ED") into a `segments`
  table with a `type`. Show "Skip intro" / "Skip credits" buttons (not an
  automatic skip by default; a profile pref chooses), and start the
  next-episode countdown at the credits.
- **Step 2, detection.** A job that fingerprints the audio of a season's
  episodes (chromaprint) and finds the shared opening. Heavy on CPU and I/O:
  same scheduling as F4.
- Admins can correct a segment from the seek bar, as with "Not a commercial".

**Done when** step 1 works on a file with named chapters; step 2 is its own
PR.

## F6 · Per-profile libraries and rating limit

**Why.** Every profile sees every library. Households want a child's profile
limited (judgement; Jellyfin "user groups" about 200).

**Scope**
- Table `profile_libraries` (none = all) and `profiles.max_rating`.
  `media_items.rated` is stored but unused.
- The profile is already in every query (`watchJoin(ctx)`,
  `summaryCols(ctx)`). Filter in items, home, people, streams, play info and
  search (the index is shared, so filter at hydration).
- Artwork routes are open without a token, so posters of hidden titles can
  still be fetched by id. Decide: accept it, or sign artwork URLs.
- Admin UI in Settings → Accounts.

**Done when** a limited profile can't list, search, open or play a title
outside its libraries or above its rating (tests on each route).

## F7 · Versions and editions

**Why.** Plex "multiple cuts" about 1,450 votes; Jellyfin about 265. Extra
`copy` files exist and `PlayInfo` picks the best, but the viewer can't choose
4K or 1080p, and "extended", "unrated" and "remastered" are stripped as junk
(`parse/parse.go:32`).

**Scope**
- Parse an `edition` from `{edition-…}` and those words into `files.edition`
  (re-parsed on scan like roles). Copies with different editions aren't
  duplicates in the Manage view.
- A version chooser on the title page (resolution, HDR, edition, size);
  the choice is remembered per item and profile.

**Done when** a film with a 4K and a 1080p copy, or two cuts, lets the viewer
pick.

## F8 · Surround sound passthrough

**Why.** Audio on the HLS path is copied only for AAC/MP3
(`transcode/session.go:168`); everything else becomes AAC, so a TV with a
receiver loses 5.1 (judgement; Plex Atmos thread about 700).

**Scope**
- An `audioCodecs` list on `POST /api/files/{id}/hls`, as live TV has. Copy
  AC3 and E-AC3 when the client lists them; convert DTS and TrueHD to E-AC3
  5.1 rather than stereo AAC. Browsers keep AAC.
- The Roku sends what `CanDecodeAudio` allows (R18).

**Done when** an AC3 5.1 file plays as 5.1 on a Roku through HLS (argument
test, then on a device).

## F9 · Sign in with OIDC

**Why.** Jellyfin about 1,200 votes.

**Scope**
- Settings: issuer, client id and secret, the claim that maps to a profile
  name, whether to create profiles on first sign-in, and an admin group.
- The callback creates the same session `login` does, so everything after
  sign-in is unchanged. Local passwords keep working; `reset-password` stays
  the recovery path.
- TV clients: a device-code screen ("go to /link and enter ABCD"), which
  also gives the Roku a way to sign in to password accounts without typing.

**Done when** a profile can sign in on the web through an OIDC provider, and
on a Roku with a code.

## F10 · Two-factor sign-in

**Why.** Jellyfin about 1,100 votes; Plex has it.

**Scope**
- TOTP per profile: enrol with a QR code in Your settings, recovery codes,
  and a second step in `login`. Admins can reset it; `reset-password -admin`
  clears it.
- The Roku asks for the code after the password, or uses F9's device code.
- Not applied to passwordless profiles.

**Done when** a profile with TOTP can't sign in without the code.

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
| S7  | Subtitle and audio track info for TVs | R4        | Open                     |
| S8  | Server discovery on the LAN           | R10       | Device test left         |

## S5 · Demo server for Roku reviewers

Built and tested locally. Left: the `demo.couchside.app` DNS record, the
cross-network check from a Roku, and B23 (what the demo library holds).

## S7 · Subtitle and audio track info for TVs

**Why.** Roku review expects captions, and the Roku app (R4) offers subtitle
and audio tracks.

**Already there.** `GET /api/files/{id}/streams` lists the tracks.
`s<N>.vtt` returns a whole embedded text track, and the web fetches 90s
chunks (`s<N>.c<K>.vtt`).

**Left to do**
- Whole-track extraction reads the entire file (about 7 minutes for a 30 GB
  remux over SMB) and is cut off at 5 minutes. Either extract in the
  background and let the client poll (202 until ready), or offer a subtitle
  rendition inside the HLS session that the Roku can select.
- Confirm with R4 which of the two the Roku's Video node can use.

**Done when** the Roku can show a chosen text track for the whole film,
including on a large remux.

## S8 · Server discovery on the LAN

Done (PR #28) for the same subnet. Left: try it on a real Roku with
`discovery.hostNetwork`.
