# Installing Couchside

Every release on the [Releases page](https://github.com/timothydodd/couchside/releases)
has a Windows installer, zips for Linux, macOS and Windows, and a multi-arch
container image (amd64 and arm64) at `ghcr.io/timothydodd/couchside`.

Once it's running, open http://localhost:8080 (or your host) and add a library
on the Libraries page. Settings are covered in [configuration.md](configuration.md).

## Windows installer

Run `couchside-<version>-windows-amd64-setup.exe`. It asks for a port, then:

- installs Couchside with ffmpeg beside it (jellyfin-ffmpeg, whose NVENC runs on
  older NVIDIA drivers than upstream builds), so there's nothing else to install;
- runs it as the **Couchside** Windows service, which starts with Windows and
  restarts if it stops unexpectedly;
- lists your graphics cards. Couchside picks the encoder at start-up: NVIDIA
  NVENC first, then Intel Quick Sync, else the CPU. Settings → System shows the
  one in use, or why a GPU couldn't be used (for example a driver too old);
- adds a Windows Firewall rule (private and domain networks) so TVs and phones
  can connect and find the server.

The first time you open Couchside in the browser, it asks for your name (and a
password, if you want one), then where your media is: drives, folders or
network shares.

The installer's settings are in `%ProgramData%\Couchside\couchside.env` (see
[configuration.md](configuration.md)). Most of them, and more, can be changed
in Settings → Server instead, which restarts the server for you.
The database, artwork and logs (`data\logs\couchside.log`) are in
`%ProgramData%\Couchside\data` and are kept when you uninstall. Running the
installer again upgrades in place and keeps your settings.

**Media on a NAS.** Add it as a network share location with its network path
(`\\nas\media`), not a mapped drive letter (services can't see mapped
drives), and the NAS's user name and password. The service runs as Local
System, which a NAS usually turns away, so Couchside signs in to the share
itself whenever it starts (like `net use`); the password is kept encrypted for
this computer (Windows DPAPI). Alternatively, in Services (`services.msc`) open
Couchside → Log On, choose an account that can read the share, and restart the
service.

Test builds of the installer come from `pre-*` tags: the installer is attached to
that workflow run (Actions → prerelease → windows-installer), not published.

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
- The container runs as user 1000. A bind-mounted `/data` must belong to it:
  Couchside keeps the folder and its database readable by that user only,
  since they hold password hashes and the session key.

## Zip

Each zip holds a single `couchside` binary with the web UI built in. Install
`ffmpeg` (which includes `ffprobe`), then run:

```bash
COUCHSIDE_MEDIA_ROOT=/path/to/media ./couchside
```

