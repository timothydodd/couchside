# Security

## Reporting a vulnerability

Please report it privately through
[GitHub's private vulnerability reporting](https://github.com/timothydodd/couchside/security/advisories/new),
not in a public issue. Include the version, how to reproduce it, and what an
attacker could do with it.

Couchside is maintained by one person in their spare time. Expect a reply within
a week; fixes ship in a new release, credited to you unless you'd rather not be.

## Supported versions

Only the latest release gets fixes. Update with
`docker pull ghcr.io/timothydodd/couchside:latest` or a new zip from the
[Releases page](https://github.com/timothydodd/couchside/releases).

## Running it safely

Couchside is built for a home network. If you expose it to the internet:

- **Put it behind HTTPS** (a reverse proxy or tunnel). Couchside itself serves
  plain HTTP, and warns when someone signs in over HTTP from the internet.
- **Turn passwordless sign-in off**, or set `COUCHSIDE_AUTH=true` to require
  passwords. With passwordless on, anyone who can reach the server can pick a
  profile without a password.
- **Give every admin a password**, and untick "Can set and change their own
  password" on shared profiles.
- **Know what's public:** posters, backdrops and episode stills are served
  without signing in, so TVs can load them. Everything else needs a session.
- **Name your proxy** in `COUCHSIDE_TRUSTED_PROXIES`. Forwarding headers are
  believed only from the addresses listed there, so sign-in throttling counts
  each visitor's real address. Left empty, it sees the proxy's address and
  failed attempts from everyone count together.

## In scope

The server, web app and container image in this repository: for example,
signing in without a valid password, reaching another profile's data or an
admin route as a user, reading or deleting files outside a library, or running
commands on the server.

Out of scope: bugs in FFmpeg, Comskip or other bundled software (report those
upstream), and anything that needs admin access already.
