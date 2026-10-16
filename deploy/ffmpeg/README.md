# The Windows ffmpeg

The Windows installer ships `ffmpeg.exe` and `ffprobe.exe` built here: upstream
FFmpeg, unmodified, static, cross-compiled for x86-64 Windows on Linux with
mingw-w64, with only the libraries Couchside uses. `.github/workflows/ffmpeg.yml`
runs `build.sh`, checks the result on a Windows runner and publishes it as a
GitHub release named `ffmpeg-<version>-<build>`; `windows-installer.yml`
downloads that release by pinned SHA-256.

## What's in it, and why

| Library | For |
| --- | --- |
| x264 | `libx264`, CPU encoding |
| nv-codec-headers 12.1 | NVENC, NVDEC, `scale_cuda`, `yadif_cuda` (compiled to PTX by clang). The header version sets the oldest NVIDIA driver NVENC runs on: 12.1 is 531.61, 13.1 is 610. 12.1 is the oldest FFmpeg 8.1 takes and has everything we use. |
| libvpl | Intel Quick Sync (`h264_qsv`, `-hwaccel qsv`) |
| OpenCL headers + ICD loader | `tonemap_opencl`: HDR tone mapping on NVIDIA, where stock FFmpeg has no CUDA tone mapper |
| zimg | `zscale`, HDR tone mapping on the CPU |
| libwebp | posters and stills |
| zlib | PNG covers in Matroska attachments, compressed Matroska headers |
| schannel (Windows) | https live-stream sources |

Not in it, on purpose: libass and fonts (text subtitles are converted to WebVTT,
and only picture subtitles are burned in), AV1/VP9/HEVC encoders, AMF (the
server only picks NVENC or Quick Sync on Windows), libmp3lame, opus, dav1d,
iconv, xml2, bzip2 and lzma. Add one by appending to `build.sh` and
`versions.env`, and extend the checks in `ffmpeg.yml`.

The result is GPL v3 (`--enable-gpl --enable-version3`, because of the Apache
2.0 OpenCL loader), the same licence the installer already carried. The
`-src.tar.gz` on each release is the complete corresponding source: every
archive as fetched plus `build.sh` and `versions.env`.

## Shipping a new build

1. Edit `versions.env`: bump `FFMPEG_VERSION` or a dependency, with the new
   SHA-256 of each changed archive (`curl -sSfL <url> | sha256sum`), and
   `BUILD` when the FFmpeg version stays the same.
2. Push, then tag: `git tag ffmpeg-<FFMPEG_VERSION>-<BUILD> && git push origin ffmpeg-<FFMPEG_VERSION>-<BUILD>`.
   The workflow refuses a tag that doesn't match `versions.env`.
3. From the new release's `SHA256SUMS.txt`, set `FFMPEG_VERSION`,
   `FFMPEG_SHA256` (the zip) and `FFMPEG_SRC_SHA256` (the tarball) in
   `.github/workflows/windows-installer.yml`.

A manual run of the workflow (Actions → ffmpeg → Run workflow) builds and
checks without publishing; the zip is a workflow artifact.

## Building locally

Debian/Ubuntu (WSL is fine; about 15 minutes on 8 cores):

```sh
sudo apt-get install mingw-w64 clang llvm nasm cmake ninja-build pkg-config autoconf automake libtool make zip curl xz-utils
sudo update-alternatives --set x86_64-w64-mingw32-gcc /usr/bin/x86_64-w64-mingw32-gcc-posix
sudo update-alternatives --set x86_64-w64-mingw32-g++ /usr/bin/x86_64-w64-mingw32-g++-posix
deploy/ffmpeg/build.sh            # works in .tmp/ffmpeg; output in .tmp/ffmpeg/out
```
