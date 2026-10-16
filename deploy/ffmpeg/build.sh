#!/usr/bin/env bash
# Builds the ffmpeg.exe and ffprobe.exe that the Windows installer ships:
# upstream FFmpeg, static, cross-compiled for x86-64 Windows on Linux with
# mingw-w64, with only the libraries Couchside uses (deploy/ffmpeg/README.md
# says which and why). Run by .github/workflows/ffmpeg.yml; works the same in
# WSL with the packages listed there.
#
#   deploy/ffmpeg/build.sh [work dir]      (default: .tmp/ffmpeg in the repo)
#
# Output, in <work dir>/out:
#   couchside-ffmpeg-<version>-<build>-win64.zip   ffmpeg.exe, ffprobe.exe, FFMPEG.txt
#   couchside-ffmpeg-<version>-<build>-src.tar.gz  every source archive plus this script
#   SHA256SUMS.txt
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=versions.env
. "$here/versions.env"

work=${1:-$(cd "$here/../.." && pwd)/.tmp/ffmpeg}
host=x86_64-w64-mingw32
prefix=$work/prefix
src=$work/src
build=$work/build
out=$work/out
tag=$FFMPEG_VERSION-$BUILD
jobs=$(nproc)

for tool in $host-gcc $host-g++ clang nasm cmake ninja pkg-config autoreconf libtoolize make tar curl zip; do
  command -v "$tool" >/dev/null || { echo "missing $tool" >&2; exit 1; }
done
# scale_cuda and yadif_cuda are compiled to PTX by clang, which must know the
# NVPTX target.
clang -print-targets | grep -q nvptx64 || { echo "clang has no nvptx64 target" >&2; exit 1; }

rm -rf "$prefix" "$build" "$out"
mkdir -p "$prefix/lib/pkgconfig" "$src" "$build" "$out"

export PKG_CONFIG_LIBDIR=$prefix/lib/pkgconfig
export PKG_CONFIG_PATH=$PKG_CONFIG_LIBDIR
export CC=$host-gcc CXX=$host-g++ AR=$host-ar RANLIB=$host-ranlib STRIP=$host-strip NM=$host-nm WINDRES=$host-windres
export CFLAGS="-O2 -pipe -I$prefix/include"
export CXXFLAGS=$CFLAGS
export LDFLAGS="-L$prefix/lib -static -static-libgcc -static-libstdc++"

# fetch <file> <url> <sha256>: downloads into $src once and checks the hash.
fetch() {
  local file=$src/$1 url=$2 sum=$3
  [ -f "$file" ] || curl -sSfL --retry 3 -o "$file" "$url"
  echo "$sum  $file" | sha256sum -c --quiet - || { echo "$1 doesn't match its pinned SHA-256" >&2; exit 1; }
}

# unpack <file> <dir>: extracts into a fresh $build/<dir> (strips the top folder).
unpack() {
  rm -rf "$build/$2" && mkdir -p "$build/$2"
  tar -xf "$src/$1" -C "$build/$2" --strip-components=1
}

# A CMake toolchain for the cross build; every CMake project below uses it.
cat >"$build/toolchain.cmake" <<EOF
set(CMAKE_SYSTEM_NAME Windows)
set(CMAKE_SYSTEM_PROCESSOR AMD64)
set(CMAKE_C_COMPILER $host-gcc)
set(CMAKE_CXX_COMPILER $host-g++)
set(CMAKE_RC_COMPILER $host-windres)
set(CMAKE_FIND_ROOT_PATH $prefix)
set(CMAKE_FIND_ROOT_PATH_MODE_PROGRAM NEVER)
set(CMAKE_FIND_ROOT_PATH_MODE_LIBRARY ONLY)
set(CMAKE_FIND_ROOT_PATH_MODE_INCLUDE ONLY)
set(CMAKE_FIND_ROOT_PATH_MODE_PACKAGE ONLY)
EOF
cmake_build() { # <dir> <cmake options...>
  local dir=$1; shift
  cmake -S "$build/$dir" -B "$build/$dir/.build" -G Ninja -Wno-dev \
    -DCMAKE_TOOLCHAIN_FILE="$build/toolchain.cmake" -DCMAKE_BUILD_TYPE=Release \
    -DCMAKE_INSTALL_PREFIX="$prefix" -DCMAKE_INSTALL_LIBDIR=lib -DBUILD_SHARED_LIBS=OFF "$@"
  cmake --build "$build/$dir/.build" -j "$jobs"
  cmake --install "$build/$dir/.build"
}

