# Profiles and accounts

Every profile is an account, and every request carries a session. At home
nobody has to type a password: **passwordless sign-in** (on by default) lists
the profiles on a "Who's watching?" screen, and picking one signs in. A
profile with a password still asks for it, so lock the admin's.

Before putting Couchside on the internet, turn passwordless sign-in off in
**Settings → Accounts**, or for good with `COUCHSIDE_AUTH=true` (Helm
`auth.enabled`), and serve it over HTTPS.

## Profiles

Each profile has its own watch progress, Continue Watching, favourite
channels, and settings: next-episode autoplay, subtitle language, commercial
skipping, live TV quality and TV passthrough. Libraries, recordings, series
rules and server settings are shared. Switch profiles from the sidebar.

## Accounts

- **Passwords** are optional while passwordless sign-in is on (Argon2id hashes
  when set). With it off, sign-in is by name and password and no list of names
  is shown.
- **First run.** A new server is passwordless with one admin profile, "Me".
  With `COUCHSIDE_AUTH=true` and no admin password yet, the server log prints a
  one-time setup code; open the UI and enter it with your name and a password.
  Use an existing profile's name to keep its watch history.
- **Roles.** Admins reach Settings, Libraries, file management, Activity and the
  account manager. Users watch and change their own preferences and password.
  Recording is a per-account switch an admin turns on. So is "can set and
  change their own password": turn it off for a shared profile (a Guest that
  anyone picks), so nobody can lock it with a password. Only an admin can then
  set or remove its password, and that password isn't temporary.
- **Account manager** (Settings → Accounts): add accounts (with a temporary
  password changed at first sign-in, or none when passwordless), set role and
  recording, reset or remove passwords, disable or delete accounts, and sign
  out their devices.
- **Lost the admin password?** Run `couchside reset-password -admin <name>`
  inside the container (`kubectl exec -it deploy/couchside -- …`).

## Sessions

- Access tokens last 15 minutes in browsers (renewed in the background) and 12
  hours on TVs, whose video players can't swap tokens mid-film. Signing a
  device out stops it at once either way.
- Refresh tokens are random, stored only as hashes, and replaced on every use;
  a replayed refresh token ends its session. A session ends after a year
  unused (each use pushes that out again), so devices stay signed in. Signing
  out, changing a password, or an admin signing a device out or disabling the
  account ends sessions straight away.
- Failed sign-ins are slowed per address and per account, then locked out for
  up to 15 minutes, and logged.
- **Open without a session:** `/healthz`, the sign-in endpoints, and artwork
  (posters, backdrops, episode stills), so TV apps can load images without a
  token. Everything else, streams included, needs a session.

## For TV apps

Read `GET /api/auth`, sign in with `POST /api/auth/pick`
(`{"profileId", "client": "tv"}`, passwordless) or `POST /api/auth/login`
(`"client": "tv"`), send `Authorization: Bearer <accessToken>`, and renew with
`POST /api/auth/refresh` (`{"refreshToken": …}`).
