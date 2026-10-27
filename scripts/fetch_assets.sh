#!/bin/bash
# Fetches the shared libraries and models needed to run go-rapidocr.
#
# Usage:
#   ./scripts/fetch_assets.sh [TAG|--upstream] [--models TIER]
#
# Library: downloads lib-<os>-<arch>.tar.gz from this repo's GitHub release
# (latest release, or TAG). --upstream falls back to the RapidAI
# RapidOcrOnnx 1.2.3 prebuilt CLIB (legacy text-only API).
#
# Models: TIER selects which PP-OCR variant to download into models/:
#   mobile      PP-OCRv4 mobile det/cls/rec (~16MB, default — fast, ~95% acc)
#   server      PP-OCRv4 server det/rec (~200MB — slow, best accuracy)
#   v5-mobile   PP-OCRv5 mobile (~18MB — newer, needs ppocrv5_dict.txt)
#   v5-server   PP-OCRv5 server (~96MB — newest + accurate + slow)
# The recognition dict MUST match the rec model; each tier downloads its
# matching keys file. See README "Model selection".
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

TAG=latest
TIER=mobile
for arg in "$@"; do
  case "$arg" in
    --upstream) TAG=--upstream ;;
    --models=*) TIER="${arg#*=}" ;;
    --models) echo "--models needs =TIER (mobile|server|v5-mobile|v5-server)" >&2; exit 2 ;;
    *) TAG="$arg" ;;
  esac
done

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

# --- models ---
# The rec dict lives under the paddle tree on ModelScope and MUST match
# the rec model version/language.
case "$TIER" in
  mobile)
    files="onnx/PP-OCRv4/det/ch_PP-OCRv4_det_mobile.onnx
onnx/PP-OCRv4/cls/ch_ppocr_mobile_v2.0_cls_mobile.onnx
onnx/PP-OCRv4/rec/ch_PP-OCRv4_rec_mobile.onnx"
    dict="paddle/PP-OCRv4/rec/ch_PP-OCRv4_rec_mobile/ppocr_keys_v1.txt"
    ;;
  server)
    files="onnx/PP-OCRv4/det/ch_PP-OCRv4_det_server.onnx
onnx/PP-OCRv4/cls/ch_ppocr_mobile_v2.0_cls_mobile.onnx
onnx/PP-OCRv4/rec/ch_PP-OCRv4_rec_server.onnx"
    # v4 server rec shares the v1 dict published under the mobile path.
    dict="paddle/PP-OCRv4/rec/ch_PP-OCRv4_rec_mobile/ppocr_keys_v1.txt"
    ;;
  v5-mobile)
    files="onnx/PP-OCRv5/det/ch_PP-OCRv5_det_mobile.onnx
onnx/PP-OCRv5/cls/ch_PP-LCNet_x0_25_textline_ori_cls_mobile.onnx
onnx/PP-OCRv5/rec/ch_PP-OCRv5_rec_mobile.onnx"
    dict="paddle/PP-OCRv5/rec/ch_PP-OCRv5_rec_mobile/ppocrv5_dict.txt"
    ;;
  v5-server)
    files="onnx/PP-OCRv5/det/ch_PP-OCRv5_det_server.onnx
onnx/PP-OCRv5/cls/ch_PP-LCNet_x1_0_textline_ori_cls_server.onnx
onnx/PP-OCRv5/rec/ch_PP-OCRv5_rec_server.onnx"
    dict="paddle/PP-OCRv5/rec/ch_PP-OCRv5_rec_server/ppocrv5_dict.txt"
    ;;
  *) echo "unknown --models tier: $TIER (mobile|server|v5-mobile|v5-server)" >&2; exit 2 ;;
esac

for m in $files; do
  f=models/$(basename "$m")
  [ -f "$f" ] || curl -fsSL -o "$f" "$MODEL_SCOPE/$m"
done
cp -n rapidocr-onnx/models/$(basename "$dict") models/ 2>/dev/null || \
  curl -fsSL -o "models/$(basename "$dict")" "$MODEL_SCOPE/$dict" || true

# Dict files may ship with CRLF endings. Windows text-mode reads strip
# \r but POSIX builds keep it, which leaks a '\r' into every recognized
# character's label. Normalize all dict files to LF.
for f in models/*keys*.txt models/*dict*.txt; do
  [ -f "$f" ] && sed -i 's/\r$//' "$f"
done

echo "Done. lib/$plat: $(ls $dst)  models[$TIER]: $(ls models)"
