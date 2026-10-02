# Installing Couchside

Every release on the [Releases page](https://github.com/timothydodd/couchside/releases)
has zips for Linux, macOS and Windows, plus a multi-arch container image
(amd64 and arm64) at `ghcr.io/timothydodd/couchside`.

Once it's running, open http://localhost:8080 (or your host) and add a library
on the Libraries page. Settings are covered in [configuration.md](configuration.md).

## Container

```bash
docker run -d --name couchside -p 8080:8080 \
  -v couchside-data:/data -v couchside-cache:/cache \
  -v /path/to/media:/media \
  ghcr.io/timothydodd/couchside:latest
```

- Mount the media read-only (`/path/to/media:/media:ro`) unless you want the DVR
  to record into a library folder, or want to delete files from the Manage view.
- Add `-e COUCHSIDE_HDHOMERUN=<tuner IP>` for [Live TV](live-tv.md).
- Add `-e TZ=America/New_York` (your zone) so guide times and recording names are local.
- For TV apps to find the server on their own (LAN discovery), run it with
  `--network host` instead of `-p`: SSDP's multicast doesn't reach a container
  on Docker's bridge network. Otherwise type the address into the TV app.
- For GPU transcoding, pass the GPU in (`--device /dev/dri`) and set
  `-e COUCHSIDE_HWACCEL=vaapi`. See [playback.md](playback.md#hardware).

## Zip

Each zip holds a single `couchside` binary with the web UI built in. Install
`ffmpeg` (which includes `ffprobe`), then run:

```bash
COUCHSIDE_MEDIA_ROOT=/path/to/media ./couchside
```

On Windows, set the variable first and run `couchside.exe`. For commercial
detection, put [Comskip](https://github.com/erikkaashoek/Comskip) on the PATH
or set `COUCHSIDE_COMSKIP` (the container image has it built in).

## Docker Compose against a NAS share

Copy `.env.example` to `.env`, fill it in, and run `docker compose up -d`.
Docker Desktop can't see mapped network drives, so the compose file mounts the
SMB share directly. It mounts the share writable so the DVR can record into
your TV library.

## Kubernetes / k3s (Helm)

```bash
kubectl create namespace media

helm install couchside deploy/helm/couchside -n media \
  --set media.type=nfs --set media.nfs.server=192.168.1.10 --set media.nfs.path=/volume1/media \
  --set ingress.enabled=true --set ingress.host=couchside.home.lan
```

- Media can be NFS, a hostPath or an existing PVC (`media.type`). The SQLite
  database lives on its own PVC. Media is mounted read-only unless you set
  `media.readOnly=false`, which recording into a library folder and deleting
  files from the Manage view both need.
- Live TV: `--set livetv.hdhomerun=<tuner IP> --set timezone=America/New_York`,
  and `persistence.recordings.enabled=true` for a recordings volume (without
  one, recordings are lost when the pod restarts). Anything else goes in
  `extraEnv`.
- The memory limit (4Gi) covers the server and every ffmpeg it runs; raise it
  for several software 4K transcodes at once.
- The Deployment uses `strategy: Recreate` because the database sits on a
  ReadWriteOnce volume. Don't scale it past one replica.
- LAN discovery needs `discovery.hostNetwork=true` (multicast doesn't reach
  the pod network); the pod then serves on the node's port 8080.
- Optional keys come from Secrets: `tmdb.existingSecret` for your own TMDB key
  (release builds have one built in) and `omdb.existingSecret` for the OMDb
  fallback. See `deploy/helm/couchside/values.yaml` for everything else,
  including `auth.enabled` and GPU settings.

## Putting it on the internet

Turn off passwordless sign-in first (`COUCHSIDE_AUTH=true`, Helm
`auth.enabled`), give every account a password, and serve it over HTTPS. See
[accounts.md](accounts.md).
