# Server stories

What's left on the server and web app. The second code audit's stories
(3 October 2026: fixes B1 to B23, cleanup C1 to C6, features F1 to F10) and
the carried-over S5 (demo server) and S7 (subtitles for TVs) are all done
and removed from this file; git history and the pull requests have them
(the audit-2 branch, PRs #50 to #62; the first audit's A1 to A17 are PRs #8
to #26). The Roku app's share is in
[couchside-roku](https://github.com/timothydodd/couchside-roku)
`docs/stories.md` (`R-n`).

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
- The Roku is for watching only: a server feature's management side (library,
  server, accounts, channels) never gets a Roku story.

---

# Open

| ID  | Story                       | Needed by | State            |
| --- | --------------------------- | --------- | ---------------- |
| S8  | Server discovery on the LAN | R10       | Device test left |

## S8 · Server discovery on the LAN

Done (PR #28) for the same subnet. Left: try it on a real Roku with
`discovery.hostNetwork`.

## Loose ends

- The privacy page on the site still carries the landing page's unused CSS
  (left from B22).
- Most of the audit's web changes were typechecked and built, not tried in a
  browser: a pass over the new Settings cards, the player menus and the
  Manage view is owed before a release.
- `demo.couchside.app` is up (6 October 2026). Left: the cross-network check
  from a Roku, for R7.

---

## Not planned

- A second guide source (XMLTV, M3U): what's there covers it.
- Music, audiobooks and photos; offline downloads; apps for other TV
  platforms; watch together; DLNA; plugins. Each is a separate product or
  works against the one-process, one-SQLite-file design.
- Asked for, not yet planned: subtitle download and styling, watch history
  and stats, collections, trailers, API keys and webhooks, Trakt, NFO files,
  multi-episode files, metadata language, an NVENC/QSV image, missing
  episodes, recording priorities, SAP audio, AirPlay and Chromecast.
