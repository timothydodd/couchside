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
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS server
ARG TARGETOS TARGETARCH VERSION=dev
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
COPY --from=web /web/dist ./internal/webui/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/couchside ./cmd/couchside

# --- runtime: Alpine for ffmpeg ------------------------------------------------
FROM alpine:3.22
# Intel VAAPI drivers (iHD for Broadwell and newer, i965 for older chips) so
# COUCHSIDE_HWACCEL=vaapi can use /dev/dri. x86 only.
RUN apk add --no-cache ffmpeg ca-certificates tzdata \
 && if [ "$(apk --print-arch)" = "x86_64" ]; then apk add --no-cache intel-media-driver libva-intel-driver; fi \
 && adduser -D -H -u 1000 couchside \
 && mkdir -p /data /cache /media /recordings \
 && chown couchside:couchside /data /cache /recordings
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