echo "== sources"
fetch "ffmpeg-$FFMPEG_VERSION.tar.xz" "https://ffmpeg.org/releases/ffmpeg-$FFMPEG_VERSION.tar.xz" "$FFMPEG_SHA256"
fetch "x264-$X264_COMMIT.tar.bz2" "https://code.videolan.org/videolan/x264/-/archive/$X264_COMMIT/x264-$X264_COMMIT.tar.bz2" "$X264_SHA256"
fetch "zimg-$ZIMG_VERSION.tar.gz" "https://github.com/sekrit-twc/zimg/archive/refs/tags/release-$ZIMG_VERSION.tar.gz" "$ZIMG_SHA256"
fetch "libwebp-$LIBWEBP_VERSION.tar.gz" "https://storage.googleapis.com/downloads.webmproject.org/releases/webp/libwebp-$LIBWEBP_VERSION.tar.gz" "$LIBWEBP_SHA256"
fetch "libvpl-$LIBVPL_VERSION.tar.gz" "https://github.com/intel/libvpl/archive/refs/tags/v$LIBVPL_VERSION.tar.gz" "$LIBVPL_SHA256"
fetch "nv-codec-headers-$NVCODEC_VERSION.tar.gz" "https://github.com/FFmpeg/nv-codec-headers/archive/refs/tags/n$NVCODEC_VERSION.tar.gz" "$NVCODEC_SHA256"
fetch "OpenCL-Headers-$OPENCL_VERSION.tar.gz" "https://github.com/KhronosGroup/OpenCL-Headers/archive/refs/tags/v$OPENCL_VERSION.tar.gz" "$OPENCL_HEADERS_SHA256"
fetch "OpenCL-ICD-Loader-$OPENCL_VERSION.tar.gz" "https://github.com/KhronosGroup/OpenCL-ICD-Loader/archive/refs/tags/v$OPENCL_VERSION.tar.gz" "$OPENCL_LOADER_SHA256"
fetch "zlib-$ZLIB_VERSION.tar.gz" "https://github.com/madler/zlib/releases/download/v$ZLIB_VERSION/zlib-$ZLIB_VERSION.tar.gz" "$ZLIB_SHA256"

echo "== zlib"
unpack "zlib-$ZLIB_VERSION.tar.gz" zlib
(cd "$build/zlib" && CHOST=$host ./configure --static --prefix="$prefix" && make -j "$jobs" install)

echo "== x264"
unpack "x264-$X264_COMMIT.tar.bz2" x264
(cd "$build/x264" && ./configure --host=$host --cross-prefix=$host- --prefix="$prefix" \
  --enable-static --enable-pic --disable-cli --disable-lavf --disable-swscale --disable-avs \
  --disable-ffms --disable-gpac --disable-lsmash --bit-depth=all \
  && make -j "$jobs" install)

echo "== zimg"
unpack "zimg-$ZIMG_VERSION.tar.gz" zimg
(cd "$build/zimg" && ./autogen.sh && ./configure --host=$host --prefix="$prefix" \
  --enable-static --disable-shared && make -j "$jobs" install)

echo "== libwebp"
unpack "libwebp-$LIBWEBP_VERSION.tar.gz" libwebp
(cd "$build/libwebp" && ./configure --host=$host --prefix="$prefix" --enable-static --disable-shared \
  --disable-libwebpmux --disable-libwebpdemux --disable-libwebpdecoder --disable-libwebpextras \
  --disable-png --disable-jpeg --disable-tiff --disable-gif --disable-wic --disable-sdl \
  && make -j "$jobs" install)

echo "== libvpl"
unpack "libvpl-$LIBVPL_VERSION.tar.gz" libvpl
cmake_build libvpl -DBUILD_TOOLS=OFF -DBUILD_EXAMPLES=OFF -DBUILD_TESTS=OFF -DINSTALL_EXAMPLE_CODE=OFF \
  -DBUILD_EXPERIMENTAL=OFF -DBUILD_DISPATCHER_ONLY=ON -DINSTALL_DEV=ON -DINSTALL_LIB=ON
# Static libvpl is C++ and uses COM; pkg-config has to say so for ffmpeg's link.
sed -i 's/^Libs.private:.*/Libs.private: -lole32 -lgdi32 -luuid -lstdc++/' "$prefix/lib/pkgconfig/vpl.pc"

echo "== nv-codec-headers"
unpack "nv-codec-headers-$NVCODEC_VERSION.tar.gz" nv-codec-headers
make -C "$build/nv-codec-headers" install PREFIX="$prefix"