`COUCHSIDE_MEDIA_ROOT` is optional: without it, the first run in the browser
asks where your media is. On Windows, run `couchside.exe` (or use the
installer above). An `ffmpeg.exe` next to `couchside.exe` is used before the
one on the PATH. (On Windows a conversion isn't paused when it gets ahead of the player, so a stream converts its whole file while it's open.) For commercial
detection, put [Comskip](https://github.com/erikkaashoek/Comskip) on the PATH
or set `COUCHSIDE_COMSKIP` (the container image has it built in).

## Docker Compose against a NAS share

Copy `.env.example` to `.env`, fill it in, and run `docker compose up -d`. That runs the published image; to build from a checkout instead, add `-f docker-compose.yml -f docker-compose.dev.yml` and `--build` (a local build has no built-in TMDB key, so set `TMDB_API_KEY`).
Docker Desktop can't see mapped network drives, so the compose file mounts the
SMB share directly. It mounts the share writable so the DVR can record into
your TV library.

## Kubernetes / k3s (Helm)

```bash
kubectl create namespace media

helm install couchside deploy/helm/couchside -n media \
  --set media.type=nfs --set media.nfs.server=192.168.1.10 --set media.nfs.path=/volume1/media \
  --set ingress.enabled=true --set ingress.host=couchside.home.lan \
  --set auth.enabled=true
```

The pod log prints a one-time setup code; open the UI and enter it with your
name and a password.

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
- GPU (Intel or AMD, VAAPI): install a GPU device plugin and set
  `--set hwaccel.mode=vaapi --set hwaccel.dri.resource=gpu.intel.com/i915
  --set 'hwaccel.dri.groups={<render gid>}'` (Intel's plugin; `squat.ai/dri`
  with generic-device-plugin). Without a plugin,
  `hwaccel.dri.enabled=true hwaccel.dri.privileged=true` mounts `/dev/dri` from
  the node into a privileged container. The image's ffmpeg has no NVENC or
  Quick Sync.
- The chart refuses an Ingress without `auth.enabled=true`. For an ingress
  only your LAN can reach, `auth.allowOpenIngress=true` keeps passwordless
  sign-in.
- The Deployment uses `strategy: Recreate` because the database sits on a
  ReadWriteOnce volume. Don't scale it past one replica. Upgrading restarts
  the pod: a recording in progress is cut and resumed by the new one.
- Liveness uses `/livez` (the process answers) and readiness `/healthz` (the
  database answers), so a long scan or backup doesn't get the pod restarted.
- LAN discovery needs `discovery.hostNetwork=true` (multicast doesn't reach
  the pod network); the pod then serves on the node's port 8080.
- Optional keys come from Secrets: `tmdb.existingSecret` for your own TMDB key
  (release builds have one built in) and `omdb.existingSecret` for the OMDb
  fallback. See `deploy/helm/couchside/values.yaml` for everything else,
  including `auth.enabled` and GPU settings.

## Putting it on the internet

Couchside serves plain HTTP. On the internet, put it behind something that
does HTTPS, and lock sign-in down first:

1. **Require passwords**: `COUCHSIDE_AUTH=true` (Helm `auth.enabled`), and give
   every account a password. See [accounts.md](accounts.md).
2. **Serve it over HTTPS** with a reverse proxy or the ingress (below). The
   sign-in page and the server log warn when someone signs in over plain HTTP
   from an internet address.
3. **Tell Couchside which proxy to believe**: `COUCHSIDE_TRUSTED_PROXIES` (Helm
   `auth.trustedProxies`), the proxy's address or network. Only then are
   `X-Forwarded-For` (the visitor's address, for the sign-in throttle, the
   sessions list and the log) and `X-Forwarded-Proto` (so sign-in cookies are
   marked `Secure`) taken from it. From anyone else they're ignored, so a
   client can't dodge the throttle by sending its own. Left empty, everyone
   seems to come from the proxy's address and shares one throttle.

The proxy needs long timeouts on `/api` (HLS segments are made on request,
and live TV playlists are polled for hours) and a request body of at least
25 MB for artwork uploads. Keep buffering off for `/api/files/*/stream`.

**Helm with TLS.** With cert-manager:

```bash
helm upgrade --install couchside deploy/helm/couchside -n media --reuse-values \
  --set auth.enabled=true \
  --set ingress.enabled=true --set ingress.host=tv.example.com \
  --set ingress.tls.enabled=true --set ingress.tls.clusterIssuer=letsencrypt \
  --set 'auth.trustedProxies={10.42.0.0/16}'
```

`10.42.0.0/16` is k3s's pod network, where Traefik runs. For Traefik to see
visitors' real addresses at all, its Service needs
`externalTrafficPolicy: Local`. Traefik's own ACME resolver works too: leave
`clusterIssuer` empty and add its `traefik.ingress.kubernetes.io/router.tls.certresolver`
annotation under `ingress.annotations`.

**Docker or a zip behind Caddy.** Caddy gets a certificate on its own:

```
tv.example.com {
	reverse_proxy localhost:8080
}
```

Run Couchside with `COUCHSIDE_TRUSTED_PROXIES=127.0.0.1` (or the Docker
network's address range, e.g. `172.16.0.0/12`, when Caddy runs in a container).
nginx works as well: set `proxy_set_header X-Forwarded-For
$proxy_add_x_forwarded_for;` and `X-Forwarded-Proto $scheme;`,
`proxy_read_timeout 3600s;`, `proxy_buffering off;` and
`client_max_body_size 25m;`.
