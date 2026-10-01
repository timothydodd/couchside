# Server stories

The server's share of the work behind search, accounts and getting
the Roku app ([couchside-roku](https://github.com/timothydodd/couchside-roku),
its stories in `docs/stories.md` there) into a Roku beta channel and then the
Channel Store. Each story says what and why; the details get worked out when
it's picked up. `R-n` refers to a Roku story.

Suggested order: **search → accounts → store readiness → extras.**

| ID  | Story                                   | Milestone       | Needed by        |
| --- | --------------------------------------- | --------------- | ---------------- |
| S1  | Search API                              | Search          | R1, web search   |
| S2  | Accounts (optional, set at deploy time) | Accounts        | R2, S4           |
| S3  | Sessions, refresh tokens, throttling    | Accounts        | R2, S4           |
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

## S2 · Accounts (optional, set at deploy time)

**Why.** Couchside has no login, so it can only live on a trusted LAN. Putting
it on the internet (for a Roku reviewer, or away from home) needs real
accounts, but home installs shouldn't be forced to have them. Profiles already
separate everyone's history, so a profile becomes the account.

**Scope**
- One toggle: `COUCHSIDE_AUTH=true` (Helm `auth.enabled`, compose an env
  entry). Off by default, and off means exactly today's behaviour: pick a
  profile, no passwords.
- On: every profile is a user with a password. Every `/api` route needs a
  signed-in user, including artwork, streams, HLS playlists/segments,
  subtitles and live TV; `/healthz` stays open for probes. The profile comes
  from the session, never from the `couchside_profile` cookie.
- **Passwords**: Argon2id (`golang.org/x/crypto/argon2`) with a random salt
  per user, stored as a self-describing hash string so the parameters can be
  raised later (rehash on next login). Minimum 8 characters, capped length so
  hashing can't be used to burn CPU, constant-time compare, and a dummy hash
  for unknown names so timing doesn't reveal which accounts exist.
- **Roles**: `admin` and `user`. Only admins reach settings (all of them,
  DVR settings included), libraries, the Manage view (file deletes), jobs and
  accounts. Users watch, and edit their own prefs and password.
- **Recording is a privilege**: a per-user `can_record` flag, off by default,
  that admins switch on in the account manager. With it, a user can schedule
  and cancel recordings and create series rules, and delete recordings they
  scheduled; without it the Record buttons are hidden and the DVR routes
  refuse. Admins can always record. Anyone can watch what's been recorded.
- **Account manager** (server API + Settings → Accounts, admins only): create
  an account (a profile with a password), rename, set role, allow recording,
  reset a password, disable, delete. A reset sets a temporary password the
  user must change at next sign-in. The last admin can't be demoted,
  disabled or deleted.
- **First run**: turning auth on with no admin yet prints a one-time setup
  code to the log; the web shows a setup page that takes the code and sets the
  first admin's name and password (existing profiles keep their history and
  get passwords from the account manager). Recovery for a lost admin password:
  `couchside reset-password <name>` run in the container.
- Login is name + password (no public list of accounts). A device can stay
  signed in to several profiles so switching on a shared TV doesn't need the
  password every time.
- `GET /api/auth` (open) says whether auth is on and whether setup is needed,
  so apps know to show sign-in.

**Done when** with the toggle on, nothing under `/api` answers without
signing in (checked with curl, including a stream and an HLS segment), users
can't reach admin routes or record unless allowed, and with it off
everything works exactly as today.

## S3 · Sessions, refresh tokens and login protection

**Why.** Accounts on the internet get guessed at, and TVs need to stay
signed in for months without keeping anything that's dangerous if stolen.

**Scope**
- **Access tokens**: short-lived (15 min), signed by a server key generated
  on first start and kept in the data dir; carry the user, role and session
  id. Checked against the in-memory list of live sessions, so a revoked
  session is refused on its next request, not when the token expires.
- **Refresh tokens**: 256-bit random, stored server-side only as a SHA-256
  hash in a `sessions` table (user, device name, user agent, IP, created,
  last used, expiry). Rotated on every refresh; reusing an old refresh token
  revokes the whole session (theft detection). Sliding expiry: a session
  ends after 30 days unused on the web, 90 on TV apps.
- **Web**: both tokens in HttpOnly cookies (`<video>`, `<img>` and hls.js
  can't send headers): access cookie `SameSite=Lax`, refresh cookie
  `SameSite=Strict` with `Path=/api/auth`; `Secure` over HTTPS. State-changing
  requests also need a matching `Origin` (or a custom header) as CSRF
  defence. The frontend refreshes on a 401 and retries once.
- **TV apps**: `POST /api/auth/login` and `/api/auth/refresh` return the
  tokens in JSON; requests send `Authorization: Bearer …` (the Roku Video
  node sends headers on stream and segment requests).
- **Throttling**: failed logins rate-limited per IP and per account with
  growing delays, a temporary lock after repeated failures, and every failure
  logged with IP and name.
- Settings: a user sees their signed-in devices and can sign one (or all
  others) out; admins can do it for anyone. Changing or resetting a password
  signs out every other session. `POST /api/auth/logout` revokes the current
  one.
- Tokens from `crypto/rand`; no tokens or passwords in logs or URLs.

**Done when** repeated wrong passwords are throttled, a refreshed token can't
be refreshed twice (and doing so kills the session), and a device signed out
in Settings is refused on its next request.

## S4 · Safe public exposure

**Why.** Once there are accounts, passwords and tokens must not travel in
plain HTTP over the internet.

**Scope**
- Docs for exposing Couchside: the Helm ingress with TLS (cert-manager or
  Traefik's ACME), or a reverse proxy for compose; which paths need long
  timeouts (HLS, live TV).
- Refuse, or warn loudly in the log and on the login page, when accounts are
  on and requests arrive over plain HTTP from outside the LAN.
- Trust `X-Forwarded-For` / `X-Real-IP` only from configured proxies. Today
  chi's `RealIP` accepts them from anyone, so a client exposed directly can
  dodge the per-address login throttle (the per-account one still holds).
- Secure, SameSite cookies when served over HTTPS.

**Done when** README/Helm docs walk through an HTTPS setup, and the server
warns about sign-in over HTTP from a public address.

## S5 · Demo server for Roku reviewers

**Why.** Roku certification needs a working server the reviewer can reach,
showing content we have the right to show. Our real libraries can't be that.

**Scope**
- A separate Couchside instance (compose profile or Helm values) with
  accounts on (a reviewer account with the `user` role), behind HTTPS (S4),
  seeded with openly licensed media: the Blender open movies (Big Buck
  Bunny, Sintel, Tears of Steel…) and a short public-domain series for the
  TV side.
- A script to download and lay out that media and its artwork.
- No live TV or DVR on the demo (no tuner, and no broadcast content).
- Reviewer notes: address, reviewer name and password, what to try.

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
  version and URL, and whether sign-in is needed.
- Works across the usual home setups, including the Roku and server on
  different subnets (a manual address stays as the fallback).

**Done when** the Roku setup screen (R10) lists the server without typing,
on the same subnet at least.
