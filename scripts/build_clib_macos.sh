#!/usr/bin/env bash
# Builds libRapidOcrOnnx.dylib (full OcrDetectInput API) on macOS.
#
# Prerequisites: cmake, opencv (brew install cmake opencv), and an official
# onnxruntime-osx-<arch>-<ver> release unpacked into deps/.
set -euo pipefail
cd "$(dirname "$0")/.."

ARCH=$(uname -m) # arm64 or x86_64
ORT_DIR=$(ls -d deps/onnxruntime-osx-* 2>/dev/null | head -1)
if [ -z "$ORT_DIR" ]; then
    echo "onnxruntime not found. Download:" >&2
    echo "  curl -L https://github.com/microsoft/onnxruntime/releases/download/v1.20.1/onnxruntime-osx-${ARCH}-1.20.1.tgz | tar -xz -C deps/" >&2
    exit 1
fi

mkdir -p deps/ort-compat/onnxruntime/core/session
cp "$ORT_DIR"/include/*.h deps/ort-compat/onnxruntime/core/session/

cmake -B build-clib-macos -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIB=ON
cmake --build build-clib-macos -j"$(sysctl -n hw.ncpu)"

# Co-locate the onnxruntime dylib; @loader_path rpath resolves it.
cp "$ORT_DIR"/lib/libonnxruntime*.dylib build-clib-macos/ 2>/dev/null || true
echo "Built: build-clib-macos/libRapidOcrOnnx.dylib"
