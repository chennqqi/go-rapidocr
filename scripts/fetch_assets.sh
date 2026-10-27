#!/bin/bash
# Fetches the shared libraries and models needed to run go-rapidocr.
set -e
cd "$(dirname "$0")/.."

CLIB_VERSION=1.2.3
MODEL_SCOPE=https://www.modelscope.cn/models/RapidAI/RapidOCR/resolve/master

# --- shared library for the purego backend ---
case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*) asset=windows-clib-vs2022-mt.7z; dst=lib/windows-x64 ;;
  Linux*)                asset=linux-clib.7z;            dst=lib/linux-x64 ;;
  Darwin*)               asset=macos-clib.7z;            dst=lib/darwin-x64 ;;
  *) echo "unsupported OS: $(uname -s)"; exit 1 ;;
esac

mkdir -p "$dst" models
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

echo "Done. lib: $(ls $dst)  models: $(ls models)"
