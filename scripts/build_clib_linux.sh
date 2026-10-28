#!/usr/bin/env bash
# Container-side build for docker/Dockerfile.linux-clib-glibc.
#
#   build_clib_linux.sh [deps-dir] [out-dir]
#
# deps-dir contents (any combination):
#   onnxruntime-static/  RapidAI static pack (OnnxRuntimeConfig.cmake at root)
#   onnxruntime-shared/  official release tree (include/ + lib/libonnxruntime.so*)
#   opencv-static/       RapidAI opencv pack (lib/cmake/opencv4 inside)
#
# Outputs in out-dir:
#   libRapidOcrOnnx.so   self-contained shared lib (purego backend; also the
#                        default cgo link target)
#   libRapidOcrOnnx.a    merged static archive = engine + onnxruntime +
#                        opencv + libstdc++ (cgo fully-static linking)
#   libonnxruntime.so*   only when the shared ORT pack was used (arm64)
set -eo pipefail

DEPS="${1:-/deps}"
OUT="${2:-/workspace/artifacts}"

# Enable the newest manylinux/RH toolset if one is installed. The enable
# scripts reference unset vars (MANPATH), so -u must come after them.
for e in /opt/rh/gcc-toolset-*/enable /opt/rh/devtoolset-*/enable; do
    [ -f "$e" ] && . "$e" && break
done
set -u

# devtoolset-* (CentOS 7 / manylinux2014) hard-codes _GLIBCXX_USE_CXX11_ABI=0
# in its c++config.h ("Ignore any pre-defined value"), which produces old-ABI
# symbols that cannot link RapidAI's new-ABI static packs. Flipping the DTS
# _GLIBCXX_USE_DUAL_ABI marker lets -D_GLIBCXX_USE_CXX11_ABI=1 through.
CXXFLAGS=""
for cfg in /opt/rh/devtoolset-*/root/usr/include/c++/*/*/bits/c++config.h; do
    if grep -q "^# define _GLIBCXX_USE_DUAL_ABI 0" "$cfg" 2>/dev/null; then
        sed -i 's/^# define _GLIBCXX_USE_DUAL_ABI 0/# define _GLIBCXX_USE_DUAL_ABI 1/' "$cfg"
        CXXFLAGS="-D_GLIBCXX_USE_CXX11_ABI=1"
        echo "patched devtoolset dual-ABI: $cfg"
    fi
done

ARGS=()
[ -d "$DEPS/onnxruntime-static" ] && ARGS+=(-DOnnxRuntime_DIR="$DEPS/onnxruntime-static")
[ -d "$DEPS/opencv-static" ] && ARGS+=(-DOpenCV_DIR="$DEPS/opencv-static/lib/cmake/opencv4")
if [ -d "$DEPS/onnxruntime-shared" ]; then
    # Shared mode: CMakeLists globs deps/onnxruntime-linux-*.
    mkdir -p deps
    ln -sfn "$DEPS/onnxruntime-shared" deps/onnxruntime-linux-shared
fi

cmake -B build-clib -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIB=ON \
    -DCMAKE_CXX_FLAGS="$CXXFLAGS" "${ARGS[@]}"
cmake --build build-clib -j"$(nproc)"

mkdir -p "$OUT"
cp build-clib/libRapidOcrOnnx.so "$OUT"/
[ -d "$DEPS/onnxruntime-shared" ] && cp -d "$DEPS"/onnxruntime-shared/lib/libonnxruntime.so* "$OUT"/

# Static archive for cgo: engine objects merged with every dependency .a
# (onnxruntime, opencv modules + 3rdparty codecs, libstdc++/libgcc) so users
# only need -l:libRapidOcrOnnx.a plus libc-family system libs.
if [ -d "$DEPS/onnxruntime-static" ]; then
    cmake -B build-static -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIB=OFF \
        -DCMAKE_CXX_FLAGS="$CXXFLAGS" "${ARGS[@]}"
    cmake --build build-static -j"$(nproc)"
    # ar MRI scripts cannot parse '+' in filenames (libstdc++.a), so
    # stage the runtime archives under safe names first.
    mkdir -p /tmp/rt
    cp "$(gcc -print-file-name=libstdc++.a)" /tmp/rt/libstdcxx.a
    cp "$(gcc -print-file-name=libgcc.a)" /tmp/rt/libgccx.a
    {
        echo "create $OUT/libRapidOcrOnnx.a"
        echo "addlib $(pwd)/build-static/librapidocr.a"
        find "$DEPS" -name '*.a' | sort | while read -r a; do echo "addlib $a"; done
        echo "addlib /tmp/rt/libstdcxx.a"
        echo "addlib /tmp/rt/libgccx.a"
        echo "save"
        echo "end"
    } | ar -M
fi

echo "=== NEEDED ==="
readelf -d "$OUT/libRapidOcrOnnx.so" | grep NEEDED || true
echo "=== max GLIBC ==="
readelf -V "$OUT/libRapidOcrOnnx.so" | grep -oE 'GLIBC_[0-9.]+' | sort -uV | tail -1
ls -l "$OUT"
