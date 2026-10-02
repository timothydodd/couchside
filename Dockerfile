# syntax=docker/dockerfile:1
# Multi-arch (linux/amd64, linux/arm64). The web UI and Go build run on the
# build machine's native platform; Go cross-compiles for the target.

# --- web: build the React UI once --------------------------------------------
FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# --- server: static Go binary with the UI embedded -----------------------------
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS server
ARG TARGETOS TARGETARCH VERSION=dev
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
COPY --from=web /web/dist ./internal/webui/dist
# Couchside's TMDB key comes in as a build secret (never a build arg, which
# would show in the image history); without it TMDB needs TMDB_API_KEY at runtime.
RUN --mount=type=secret,id=tmdb_key \
    TMDB_KEY="$(cat /run/secrets/tmdb_key 2>/dev/null || true)"; \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION} -X github.com/timothydodd/couchside/internal/config.builtinTMDBKey=${TMDB_KEY}" \
      -o /out/couchside ./cmd/couchside

# --- comskip: commercial detection for DVR recordings --------------------------
# Not packaged for Alpine, so it's built from source against Alpine's ffmpeg.
# argtable2 (only in edge) is built static so the runtime needs no extra package;
# its 2005 config.guess/config.sub don't know aarch64, so automake's replace them.
# Runs on the target platform: it links the same ffmpeg libraries as the runtime.
FROM alpine:3.22 AS comskip
ARG COMSKIP_REF=V0.83
RUN apk add --no-cache build-base autoconf automake libtool pkgconf git ffmpeg-dev
# Older C in both projects; gcc 14 would otherwise stop on implicit declarations.
ENV CFLAGS="-O2 -std=gnu17 -Wno-implicit-function-declaration -Wno-incompatible-pointer-types -Wno-int-conversion"
# Debian's copy of the upstream tarball first: SourceForge downloads go down
# (HTTP 522 broke the v0.4.0 build). The checksum pins it either way.
ARG ARGTABLE_SHA256=8f77e8a7ced5301af6e22f47302fdbc3b1ff41f2b83c43c77ae5ca041771ddbf
RUN for u in http://deb.debian.org/debian/pool/main/a/argtable2/argtable2_13.orig.tar.gz \
             https://downloads.sourceforge.net/argtable/argtable2-13.tar.gz; do \
      wget -qO /tmp/argtable2.tgz "$u" && echo "${ARGTABLE_SHA256}  /tmp/argtable2.tgz" | sha256sum -c - && break; \
      rm -f /tmp/argtable2.tgz; \
    done \
 && mkdir -p /src && cp /tmp/argtable2.tgz /src/argtable2-13.tar.gz \
 && tar xzf /tmp/argtable2.tgz -C /tmp \
 && cd /tmp/argtable2-13 \
 && cp /usr/share/automake-*/config.guess /usr/share/automake-*/config.sub . \
 && ./configure --prefix=/usr/local --disable-shared --enable-static \
 && make -j"$(nproc)" && make install
# Both are GPL/LGPL and comskip is shipped as a binary, so their exact source
# goes into the image too (/usr/share/src).
RUN git clone --depth 1 --branch ${COMSKIP_REF} https://github.com/erikkaashoek/Comskip /comskip \
 && git -C /comskip archive --format=tar.gz --prefix=comskip-${COMSKIP_REF}/ -o /src/comskip-${COMSKIP_REF}.tar.gz HEAD \
 && cd /comskip && ./autogen.sh && PKG_CONFIG_PATH=/usr/local/lib/pkgconfig ./configure \
 && make -j"$(nproc)" && strip comskip

# --- runtime: Alpine for ffmpeg ------------------------------------------------
FROM alpine:3.22
# Intel VAAPI drivers (iHD for Broadwell and newer, i965 for older chips) so
# COUCHSIDE_HWACCEL=vaapi can use /dev/dri. x86 only.
RUN apk add --no-cache ffmpeg ca-certificates tzdata \
 && if [ "$(apk --print-arch)" = "x86_64" ]; then apk add --no-cache intel-media-driver libva-intel-driver; fi \
 && adduser -D -H -u 1000 couchside \
 && mkdir -p /data /cache /media /recordings \
 && chown couchside:couchside /data /cache /recordings
COPY --from=comskip /comskip/comskip /usr/local/bin/comskip
COPY --from=comskip /src/ /usr/share/src/
COPY LICENSE THIRD_PARTY_NOTICES.txt /usr/share/licenses/couchside/
COPY --from=server /out/couchside /usr/local/bin/couchside

ENV COUCHSIDE_ADDR=:8080 \
    COUCHSIDE_DATA_DIR=/data \
    COUCHSIDE_CACHE_DIR=/cache \
    COUCHSIDE_MEDIA_ROOT=/media \
    COUCHSIDE_RECORDINGS_DIR=/recordings

USER 1000:1000
EXPOSE 8080
VOLUME ["/data", "/cache", "/recordings"]
ENTRYPOINT ["couchside"]
