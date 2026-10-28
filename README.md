# go-rapidocr

Go bindings for [RapidOcrOnnx](https://github.com/RapidAI/RapidOcrOnnx) — a
PP-OCR (det + cls + rec) pipeline built on ONNX Runtime and OpenCV.

Two backends, same API:

- **cgo** — statically links the RapidOcrOnnx engine (fully self-contained
  binaries). Per-platform static libs under `lib/{os}-{arch}/`.
- **purego** — `dlopen`s the upstream `libRapidOcrOnnx` shared library at
  runtime via [purego](https://github.com/ebitengine/purego). No C toolchain
  required.

## Quick start

```bash
./scripts/fetch_assets.sh   # lib/<os>-<arch>/ from our release + models
                            # (needs curl; --upstream for the legacy CLIB)

go run ./cmd/rapidocr-cli -backend purego \
    -lib lib/windows-x64/libRapidOcrOnnx.dll \
    -det ch_PP-OCRv4_det_mobile.onnx \
    -cls ch_ppocr_mobile_v2.0_cls_mobile.onnx \
    -rec ch_PP-OCRv4_rec_mobile.onnx \
    testdata/1.jpg
```

## Library usage

```go
cfg := rapidocr.DefaultConfig()
cfg.ModelDir = "models"
cfg.DetModel = "ch_PP-OCRv4_det_mobile.onnx"
cfg.ClsModel = "ch_ppocr_mobile_v2.0_cls_mobile.onnx"
cfg.RecModel = "ch_PP-OCRv4_rec_mobile.onnx"
cfg.LibraryPath = "lib/windows-x64/libRapidOcrOnnx.dll" // purego only

engine, err := rapidocr.New(cfg) // backend: auto
if err != nil { log.Fatal(err) }
defer engine.Close()

result, err := engine.DetectBytes(ctx, pngBytes, &rapidocr.Param{
    MaxSideLen: 1024, DoAngle: true, MostAngle: true,
})
for _, b := range result.TextBlocks {
    fmt.Println(b.BoxScore, b.Text, b.Box)
}
```

Semantics:

- `ctx` is checked before and after the native call — a cancelled context
  returns `ctx.Err()`. The C++ inference itself cannot be aborted
  mid-flight.
- Native failures (corrupt image bytes, engine errors) return an `error`
  with the C++ message attached when the safe-wrapper CLIB is used; an
  empty `*Result` with `nil` error means "no text found" only.

### Build tags

- `rapidocr_nocgo` — exclude the cgo backend even when `CGO_ENABLED=1`,
  producing a purego-only binary that does not link the native library
  (useful for cross-compiles and minimal deployments).

## Model selection

RapidOCR is a three-model pipeline — det (find text lines), cls (0/180°
rotation), rec (read characters) — and the `models/` you pick matter far
more for accuracy than the backend. `fetch_assets.sh --models <tier>`
downloads a consistent set:

| Tier | Models | Size | Speed (CPU) | Notes |
|------|--------|------|-------------|-------|
| `mobile` (default) | PP-OCRv4 mobile | ~16 MB | ~0.8 s/img | good baseline; slips on stylized fonts |
| `server` | PP-OCRv4 server det+rec | ~200 MB | ~30–135 s/img | best v4 accuracy |
| `v5-mobile` | PP-OCRv5 mobile | ~18 MB | ~1 s/img | newer arch |
| `v5-server` | PP-OCRv5 server | ~96 MB | slower | newest + accurate |

Measured on this repo's test images (see `bench/README.md`): server rec
fixed 3 of 4 mobile misreads but costs ~35–150x wall time. Choose per
workload: mobile for interactive use, server for batch/quality-critical.

Rules:

- **The keys/dict file MUST match the rec model** — `ppocr_keys_v1.txt`
  for PP-OCRv3/v4, `ppocrv5_dict.txt` for PP-OCRv5, `<lang>_dict.txt`
  for non-Chinese rec models (japan/korean/latin/cyrillic/…). Set
  `Config.KeysFile` accordingly; a mismatched dict yields garbage text.
- det/cls may be mixed across tiers (e.g. mobile det + server rec for a
  middle ground) — only the rec↔dict pairing is strict.
- Any `.onnx` path works; `Config.DetModel`/`ClsModel`/`RecModel`/`KeysFile`
  are just filenames inside `Config.ModelDir` or absolute paths.

## Building the CLIB yourself (recommended)

The upstream `windows-clib` release (<=1.2.3) exports only the legacy
text-only API. Building `RapidOcrOnnx` main with MSYS2 MinGW produces a
dll with the full structured `OcrDetectInput` API (boxes, scores, timings):

```bash
# MSYS2 ucrt64 shell:
pacman -S mingw-w64-ucrt-x86_64-toolchain mingw-w64-ucrt-x86_64-cmake \
          mingw-w64-ucrt-x86_64-opencv
# deps/: unzip onnxruntime-win-x64-1.20.1.zip into deps/
./scripts/build_clib_mingw.sh
# -> build-clib/libRapidOcrOnnx.dll + onnxruntime.dll
```

Then point `-lib`/`LibraryPath` at `build-clib/libRapidOcrOnnx.dll`. The
loader prefers sibling dlls next to it, so no PATH juggling is needed.

### Linux (Docker, glibc-frozen release artifacts)

```bash
./scripts/fetch_static_deps.sh x64   # or arm64
docker build --build-arg BASE_IMAGE=quay.io/pypa/manylinux2014_x86_64 \
    -t go-rapidocr-linux-x64 -f docker/Dockerfile.linux-clib-glibc .
cid=$(docker create go-rapidocr-linux-x64)
docker cp "$cid:/artifacts" lib/linux-x64 && docker rm "$cid"
```

Produces:

- `libRapidOcrOnnx.so` — self-contained shared lib (onnxruntime + opencv +
  codecs + libstdc++ inside), needs only ~GLIBC_2.16 (CentOS 7 compatible).
  Used by the purego backend and as the default cgo link target.
- `libRapidOcrOnnx.a` — merged static archive (engine + all deps). For
  cgo fully-static linking, point `CGO_LDFLAGS` at it explicitly:
  `CGO_LDFLAGS="-L$PWD/lib/linux-x64 -l:libRapidOcrOnnx.a -ldl -lpthread -lrt -lm"`.

linux-arm64 ships `libRapidOcrOnnx.so` + `libonnxruntime.so*` instead
(no glibc static ORT pack exists upstream; needs GLIBC_2.27/2.28).

## Status

Verified end-to-end on Windows x64 and Linux x64 (bookworm container):

- cgo backend: `TestCGODetectBytes` passes with boxes/scores/timings.
- purego backend: same result with `CGO_ENABLED=0`, no C toolchain.
- Legacy prebuilt CLIB (upstream <=1.2.3): automatic text-only fallback.

Fully-static cgo linking is available on linux-x64 via the merged
`libRapidOcrOnnx.a` release artifact. macOS/Windows ship shared libs only.
the next step.

See `AGENTS.md` for architecture and build details.
