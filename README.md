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
./scripts/fetch_assets.sh   # download shared lib + models (needs python3+py7zr, curl)

go run ./cmd/rapidocr-cli -backend purego \
    -lib lib/windows-x64/RapidOcrOnnx.dll \
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
cfg.LibraryPath = "lib/windows-x64/RapidOcrOnnx.dll" // purego only

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

## Status

Early scaffold. Working today: purego backend (verified end-to-end on
Windows x64 against the upstream `windows-clib` 1.2.3 dll). CGO static-link
backend code is in place; per-platform library builds are the next step.

See `AGENTS.md` for architecture and build details.