echo "== OpenCL"
unpack "OpenCL-Headers-$OPENCL_VERSION.tar.gz" opencl-headers
cmake_build opencl-headers -DBUILD_TESTING=OFF
unpack "OpenCL-ICD-Loader-$OPENCL_VERSION.tar.gz" opencl-loader
cmake_build opencl-loader -DOPENCL_ICD_LOADER_HEADERS_DIR="$prefix/include" \
  -DOPENCL_ICD_LOADER_BUILD_SHARED_LIBS=OFF -DOPENCL_ICD_LOADER_BUILD_TESTING=OFF -DBUILD_TESTING=OFF \
  -DENABLE_OPENCL_LAYERINFO=OFF
# mingw names the static library OpenCL.a; -lOpenCL looks for libOpenCL.a.
[ -f "$prefix/lib/libOpenCL.a" ] || mv "$prefix/lib/OpenCL.a" "$prefix/lib/libOpenCL.a"
# The loader finds vendor drivers through the registry and the display
# driver: it needs these system libraries.
cat >"$prefix/lib/pkgconfig/OpenCL.pc" <<EOF
prefix=$prefix
libdir=\${prefix}/lib
includedir=\${prefix}/include
Name: OpenCL
Description: Khronos OpenCL ICD loader
Version: $OPENCL_VERSION
Libs: -L\${libdir} -lOpenCL
Libs.private: -lcfgmgr32 -lruntimeobject -lole32 -lgdi32 -ladvapi32
Cflags: -I\${includedir}
EOF

echo "== ffmpeg"
unpack "ffmpeg-$FFMPEG_VERSION.tar.xz" ffmpeg
(cd "$build/ffmpeg" && ./configure \
  --prefix="$prefix" --arch=x86_64 --target-os=mingw32 --cross-prefix=$host- \
  --pkg-config=pkg-config --pkg-config-flags=--static \
  --extra-cflags="-I$prefix/include" --extra-ldflags="-L$prefix/lib -static -static-libgcc -static-libstdc++" \
  --extra-version="couchside-$BUILD" \
  --enable-gpl --enable-version3 --enable-static --disable-shared \
  --disable-debug --disable-doc --disable-ffplay \
  --enable-zlib --enable-schannel \
  --enable-libx264 --enable-libzimg --enable-libwebp --enable-libvpl \
  --enable-ffnvcodec --enable-nvenc --enable-nvdec --enable-cuvid --enable-cuda-llvm \
  --enable-opencl --enable-d3d11va --enable-dxva2 \
  && make -j "$jobs" && make install)

echo "== package"
stage=$build/stage
rm -rf "$stage" && mkdir -p "$stage"
cp "$prefix/bin/ffmpeg.exe" "$prefix/bin/ffprobe.exe" "$stage/"
$STRIP "$stage"/*.exe
configure_line=$(grep -m1 "^FFMPEG_CONFIGURATION=" "$build/ffmpeg/ffbuild/config.mak" | cut -d= -f2-)
cat >"$stage/FFMPEG.txt" <<EOF
ffmpeg.exe and ffprobe.exe are FFmpeg $FFMPEG_VERSION, built by Couchside (build $BUILD) from
unmodified upstream sources, static, for x86-64 Windows. They are licensed under the GNU
General Public License version 3 (GPL-3.0.txt beside them).

Their complete source (FFmpeg and every library linked in, with the build script) is
couchside-ffmpeg-$tag-src.tar.gz on the ffmpeg-$tag release at
https://github.com/timothydodd/couchside/releases, and is also attached to every Couchside
release whose installer ships this build.

Libraries: x264 (GPL v2+), zimg (WTFPL), libwebp (BSD-3-Clause), libvpl (MIT),
nv-codec-headers (MIT), OpenCL headers and ICD loader (Apache-2.0), zlib (zlib).

configure: $configure_line
EOF
(cd "$stage" && zip -q -9 "$out/couchside-ffmpeg-$tag-win64.zip" ffmpeg.exe ffprobe.exe FFMPEG.txt)

# The GPL's complete corresponding source: every archive as fetched, the pins
# and this script, so anyone can rebuild the same exe.
srcdir=$build/couchside-ffmpeg-$tag-src
rm -rf "$srcdir" && mkdir -p "$srcdir"
cp "$src"/* "$srcdir/"
cp "$here/build.sh" "$here/versions.env" "$here/README.md" "$srcdir/"
tar -czf "$out/couchside-ffmpeg-$tag-src.tar.gz" -C "$build" "couchside-ffmpeg-$tag-src"
(cd "$out" && sha256sum ./*.zip ./*.tar.gz > SHA256SUMS.txt && cat SHA256SUMS.txt)
ls -l "$stage"
