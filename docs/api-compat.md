# API compatibility

The web UI ships with the server, but other clients don't: the Roku channel
updates through the Roku store on its own schedule, so an old channel meets a
new server and a new channel meets an old one. These rules keep that working.

## What a client asks first

`GET /api/server` is open (no sign-in), like `/api/discovery`:

```json
{
  "app": "couchside",
  "id": "8f1c…",
  "name": "Den",
  "version": "0.19.0",
  "apiVersion": 1,
  "signIn": "passwordless",
  "features": ["deviceCode", "editions", "hls", "livetv", "…"]
}
```

| Field | Type | Notes |
| --- | --- | --- |
| `version` | string | The release (`0.19.0`), or `dev` for a local build. For people, not for comparing. |
| `apiVersion` | number | Goes up only on a breaking change (below). Clients compare it with the lowest they understand. |
| `signIn` | string | `passwordless` or `password`. |
| `features` | string[] | Sorted. What this server can do now, from its real state. |

The same `apiVersion` and `features` are in `GET /api/status` for clients
already signed in.

A `404` from `/api/server` means a server older than 0.19: treat it as
`apiVersion` 1 with every feature that release had. A feature name a client
doesn't know means nothing to it.

### Feature names

| Name | Means |
| --- | --- |
| `deviceCode`, `editions`, `hls`, `optimize`, `people`, `segments`, `totp`, `trickplay`, `virtualChannels`, `watchlist` | API surfaces every 0.19+ server has. |
| `hwaccel` | A GPU encoder passed its test. |
| `livetv` | Live TV is set up (a tuner or a virtual channel). |
| `tuner`, `dvr` | An HDHomeRun is configured: tuning and recording. |
| `commercials` | comskip is installed, so recordings get commercial breaks. |
| `metadata` | A metadata provider (TMDB, OMDb) is configured. |
| `passwordless` | Picking a profile signs in. |
| `oidc` | Single sign-on is configured. |
| `discovery` | The server answers SSDP searches on the LAN. |

## Rules for the server

- Fields are only ever added. A field never changes type or meaning, and
  isn't removed.
- A route is never removed or changed in place. A breaking change gets a new
  route (or `/api/v2/…`) and bumps `apiVersion`; the old route stays for at
  least one release after clients have moved.
- Enums are strings. Adding a value is allowed, so clients treat an unknown
  value as "other".
- New list endpoints answer an object, `{"items": [...]}`, so paging or totals
  can be added later. The bare arrays that exist (`GET /api/items`,
  `/api/dvr/recordings`, `/api/profiles`) stay as they are.
- Nullable fields stay nullable. Don't add `omitempty` to a field clients
  read as a boolean or number: the Roku would see it go missing.

## Rules for clients

- Read `/api/server` before signing in. Refuse to continue, with "Update your
  Couchside server", when `apiVersion` is lower than the client needs.
- Gate optional calls on `features`, and treat a missing list as "has it".
- Ignore fields you don't know.
