# Profiles and accounts

Every profile is an account, and every request carries a session. At home
nobody has to type a password: **passwordless sign-in** (on by default) lists
the profiles on a "Who's watching?" screen, and picking one signs in. A
profile with a password still asks for it, so lock the admin's.

Before putting Couchside on the internet, turn passwordless sign-in off in
**Settings → Accounts**, or for good with `COUCHSIDE_AUTH=true` (Helm
`auth.enabled`), and serve it over HTTPS behind a proxy you name in
`COUCHSIDE_TRUSTED_PROXIES` ([install.md](install.md#putting-it-on-the-internet)).

## Profiles

Each profile has its own watch progress, Continue Watching, favourite
channels, and settings: next-episode autoplay, subtitle language, commercial
skipping, live TV quality and TV passthrough. Libraries, recordings, series
rules and server settings are shared. Switch profiles from the sidebar.

## Accounts

- **Passwords** are optional while passwordless sign-in is on (Argon2id hashes
  when set). With it off, sign-in is by name and password and no list of names
  is shown.
- **Hiding admins.** With passwordless sign-in on, *Hide admin accounts on
  the sign-in page* (Settings → Accounts) leaves admins off the profile
  picker, in the browser and in TV apps. Admins sign in at `/admin` instead.
  It suits a demo server, where visitors needn't see the admin account. It
  only keeps the account out of sight, so give it a password.
- **First run.** A new server is passwordless with one admin profile, "Me".
  Until the first run is finished, only a browser on the home network (or the
  server itself, a VPN or Tailscale) can claim it. From anywhere else it asks
  for the one-time setup code printed in the server log, and a password.
  Behind a reverse proxy, set `COUCHSIDE_TRUSTED_PROXIES` so the server sees
  visitors' real addresses (or every visitor looks like the proxy, which is on
  the home network). With `COUCHSIDE_AUTH=true` and no admin password yet, the
  log prints the setup code too; open the UI and enter it with your name and a
  password. Use an existing profile's name to keep its watch history.
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
- **What an account sees:** tick libraries to limit an account to those, and
  pick a rating ("Up to PG") to hide anything rated higher; TV ratings count
  (TV-PG as PG, TV-14 as PG-13, TV-MA as R), and with a limit, titles nobody
  has rated are hidden too. A hidden title can't be listed, searched for,
  opened or played. Admins see everything. Live TV isn't limited, and
  posters are public (see below).
- **Lost the admin password?** Run `couchside reset-password -admin <name>`
  inside the container (`kubectl exec -it deploy/couchside -- …`).

## Sessions

- Access tokens last 15 minutes in browsers (renewed in the background) and 12
  hours on TVs, whose video players can't swap tokens mid-film. Signing a
  device out stops it at once either way.
- Refresh tokens are random, stored only as hashes, and replaced on every use;
  a replayed refresh token ends its session. A TV app may repeat its last
  refresh when the answer didn't arrive; the token it never received stops
  working. A session ends after a year
  unused (each use pushes that out again), so devices stay signed in. Signing
  out, changing a password, or an admin signing a device out or disabling the
  account ends sessions straight away. So does turning passwordless sign-in
  off (or starting with `COUCHSIDE_AUTH=true`) for profiles without a
  password. A profile keeps at most 50 sessions; signing in again drops the
  least recently used.
- Failed sign-ins are slowed per address and per account, then locked out for
  up to 15 minutes, and logged. The per-account limit means someone who knows
  a name can keep that account locked by failing on purpose; that's the price
  of stopping password guessing from many addresses. Passwordless picks are
  limited to 20 per address per 10 minutes.
- **Open without a session:** `/healthz` (the database answers), `/livez`
  (the process is up), `/api/server` (version, API version and features),
  `/api/discovery` (name, version and
  sign-in mode, for TV apps finding the server), the sign-in endpoints, and
  artwork (posters, backdrops, episode stills, cast photos and
  `/api/artwork/remote`, the cache of guide and provider images), so TV apps
  can load images without a token. Everything else, streams included, needs
  a session.

## For TV apps

**Finding the server.** Send an SSDP search to `239.255.255.250:1900`:

```
M-SEARCH * HTTP/1.1
HOST: 239.255.255.250:1900
MAN: "ssdp:discover"
MX: 1
ST: urn:couchside-app:device:server:1
```

Each Couchside on the LAN answers (unicast, within a second) with
`LOCATION: <base>/api/discovery`, `USN: uuid:<server id>::…` and
`X-COUCHSIDE-NAME`, `X-COUCHSIDE-VERSION`, `X-COUCHSIDE-URL` (the base URL)
and `X-COUCHSIDE-SIGNIN` (`passwordless` or `password`). `GET /api/discovery`
returns the same as JSON without signing in. The server id survives restarts,
so a TV can remember which server it picked. Discovery only works on the same
network segment; typing the address stays the fallback.

Read `GET /api/auth`, sign in with `POST /api/auth/pick`
(`{"profileId", "client": "tv"}`, passwordless) or `POST /api/auth/login`
(`"client": "tv"`), send `Authorization: Bearer <accessToken>`, and renew with
`POST /api/auth/refresh` (`{"refreshToken": …}`).

## Single sign-on

If you already run an OpenID Connect provider (Authelia, Authentik, Keycloak,
or a hosted one), Couchside can use it for signing in on the web.

1. In the provider, register a client for Couchside. Its redirect (callback)
   address is shown in Settings → Accounts → Single sign-on; it's
   `https://<your couchside>/api/auth/oidc/callback`.
2. In that card, enter the provider's address, the client id and the secret.
   Saving checks the provider can be reached. The provider (and its token
   endpoint) must use https unless it's on your own network (a private
   address, `localhost`, or a name like `auth.lan` or `sso.home.arpa`):
   Couchside trusts the identity it gets back because of that connection.
3. Choose which claim holds the profile name (`preferred_username` by
   default; `email` or `name` also work). Someone is signed in to the profile
   with that name. Tick "Make a profile…" to create one for people who don't
   have one; give an admin group to make people in it admins when their
   profile is created.

The sign-in page then has a button for it. Passwords set here keep working,
and `couchside reset-password` is still the way back in if the provider is
down.

Use it with passwordless sign-in **off**: a profile created by single sign-on
has no password, and with passwordless on anyone could pick it.

## Signing in a TV with a code

A TV app can show a short code instead of asking for a password. On a phone
or computer where you're signed in, open `/link` (or Settings → Your account
→ Sign in a TV), enter the code, and the TV is signed in as you. It works for
every kind of account, including ones that use single sign-on.

## Two-step sign-in

Anyone with a password can add a second step: Settings → Your account →
Two-step sign-in → Turn on. Scan the QR code with an authenticator app
(Google Authenticator, Aegis, 1Password…), type the code it shows, and save
the eight recovery codes. From then on signing in with the password also
asks for the app's code; a recovery code works once in its place.

Lost the phone and the codes? An admin can turn it off for the account in
Settings → Accounts, and `couchside reset-password <name>` clears it along
with setting a new password.

It covers signing in with a password. Sign a TV in with a code from `/link`
rather than typing both with a remote.
