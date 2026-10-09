# syntax=docker/dockerfile:1
# Multi-arch (linux/amd64, linux/arm64). The web UI and Go build run on the
# build machine's native platform; Go cross-compiles for the target.

# --- web: build the React UI once --------------------------------------------
FROM --platform=$BUILDPLATFORM node:26-alpine@sha256:0b36e8c136b94cd4fcf02188228e76c31ad5872eef3fec8cbd2eee500cfd9e80 AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# --- server: static Go binary with the UI embedded -----------------------------
FROM --platform=$BUILDPLATFORM golang:1.27-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS server
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
FROM alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6 AS comskip
ARG COMSKIP_REF=V0.83
# The tag's commit: the build stops if the tag is ever moved.
ARG COMSKIP_COMMIT=55b0bcd018ddb9dacfad79addc48df55c1411073
RUN apk add --no-cache build-base autoconf automake libtool pkgconf git ffmpeg-dev ffmpeg
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
#
# FFmpeg 8 removed AVCodecContext.ticks_per_frame, which V0.83 reads. Its
# documented replacement is 2 for codecs that code fields (MPEG-2, H.264) and
# 1 otherwise. (Upstream PR #187 uses "props & AV_CODEC_PROP_FIELDS", which
# is 16 or 0, and would break broadcast timing.) The patch is shipped with the
# source.
RUN git clone --depth 1 --branch ${COMSKIP_REF} https://github.com/erikkaashoek/Comskip /comskip \
 && test "$(git -C /comskip rev-parse HEAD)" = "${COMSKIP_COMMIT}" \
 && git -C /comskip archive --format=tar.gz --prefix=comskip-${COMSKIP_REF}/ -o /src/comskip-${COMSKIP_REF}.tar.gz HEAD \
 && cd /comskip \
 && sed -i -e 's/is->dec_ctx->ticks_per_frame = 1;/;/' \
      -e 's/is->dec_ctx->ticks_per_frame/COMSKIP_TICKS(is->dec_ctx)/g' mpeg2dec.c \
 && sed -i '1i #define COMSKIP_TICKS(c) (((c)->codec_descriptor \&\& ((c)->codec_descriptor->props \& AV_CODEC_PROP_FIELDS)) ? 2 : 1)' mpeg2dec.c \
 && ! grep -q 'dec_ctx->ticks_per_frame' mpeg2dec.c \
 && git diff > /src/comskip-${COMSKIP_REF}-ffmpeg8.patch \
 && ./autogen.sh && PKG_CONFIG_PATH=/usr/local/lib/pkgconfig ./configure \
 && make -j"$(nproc)" && strip comskip
# Smoke test: comskip reads an MPEG-2 broadcast-like clip and writes an EDL.
# (It can exit 1 on success, so the EDL is what counts.)
RUN ffmpeg -loglevel error -f lavfi -i testsrc2=size=320x240:rate=30000/1001 -f lavfi -i sine=frequency=440 \
      -t 20 -c:v mpeg2video -flags +ilme+ildct -c:a mp2 -f mpegts /tmp/clip.ts \
 && printf 'output_edl=1\n' > /tmp/test.ini \
 && mkdir -p /tmp/out && (/comskip/comskip --ini=/tmp/test.ini --output=/tmp/out /tmp/clip.ts || true) \
 && test -f /tmp/out/clip.edl

# --- runtime: Alpine for ffmpeg ------------------------------------------------
FROM alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6
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
    COUCHSIDE_RECORDINGS_DIR=/recordings \
    COUCHSIDE_HWACCEL=none

USER 1000:1000
EXPOSE 8080 1900/udp
VOLUME ["/data", "/cache", "/recordings"]
# For Docker and compose (Kubernetes uses the chart's probes, with the
# database-free /livez for liveness). Marks the container unhealthy when the
# database can't be reached; Docker doesn't restart it for that. The port is
# whatever COUCHSIDE_ADDR ends in.
HEALTHCHECK --interval=30s --timeout=10s --start-period=90s --retries=5 \
    CMD wget -qO /dev/null "http://127.0.0.1:${COUCHSIDE_ADDR##*:}/healthz" || exit 1
ENTRYPOINT ["couchside"]
