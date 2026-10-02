# Development

- **Backend:** one static Go binary (`server/`), pure-Go SQLite (no CGO), and
  ffmpeg/ffprobe for probing, stills and transcoding. Go 1.26; the `go`
  command fetches it automatically.
- **Frontend:** React 19, Vite and Tailwind v4 (`web/`). Colours and component
  styles are in [style.md](style.md).
- **Deploy:** a Helm chart in `deploy/helm/couchside`.

`CLAUDE.md` at the repo root has the architecture notes: the job queue, the
scan and match flow, encoding, live TV and the DVR.

## Running locally

```bash
# API on :8080
cd server
COUCHSIDE_MEDIA_ROOT=/path/to/media go run ./cmd/couchside

# UI on :5173, proxied to the API
cd web
npm install
npm run dev
```

A plain `go build` serves the API only. Release builds copy `web/dist` into
`server/internal/webui/dist` first, which embeds the UI; `COUCHSIDE_WEB_DIR`
points at a UI folder on disk instead.

## Checks

```bash
cd server && go vet ./... && go test ./...
cd web && npx tsc --noEmit && npx vite build
```

CI (`.github/workflows/ci.yml`) runs both on pushes and pull requests.

Dependabot (`.github/dependabot.yml`) opens weekly update pull requests for Go,
npm, GitHub Actions and the Dockerfile's base images. When a push to main
changes `go.sum` or `package-lock.json`, `notices.yml` regenerates
`THIRD_PARTY_NOTICES.txt` and commits it. To do it by hand:
`python3 scripts/third-party-notices.py` (with `web/node_modules` installed).

## Icons

`python3 branding/make-icons.py` regenerates every icon size from
`branding/logo-c-original.png`, and `python3 branding/make-transparent.py`
cuts the logo out onto a transparent background.

## Releasing

Push a version tag. The release workflow builds the zips (Linux and macOS
amd64/arm64, Windows amd64), publishes the container to
`ghcr.io/<owner>/couchside` (`:<version>`, `:<major>.<minor>`, `:latest`), and
creates a GitHub Release with checksums. Tags with a hyphen, such as
`v0.2.0-rc1`, become pre-releases and don't move `:latest`.

```bash
git tag -a v0.2.0 -m "v0.2.0: …" && git push origin v0.2.0
```
