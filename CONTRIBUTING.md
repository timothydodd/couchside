# Contributing

Thanks for helping. Couchside is looked after by one person in spare time, so
small, focused pull requests get in fastest.

- **Bugs:** open an issue with the bug form and attach the diagnostics zip
  (Settings → System → Advanced → Diagnostics).
- **Features:** open an issue first to talk it through. Couchside stays a
  self-hosted media server with no telemetry and no cloud service of its own.
- **Code:** Go in `server/`, React and TypeScript in `web/`, the Helm chart in
  `deploy/helm/couchside`. Run the checks in [docs/development.md](docs/development.md)
  before pushing; CI runs them too, with govulncheck, npm audit and a chart
  lint.
- **API changes:** the Roku channel updates on its own schedule, so follow
  [docs/api-compat.md](docs/api-compat.md): add fields, don't change them.
- **Style:** match the code around yours. Colours and tokens come from
  [docs/style.md](docs/style.md).
- **Dependencies:** they need a license compatible with MIT (MIT, ISC, BSD,
  Apache) for anything compiled in; GPL only for programs run as separate
  processes, like ffmpeg. Regenerate the notices with
  `python3 scripts/third-party-notices.py`; CI checks they're current.
- **Security:** never in a public issue; see [SECURITY.md](SECURITY.md).

By contributing you agree your work is released under the MIT License.
