#!/usr/bin/env bash
# Three-way OCR comparison: upstream C++ binary, Python rapidocr_onnxruntime,
# and go-rapidocr (purego + cgo backends). Same image, models, parameters.
#
#   ./bench/bench.sh [image] [loops]
#
# Windows prerequisites:
#   - build-bins/rapidocr-bench.exe  (cmake -DBUILD_BINS=ON, MinGW)
#   - lib/windows-x64/libRapidOcrOnnx.dll + dependency dlls
#   - msys2 ucrt64 on PATH for the C++ binaries' runtime deps
#   - python + rapidocr_onnxruntime
set -euo pipefail
cd "$(dirname "$0")/.."

IMG="${1:-testdata/1.jpg}"
LOOPS="${2:-10}"
MODELS="ch_PP-OCRv4_det_mobile.onnx ch_ppocr_mobile_v2.0_cls_mobile.onnx ch_PP-OCRv4_rec_mobile.onnx"
set -- $MODELS; DET=$1; CLS=$2; REC=$3

# Resolve the Windows python (with rapidocr_onnxruntime installed) BEFORE
# prepending the MSYS2 dirs — ucrt64's own python shadows it otherwise.
PYBIN="$(command -v python || true)"
[ -z "$PYBIN" ] && PYBIN=py
export PATH="/c/msys64/ucrt64/bin:$PWD/build-bins:$PWD/lib/windows-x64:$PATH"

echo "### 1/4 upstream C++ benchmark (--loopCount $LOOPS)"
./build-bins/rapidocr-bench.exe -d models -1 "$DET" -2 "$CLS" -3 "$REC" \
    -4 ppocr_keys_v1.txt -i "$IMG" -u 2.0 --loopCount "$LOOPS" 2>&1 | tail -4

echo "### 2/4 go-rapidocr purego backend (-n $LOOPS)"
go build -o /tmp/rapidocr-cli-pg ./cmd/rapidocr-cli
/tmp/rapidocr-cli-pg -backend purego -lib lib/windows-x64/libRapidOcrOnnx.dll \
    -det "$DET" -cls "$CLS" -rec "$REC" -n "$LOOPS" "$IMG" 2>&1 | tail -3

echo "### 3/4 go-rapidocr cgo backend (-n $LOOPS)"
CGO_ENABLED=1 go build -o /tmp/rapidocr-cli-cgo ./cmd/rapidocr-cli
/tmp/rapidocr-cli-cgo -backend cgo \
    -det "$DET" -cls "$CLS" -rec "$REC" -n "$LOOPS" "$IMG" 2>&1 | tail -3

echo "### 4/4 python rapidocr_onnxruntime ($LOOPS loops)"
PYTHONIOENCODING=utf-8 "$PYBIN" bench/bench.py "$IMG" "$LOOPS" 2>&1 | tail -3
