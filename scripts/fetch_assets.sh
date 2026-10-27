#!/bin/bash
# Fetches the shared libraries and models needed to run go-rapidocr.
#
# Usage:
#   ./scripts/fetch_assets.sh [TAG|--upstream]
#
# Default: download lib-<os>-<arch>.tar.gz from this repo's GitHub release
# (latest release, or the given TAG). --upstream falls back to the
# RapidAI/RapidOcrOnnx 1.2.3 prebuilt CLIB (legacy text-only API).
set -e
cd "$(dirname "$0")/.."

REPO=chennqqi/go-rapidocr
CLIB_VERSION=1.2.3
MODEL_SCOPE=https://www.modelscope.cn/models/RapidAI/RapidOCR/resolve/master

# --- platform name matching workflow artifact names ---
case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*) os=windows ;;
  Linux*)               os=linux   ;;
  Darwin*)              os=darwin  ;;
  *) echo "unsupported OS: $(uname -s)"; exit 1 ;;
esac
arch=x64
case "$(uname -m)" in aarch64|arm64) arch=arm64 ;; esac
plat=$os-$arch
dst=lib/$plat

mkdir -p "$dst" models

TAG="${1:-latest}"
if [ "$TAG" != "--upstream" ]; then
  # --- this repo's release (full structured API, safe wrappers) ---
  if [ "$TAG" = "latest" ]; then
    url="https://github.com/$REPO/releases/latest/download/lib-$plat.tar.gz"
  else
    url="https://github.com/$REPO/releases/download/$TAG/lib-$plat.tar.gz"
  fi
  echo "fetching $url"
  if curl -fsSL -o "/tmp/lib-$plat.tar.gz" "$url"; then
    tar -xzf "/tmp/lib-$plat.tar.gz" -C "$dst"
  else
    echo "release asset unavailable, falling back to upstream CLIB" >&2
    TAG=--upstream
  fi
fi

if [ "$TAG" = "--upstream" ]; then
  # --- upstream RapidAI prebuilt CLIB (legacy OcrDetect API only) ---
  case "$os" in
    windows) asset=windows-clib-vs2022-mt.7z ;;
    linux)   asset=linux-clib.7z ;;
    darwin)  asset=macos-clib.7z ;;
  esac
  if [ ! -f /tmp/$asset ]; then
    curl -L -o /tmp/$asset "https://github.com/RapidAI/RapidOcrOnnx/releases/download/$CLIB_VERSION/$asset"
  fi
  python - "$asset" "$dst" <<'EOF'
import py7zr, sys, os
asset, dst = sys.argv[1], sys.argv[2]
with py7zr.SevenZipFile('/tmp/' + asset) as z:
    names = [n for n in z.getnames() if n.endswith(('.dll', '.so', '.dylib'))]
    z.extract(path=dst, targets=names)
# flatten: move extracted libs up into dst
for root, _, files in os.walk(dst):
    for f in files:
        if f.endswith(('.dll', '.so', '.dylib')):
            os.replace(os.path.join(root, f), os.path.join(dst, f))
EOF
  find "$dst" -type d -empty -delete 2>/dev/null || true
fi

# --- models (PP-OCRv4 mobile) ---
for m in \
  "onnx/PP-OCRv4/det/ch_PP-OCRv4_det_mobile.onnx" \
  "onnx/PP-OCRv4/cls/ch_ppocr_mobile_v2.0_cls_mobile.onnx" \
  "onnx/PP-OCRv4/rec/ch_PP-OCRv4_rec_mobile.onnx"; do
  f=models/$(basename "$m")
  [ -f "$f" ] || curl -L -o "$f" "$MODEL_SCOPE/$m"
done
cp -n rapidocr-onnx/models/ppocr_keys_v1.txt models/ 2>/dev/null || \
  curl -L -o models/ppocr_keys_v1.txt "$MODEL_SCOPE/others/ppocr_keys_v1.txt" || true

# ppocr_keys_v1.txt may ship with CRLF endings. Windows text-mode reads
# strip \r but POSIX builds keep it, which leaks a '\r' into every
# recognized character's label. Normalize to LF.
[ -f models/ppocr_keys_v1.txt ] && sed -i 's/\r$//' models/ppocr_keys_v1.txt

echo "Done. lib/$plat: $(ls $dst)  models: $(ls models)"
