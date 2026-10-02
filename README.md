<p align="center"><img src="web/public/icons/logo-256.png" alt="Couchside" width="128" height="128"></p>

# Couchside

[couchside.app](https://couchside.app)

A lightweight, self-hosted media server for your home. Point it at your movie
and TV folders and it matches them against [TMDB](https://www.themoviedb.org),
fetches artwork, and plays them in any browser or on a TV, with live TV and a
DVR if you have an HDHomeRun tuner.

- **Your library, matched.** Posters, backdrops, plots, cast and episode titles
  with no setup. Split movies play as one, and extras are listed separately.
- **Plays anywhere.** Direct play when the device can, otherwise transcoding on
  the fly, on the GPU with VAAPI. Subtitles, audio tracks, quality presets.
- **Live TV and DVR.** Guide, recording, series rules, and automatic commercial
  skipping.
- **For the whole household.** Profiles with their own progress and settings,
  passwordless at home, proper accounts when exposed to the internet.
- **Small.** One Go binary with SQLite built in, or one container. A Helm chart
  for Kubernetes/k3s.

## Quick start

```bash
docker run -d --name couchside -p 8080:8080 \
  -v couchside-data:/data -v couchside-cache:/cache \
  -v /path/to/media:/media \
  ghcr.io/timothydodd/couchside:latest
```

Open http://localhost:8080 and add a library on the Libraries page. Add
`-e COUCHSIDE_HDHOMERUN=<tuner IP>` for live TV.

Zips for Linux, macOS and Windows are on the
[Releases page](https://github.com/timothydodd/couchside/releases).

## Documentation

- [Installing](docs/install.md): container, zip, Docker Compose, Helm
- [Configuration](docs/configuration.md): environment variables and settings
- [Libraries and metadata](docs/library.md)
- [Playback and transcoding](docs/playback.md)
- [Live TV and DVR](docs/live-tv.md)
- [Profiles and accounts](docs/accounts.md)
- [Development](docs/development.md): building, testing and releasing
