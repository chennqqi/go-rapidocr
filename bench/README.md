# Benchmarks

Three-way comparison on `testdata/1.jpg` (600x560, mixed CJK/English),
identical models (`ch_PP-OCRv4_*_mobile` + `ppocr_keys_v1.txt`) and
parameters (`padding=50, maxSideLen=1024, unClipRatio=2.0, threads=4,
doAngle=1`), Windows 11, Ryzen-class CPU, 10 loops after warmup.

## Results

| | C++ (upstream bench) | Go purego | Go cgo | Python (rapidocr_onnxruntime) |
|---|---|---|---|---|
| Engine init | n/a | ~330 ms | ~396 ms | ~620 ms |
| Detection | 117 ms | 128 ms | 131 ms | 334–451 ms |
| Classification | 29 ms | 33 ms | 35 ms | 12–24 ms |
| Recognition | 621 ms | 683 ms | 728 ms | 1727–2092 ms |
| **Full / wall** | **776 ms** | **858 ms** | **908 ms** | **2132–2679 ms** |
| Text blocks | 12 | 12 | 12 | 11 |

- **Correctness on `testdata/1.jpg`** (ground truth = the 12 lines
  visible in the rotated image; the test image is intentionally
  upside-down to exercise the angle classifier): C++ and Go produce
  identical, faithful output — all 12 lines with punctuation preserved.
  Python finds only 11 lines (misses the last, lowest-scoring one),
  strips all spaces from English text and drops some punctuation — a
  known postprocessing difference of the Python port.
- **Correctness on `ch_en_num.jpg`** (upstream `python/tests` asset,
  their suite expects 18 blocks with their bundled models): both find
  15. Go/C++ detects the main title 正品促销 and 券后价￥ correctly
  but misreads 破损就赔→破损京焙 and the flame-styled 直击→高;
  Python misses the title entirely, misreads 券后价→后价？ and
  直击→真击， but gets 破损就赔 right. Roughly even; neither is
  authoritative — the mobile rec model tops out around ~95% character
  accuracy on stylized text. C++/Go parity proves the Go FFI layer adds
  zero distortion; a real accuracy metric (CER) needs a labelled
  dataset, which is out of scope for this smoke comparison.
- **Why is Python slower if it uses the same ONNX Runtime?** Raw
  `session.run` timing on the same models/sizes shows the slowdown is
  almost entirely outside the ONNX kernels:
  - rec: raw inference ≈15.6 ms/line, but the Python pipeline reports
    ~157 ms/line — ~140 ms is Python glue (crop/resize/normalize +
    CTC decode + interpreter overhead) on every line.
  - det: the pypi onnxruntime 1.31.1 itself runs ~286 ms where the
    RapidAI-built static ORT 1.20.1 finishes the whole det stage in
    ~117 ms — a build/backend difference, not a language one.
- Go FFI overhead vs the same native code is roughly +10–17 % wall time;
  the purego (runtime-loaded dll) and cgo (linked dll) backends are within
  noise of each other.

## Reproduce

```bash
# C++ binaries (MSYS2 ucrt64):
cmake -B build-bins -G "MinGW Makefiles" -DCMAKE_BUILD_TYPE=Release \
  -DBUILD_STATIC_LIB=OFF -DBUILD_SHARED_LIB=OFF -DBUILD_BINS=ON \
  -DOpenCV_DIR=/c/msys64/ucrt64/lib/cmake/opencv5
cmake --build build-bins -j
cp build-clib/onnxruntime-1.20.dll build-clib/onnxruntime_providers_shared.dll build-bins/

pip install rapidocr_onnxruntime
./bench/bench.sh testdata/1.jpg 10
```

Individual pieces:

```bash
# upstream C++ (note: -l takes no arg, use the long option)
./build-bins/rapidocr-bench.exe -d models -1 ch_PP-OCRv4_det_mobile.onnx \
  -2 ch_ppocr_mobile_v2.0_cls_mobile.onnx -3 ch_PP-OCRv4_rec_mobile.onnx \
  -4 ppocr_keys_v1.txt -i testdata/1.jpg -u 2.0 --loopCount 10

# go backends
./rapidocr-cli -backend purego -lib lib/windows-x64/libRapidOcrOnnx.dll \
  -det ch_PP-OCRv4_det_mobile.onnx -cls ch_ppocr_mobile_v2.0_cls_mobile.onnx \
  -rec ch_PP-OCRv4_rec_mobile.onnx -n 10 testdata/1.jpg

# python
python bench/bench.py testdata/1.jpg 10
```
