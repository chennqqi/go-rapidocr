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

### MinGW shared build (verified)

`scripts/build_clib_mingw.sh` builds `libRapidOcrOnnx.dll` on Windows with
MSYS2 ucrt64: `pacman -S mingw-w64-ucrt-x86_64-toolchain -cmake -opencv`,
plus the official `onnxruntime-win-x64-*.zip` unpacked to `deps/`.
`deps/ort-compat/` remaps the flat onnxruntime headers to upstream's nested
`onnxruntime/core/session/` include layout. MinGW cannot link the MSVC
`onnxruntime.lib`; `dlltool` generates `libonnxruntime.dll.a` from
`cmake/onnxruntime.def`. OpenCV 5 moved `minAreaRect`/`getPerspectiveTransform`
to `opencv2/geometry/2d.hpp` — `include/cv_compat.h` is force-included when
`OpenCV_VERSION >= 5`.

Runtime dependencies of the MinGW dll: `onnxruntime-1.20.dll`,
`onnxruntime_providers_shared.dll`, `libopencv_*-500.dll`,
`libstdc++-6.dll`, `libgcc_s_seh-1.dll` (+ transitive codec dlls like
libjpeg/libpng/libwebp from msys2). `dlopen_windows.go` registers the
library's directory via `AddDllDirectory` and preloads a sibling
`onnxruntime.dll`/`onnxruntime-1.20.dll`, so dependencies resolve next to
the engine dll and older System32 copies (e.g. onnxruntime 1.17) are not
picked up. The import lib deliberately references the *versioned* name
`onnxruntime-1.20.dll` — a cgo-linked exe cannot preload dependencies
before its imports resolve, so the unique name is required there too.

### cgo backend on Windows

`CGO_ENABLED=1 go test` links `lib/windows-x64/*.dll.a` import libs and
needs `lib/windows-x64` + `C:\msys64\ucrt64\bin` on PATH for `gcc` and the
runtime dlls. `TestCGODetectBytes` verifies the real cgo path end-to-end.
Note: `OCR_INPUT`/`OCR_PARAM` structs that hold Go pointers must be
allocated via `C.malloc` — passing a Go struct containing a Go pointer
trips the cgo pointer checker ("Go pointer to unpinned Go pointer").

### Linux build (Docker)

`docker/Dockerfile.linux-clib` (golang:1.25-bookworm + `libopencv-dev` +
official `onnxruntime-linux-x64`) builds `libRapidOcrOnnx.so` and runs the
full suite inside the container — both cgo (links `lib/linux-x64/*.so`) and
purego paths pass. CMake links the versioned `libonnxruntime.so.1.x.y` with
`$ORIGIN` rpath so the loader finds it beside the engine lib.

### Platform lib builds (GitHub Actions)

`.github/workflows/build-libs.yml` (workflow_dispatch, artifacts only) builds
the CLIB shared libraries per platform:

- `linux-x64`: `docker/Dockerfile.linux-clib` inside ubuntu-latest
- `windows-x64`: MSYS2 UCRT64 + `scripts/build_clib_mingw.sh`;
  `scripts/collect_dll_deps.sh` recurses `objdump` import tables to stage
  every transitive MinGW dll beside the engine dll
- `darwin-x64`/`darwin-arm64`: macos-13/macos-15 + `brew install opencv` +
  `scripts/build_clib_macos.sh`

Downloaded artifacts are staged into `lib/<os>-<arch>/`; they are not
committed (lib binaries are gitignored).

### go-rapidocr C extensions

`rapidocr_init` / `rapidocr_detect_input` / `rapidocr_last_error` wrap the
upstream calls in try/catch. C++ exceptions (bad/missing models, corrupt
images) cannot cross the FFI boundary — they crash the process. Both
backends prefer the safe wrappers when exported; Go also stats model files
upfront so legacy libraries fail cleanly. On C++ exception the message is
surfaced via `rapidocr_last_error` and appended to the Go error.

### models/ppocr_keys_v1.txt must be LF-only

Upstream's `getline` loop does not strip `\r`. On Windows, text-mode reads
hide CRLF, but on Linux/macOS the trailing `\r` is appended to every key
label, so each decoded character leaks a `\r` into the result text.
`fetch_assets.sh` normalizes the file; keep it LF.

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
