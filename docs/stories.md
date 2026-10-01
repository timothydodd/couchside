# Server stories

The server's share of the work behind search, password protection and getting
the Roku app ([couchside-roku](https://github.com/timothydodd/couchside-roku),
its stories in `docs/stories.md` there) into a Roku beta channel and then the
Channel Store. Each story says what and why; the details get worked out when
it's picked up. `R-n` refers to a Roku story.

Suggested order: **search → password → store readiness → extras.**

| ID  | Story                                   | Milestone       | Needed by        |
| --- | --------------------------------------- | --------------- | ---------------- |
| S1  | Search API                              | Search          | R1, web search   |
| S2  | Optional password, set at deploy time   | Password        | R2, S4           |
| S3  | Login protection and session handling   | Password        | S4               |
| S4  | Safe public exposure (HTTPS, docs)      | Store readiness | S5               |
| S5  | Demo server for Roku reviewers          | Store readiness | R7               |
| S6  | Privacy policy                          | Store readiness | R7               |
| S7  | Subtitle and audio track info for TVs   | Store readiness | R4               |
| S8  | Server discovery on the LAN             | Extras          | R10              |

---

## S1 · Search API

**Why.** Both apps need search, and there's no endpoint for it. The web filters
`/api/items` in the browser, which works for titles but can't reach episodes,
and a TV can't afford to download and filter the whole library every time.

**Scope**
- `GET /api/search?q=` across movie and show titles, episode titles, and (when
  live TV is on) channels and guide programmes airing in the next day or so.
- Results grouped by kind, ranked (exact and prefix matches first), capped per
  group, scoped to the profile like the rest (watched state).
- Forgiving matching: case, accents, punctuation ("spiderman" finds
  "Spider-Man"), reusing the title normalisation from matching where it fits.
- Web: a search box using it (the Movies/TV filters can stay client-side).

**Done when** the Roku (R1) and the web can find a movie, a show, an episode by
its title and a programme in the guide by typing a few letters.

## S2 · Optional password, set at deploy time

**Why.** Couchside has no login, so it can only live on a trusted LAN. Putting
it on the internet (for a Roku reviewer, or away from home) needs a password,
but home installs shouldn't be forced to have one.

**Scope**
- Off unless configured: a `COUCHSIDE_PASSWORD` env var (or a hash, e.g.
  `COUCHSIDE_PASSWORD_HASH`, so the plain text needn't sit in config). Docker
  compose: an env entry. Helm: a value backed by a Kubernetes Secret.
- When it's set, every `/api` route needs a session, including artwork,
  streams, HLS playlists/segments, subtitles and live TV. `/healthz` stays open
  for probes.
- `POST /api/login` → a session token. The web gets it as an HttpOnly cookie;
  TV apps send it as `Authorization: Bearer …` (the Roku Video node can send
  headers on stream requests). `POST /api/logout`.
- `/api/status` (or a small `/api/auth`) says whether a password is required,
  so apps know to show a login screen.
- Web: a login page; logout in Settings.
- Profiles stay as they are: the password guards the server, profiles still
  pick whose history it is.

**Done when** with the variable set, nothing under `/api` answers without
logging in (checked with curl, including a stream and an HLS segment), and
with it unset everything works exactly as today.

## S3 · Login protection and session handling

**Why.** A password on the internet gets guessed at.

**Scope**
- Rate-limit and slow down failed logins per IP; log them.
- Long-lived sessions for TVs (no re-entering a password on a remote every
  week), stored server-side so they can be revoked.
- Settings: list signed-in devices and sign one (or all) out; changing the
  password signs everyone out.
- Constant-time password comparison; tokens from a secure random source.

**Done when** repeated wrong passwords are throttled, and a device signed out
in Settings is refused on its next request.

## S4 · Safe public exposure

**Why.** Once there's a password, it must not travel in plain HTTP over the
internet.

**Scope**
- Docs for exposing Couchside: the Helm ingress with TLS (cert-manager or
  Traefik's ACME), or a reverse proxy for compose; which paths need long
  timeouts (HLS, live TV).
- Refuse, or warn loudly in the log and on the login page, when a password is
  set and requests arrive over plain HTTP from outside the LAN.
- Secure, SameSite cookies when served over HTTPS.

**Done when** README/Helm docs walk through an HTTPS setup, and the server
warns about password-over-HTTP from a public address.

## S5 · Demo server for Roku reviewers

**Why.** Roku certification needs a working server the reviewer can reach,
showing content we have the right to show. Our real libraries can't be that.

**Scope**
- A separate Couchside instance (compose profile or Helm values) with the
  password on, behind HTTPS (S4), seeded with openly licensed media: the
  Blender open movies (Big Buck Bunny, Sintel, Tears of Steel…) and a short
  public-domain series for the TV side.
- A script to download and lay out that media and its artwork.
- No live TV or DVR on the demo (no tuner, and no broadcast content).
- Reviewer notes: address, password, what to try.

**Done when** a Roku on another network can log in to the demo with the
reviewer notes alone and play a movie and an episode.

## S6 · Privacy policy

**Why.** The Channel Store listing needs a privacy policy URL.

**Scope**
- A short, honest policy: Couchside runs on your own server; the app talks
  only to the server you enter; no analytics or third-party tracking; what the
  server stores (watch history, profiles) and where.
- Published at a stable URL (e.g. GitHub Pages from `docs/`).

**Done when** the policy is live at a URL the store listing can use.

## S7 · Subtitle and audio track info for TVs

**Why.** Roku review expects captions, and the Roku app (R4) needs to offer
subtitle and audio tracks. `GET /api/files/{id}/streams` exists, but embedded
subtitles are cut into 90-second chunks for the web player, which a Roku
can't use.

**Scope**
- A way to get a whole embedded text subtitle track as one WebVTT file
  (cached after the first extraction, with a sensible wait for long files), or
  a subtitle rendition inside the HLS session the Roku can select.
- Language and "forced" flags on every track, so the app can honour the
  profile's subtitle language preference.
- Picture subtitles (PGS/DVD) stay burned in via the existing HLS option.

**Done when** the Roku can list a file's subtitle tracks and show a chosen
text track for the whole film.

## S8 · Server discovery on the LAN

**Why.** Typing `192.168.2.63:8095` with a remote is the worst part of setup.

**Scope**
- Advertise the server on the LAN (SSDP or mDNS/DNS-SD), with its name,
  version and URL, and whether a password is needed.
- Works across the usual home setups, including the Roku and server on
  different subnets (a manual address stays as the fallback).

**Done when** the Roku setup screen (R10) lists the server without typing,
on the same subnet at least.
