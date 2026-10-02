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
  database lives on its own PVC.
- The Deployment uses `strategy: Recreate` because the database sits on a
  ReadWriteOnce volume. Don't scale it past one replica.
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
