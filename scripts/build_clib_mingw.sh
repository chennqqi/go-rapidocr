#!/usr/bin/env bash
# Builds libRapidOcrOnnx.dll (full OcrDetectInput API) with MSYS2 MinGW-w64.
#
# Prerequisites (one-time):
#   pacman -S mingw-w64-ucrt-x86_64-toolchain mingw-w64-ucrt-x86_64-cmake \
#             mingw-w64-ucrt-x86_64-opencv
#   unzip official onnxruntime-win-x64-<ver>.zip into deps/   (see below)
#
# Output: build-clib/libRapidOcrOnnx.dll (+ onnxruntime.dll copied beside it)
set -euo pipefail
cd "$(dirname "$0")/.."

MSYS2_UCRT64=${MSYS2_UCRT64:-/c/msys64/ucrt64}
export PATH="$MSYS2_UCRT64/bin:$PATH"

ORT_DIR=$(ls -d deps/onnxruntime-win-x64-* 2>/dev/null | head -1)
if [ -z "$ORT_DIR" ]; then
    echo "onnxruntime not found. Download:" >&2
    echo "  curl -L -o deps/ort-win.zip https://github.com/microsoft/onnxruntime/releases/download/v1.20.1/onnxruntime-win-x64-1.20.1.zip" >&2
    echo "  unzip deps/ort-win.zip -d deps/" >&2
    exit 1
fi

# Upstream headers include <onnxruntime/core/session/onnxruntime_cxx_api.h>;
# remap the flat release headers into that layout.
mkdir -p deps/ort-compat/onnxruntime/core/session
cp "$ORT_DIR"/include/*.h deps/ort-compat/onnxruntime/core/session/

cmake -B build-clib -G "MinGW Makefiles" \
    -DCMAKE_BUILD_TYPE=Release \
    -DBUILD_SHARED_LIB=ON \
    -DOpenCV_DIR="$MSYS2_UCRT64/lib/cmake/opencv5"
cmake --build build-clib -j"$(nproc)"

# Co-locate onnxruntime.dll so the loader resolves it next to the engine dll.
cp "$ORT_DIR"/lib/onnxruntime*.dll build-clib/
echo "Built: build-clib/libRapidOcrOnnx.dll"
