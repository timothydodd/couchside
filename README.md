<p align="center"><img src="web/public/icons/logo-256.png" alt="Couchside" width="128" height="128"></p>

# Couchside

[couchside.app](https://couchside.app)

A fast, lightweight media server for your home: your movies, your shows and
live TV from an HDHomeRun tuner, with a DVR. It runs on your own network,
answers to no cloud account, and puts care into the parts you look at.

## What it's for

- **Fast and light.** One Go binary with SQLite built in: no database server,
  no runtime to install, nothing to tune. The release is a 7 MB download, and
  a small server idles at a few tens of megabytes of memory, so it's happy on
  a NAS, a Raspberry Pi or a corner of a k3s cluster. Pages open instantly and
  grids scroll smoothly through thousands of titles.
- **Local only.** Built for your home network. There's no Couchside account,
  no telemetry and nothing that phones home. The server only reaches out for
  what you set up: TMDB for metadata, and the HDHomeRun guide. It caches
  everything it fetches, artwork included, so your devices never talk to
  anyone but your server.
- **Live TV and DVR done properly.** A guide, one-off and series recordings
  (new episodes only, skip what's in your library, keep the last N), recording
  padding, watching a recording while it's still going, and automatic
  commercial detection and skipping. Recordings survive restarts, tuners can be
  shared with another DVR, and channels the TV can decode are passed straight
  through instead of transcoded.
- **A UI worth using.** A clean dark web app that works just as well on a
  phone, a Plex-style player with subtitles, audio tracks and quality presets,
  and a Roku app for the TV.

## Everything else

- **Your library, matched.** Point it at your folders: titles, posters,
  backdrops, plots, cast and episode names come from TMDB. Split movies play as
  one, extras are listed separately, and a Manage view finds duplicates and
  low-quality copies.
- **Plays anything.** Direct play when the device can, otherwise on-the-fly
  HLS, on Intel GPUs with VAAPI, with HDR tone mapping.
- **For the whole household.** Profiles keep their own progress, favourites
  and settings. Sign-in is a tap on your profile at home, or passwords when you
  want them. Admins decide who can record.

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

## Attributions

<a href="https://www.themoviedb.org"><img src="web/public/brand/tmdb.svg" alt="TMDB" height="20"></a>

**This product uses the TMDB API but is not endorsed or certified by TMDB.**
Movie and TV metadata and artwork come from [The Movie Database (TMDB)](https://www.themoviedb.org).

- **[OMDb API](https://www.omdbapi.com)**: fallback metadata, licensed
  [CC BY-NC 4.0](https://creativecommons.org/licenses/by-nc/4.0/).
- **[HDHomeRun](https://www.silicondust.com)**: live TV and the program guide
  come from your HDHomeRun tuner and SiliconDust's guide service. HDHomeRun is
  a trademark of SiliconDust USA Inc.; Couchside isn't affiliated with them.
- **[FFmpeg](https://ffmpeg.org)**: all transcoding, remuxing, probing and
  recording. The container image includes the Alpine Linux build of FFmpeg,
  under the [GPL](https://ffmpeg.org/legal.html).
- **[Comskip](https://github.com/erikkaashoek/Comskip)**: commercial
  detection. The container image builds it from source, under the GPL v2.
- **[Intel Media Driver](https://github.com/intel/media-driver)** and
  **libva-intel-driver**: GPU transcoding in the container image (MIT).

Couchside is built with:

| Project | License |
| --- | --- |
| [Go](https://go.dev), [golang.org/x/crypto](https://pkg.go.dev/golang.org/x/crypto), [golang.org/x/term](https://pkg.go.dev/golang.org/x/term) | BSD-3-Clause |
| [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) (SQLite itself is public domain) | BSD-3-Clause |
| [chi](https://github.com/go-chi/chi) | MIT |
| [React](https://react.dev) | MIT |
| [hls.js](https://github.com/video-dev/hls.js) | Apache-2.0 |
| [TanStack Virtual](https://tanstack.com/virtual) | MIT |
| [Zustand](https://github.com/pmndrs/zustand) | MIT |
| [Lucide](https://lucide.dev) icons | ISC |
| [Tailwind CSS](https://tailwindcss.com) | MIT |
| [Vite](https://vite.dev) | MIT |

Trademarks belong to their owners.
