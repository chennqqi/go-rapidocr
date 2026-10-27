#!/usr/bin/env bash
# Fetches the RapidAI prebuilt dependency packs used by the glibc-frozen
# Linux CLIB build (docker/Dockerfile.linux-clib-glibc) into docker-deps/.
#
#   ./scripts/fetch_static_deps.sh x64     # linux x86_64
#   ./scripts/fetch_static_deps.sh arm64   # linux aarch64
#
# Layout produced:
#   docker-deps/onnxruntime-static/   (x64 only: full static link)
#   docker-deps/onnxruntime-shared/   (arm64 only: official .so, GLIBC<=2.27)
#   docker-deps/opencv-static/        (both arches)
#
# Requires: curl, and either 7z or python3 + py7zr for .7z packs.
set -euo pipefail

ARCH="${1:-x64}"
ORT_VER=1.20.1
CV_VER=4.10.0
ORT_BASE=https://github.com/RapidAI/OnnxruntimeBuilder/releases/download
CV_BASE=https://github.com/RapidAI/OpenCVBuilder/releases/download
MS_BASE=https://github.com/microsoft/onnxruntime/releases/download
OUT=docker-deps
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

extract() {
    # extract <archive> <dest>; handles .7z and .tgz
    local arc=$1 dst=$2
    mkdir -p "$dst"
    case "$arc" in
        *.tgz|*.tar.gz) tar -xzf "$arc" -C "$dst" --strip-components=1 ;;
        *.7z)
            if command -v 7z >/dev/null; then
                7z x -y -o"$dst" "$arc" >/dev/null
            else
                python3 -c "import py7zr,sys; py7zr.SevenZipFile(sys.argv[1]).extractall(sys.argv[2])" "$arc" "$dst"
            fi
            # strip the single top-level directory the packs wrap files in
            local inner
            inner=$(find "$dst" -mindepth 1 -maxdepth 1 -type d | head -1)
            if [ -n "$inner" ] && [ "$(find "$dst" -mindepth 1 -maxdepth 1 | wc -l)" -eq 1 ]; then
                cp -a "$inner"/. "$dst"/ && rm -rf "$inner"
            fi
            ;;
    esac
}

fetch() { curl -fsSL -o "$TMP/$2" "$1"; }

case "$ARCH" in
    x64)
        fetch "$ORT_BASE/$ORT_VER/onnxruntime-v$ORT_VER-ubuntu-20.04-amd64-static.7z" ort.7z
        fetch "$CV_BASE/$CV_VER/opencv-$CV_VER-ubuntu-16.04-amd64.7z" cv.7z
        extract "$TMP/ort.7z" "$OUT/onnxruntime-static"
        extract "$TMP/cv.7z"  "$OUT/opencv-static"
        ;;
    arm64)
        # No glibc aarch64 static pack exists upstream; use the official
        # shared lib (needs only GLIBC_2.27) and static opencv.
        fetch "$MS_BASE/v$ORT_VER/onnxruntime-linux-aarch64-$ORT_VER.tgz" ort.tgz
        fetch "$CV_BASE/$CV_VER/opencv-$CV_VER-ubuntu-16.04-arm64.7z" cv.7z
        extract "$TMP/ort.tgz" "$OUT/onnxruntime-shared"
        extract "$TMP/cv.7z"  "$OUT/opencv-static"
        ;;
    *) echo "usage: $0 {x64|arm64}" >&2; exit 1 ;;
esac

find "$OUT" -name "*.a" -o -name "*.so*" -o -name "*.cmake" | sort | head -30
echo "deps ready in $OUT/"
