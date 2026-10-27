# AGENTS.md

Guidelines for AI agents and contributors working on this repository.

## Project Layout

- `rapidocr-onnx/` — git submodule of RapidAI/RapidOcrOnnx (C++ engine)
- `include/rapidocr.h` — pure-C declarations matching the upstream `OcrLiteCApi` ABI
  (the upstream header is `#ifdef __cplusplus`-guarded and unusable from cgo)
- `src/rapidocr.cpp` — wrapper source adding go-rapidocr extensions
- `pkg/rapidocr/` — Go package: `interface.go` (Engine API), `config.go`,
  `factory.go`, `cgo_binding.go`/`cgo_stub.go` (static-link backend),
  `purego_impl.go`/`dlopen_{unix,windows}.go`/`purego_stub.go` (runtime-dlopen backend)
- `cmd/rapidocr-cli/` — CLI demo
- `models/` — det/cls/rec `.onnx` + `ppocr_keys_v1.txt` (fetched, not committed)
- `lib/{os}-{arch}/` — per-platform static libs (cgo) or shared libs (purego)

## Backends

| Backend | How it works | Requires |
|---------|--------------|----------|
| `cgo` | statically links `librapidocr.a` + onnxruntime + opencv | CGO_ENABLED=1, C toolchain, per-platform static libs |
| `purego` | `dlopen`/`LoadLibrary` on `libRapidOcrOnnx` shared lib at runtime | shared lib file only; no C toolchain |

`BackendAuto` selects cgo when compiled with CGO enabled, else purego.

### Legacy CLIB caveat

Upstream release CLIB (<=1.2.3) exports only `OcrInit/OcrDetect/OcrGetLen/OcrGetResult/OcrDestroy`
— the structured `OcrDetectInput` API exists only on the main branch. The purego
backend detects missing `OcrDetectInput` and falls back to a temp-file + plain-text
path (no boxes/scores). Building our own CLIB from `rapidocr-onnx` main restores
full structured output.

## Verification

```bash
# fetch prebuilt CLIB + models
./scripts/fetch_assets.sh        # needs python + py7zr, curl

# run tests (purego backend, works with CGO_ENABLED=0)
go test ./pkg/rapidocr/ -v

# CLI
go run ./cmd/rapidocr-cli -backend purego -lib lib/windows-x64/RapidOcrOnnx.dll \
  -det ch_PP-OCRv4_det_mobile.onnx -cls ch_ppocr_mobile_v2.0_cls_mobile.onnx \
  -rec ch_PP-OCRv4_rec_mobile.onnx testdata/1.jpg
```

## Building the C++ static library (cgo backend)

Not yet automated per-platform; see `CMakeLists.txt`. Requires deps from
RapidAI/OpenCVBuilder and RapidAI/OnnxruntimeBuilder releases unpacked under
`deps/{opencv-static,onnxruntime-static}/<platform>`.

- Linux static libs must target old glibc (same rule as sister project zxing:
  build on CentOS 7 / manylinux, never Alpine musl, for the *Go binary*; a musl
  static lib cannot be linked into a glibc cgo binary).
- Windows upstream static libs are MSVC — unusable by MinGW cgo. Options:
  link `onnxruntime.dll` via a generated import `.a`, or build deps with
  MinGW/MSYS2. The purego backend sidesteps this entirely.

## Code Style

- Go code: use `len(strVal) == 0` instead of `strVal == ""` for string comparison
- Comments, documentation, and log messages: English
- New functions: doc comments
- Use early return pattern
- File encoding: UTF-8
- C ABI structs are marshalled with explicit byte offsets in purego_impl.go;
  do NOT rely on Go struct layout matching the C layout.
