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
# The result is libRapidOcrOnnx.so in out-dir (plus libonnxruntime.so* when
# the shared pack was used). C++ runtime is always linked statically.
set -euo pipefail

DEPS="${1:-/deps}"
OUT="${2:-/workspace/artifacts}"

# Enable the newest manylinux/RH toolset if one is installed.
for e in /opt/rh/gcc-toolset-*/enable /opt/rh/devtoolset-*/enable; do
    [ -f "$e" ] && . "$e" && break
done

ARGS=()
[ -d "$DEPS/onnxruntime-static" ] && ARGS+=(-DOnnxRuntime_DIR="$DEPS/onnxruntime-static")
[ -d "$DEPS/opencv-static" ] && ARGS+=(-DOpenCV_DIR="$DEPS/opencv-static/lib/cmake/opencv4")
if [ -d "$DEPS/onnxruntime-shared" ]; then
    # Shared mode: CMakeLists globs deps/onnxruntime-linux-*.
    mkdir -p deps
    ln -sfn "$DEPS/onnxruntime-shared" deps/onnxruntime-linux-shared
fi

cmake -B build-clib -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIB=ON "${ARGS[@]}"
cmake --build build-clib -j"$(nproc)"

mkdir -p "$OUT"
cp build-clib/libRapidOcrOnnx.so "$OUT"/
[ -d "$DEPS/onnxruntime-shared" ] && cp -d "$DEPS"/onnxruntime-shared/lib/libonnxruntime.so* "$OUT"/

echo "=== NEEDED ==="
readelf -d "$OUT/libRapidOcrOnnx.so" | grep NEEDED || true
echo "=== max GLIBC ==="
readelf -V "$OUT/libRapidOcrOnnx.so" | grep -oE 'GLIBC_[0-9.]+' | sort -uV | tail -1
ls -l "$OUT"
