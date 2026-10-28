//go:build cgo && (linux || windows || darwin) && !rapidocr_nocgo

// Package rapidocr CGO binding for Linux, Windows and macOS.
// Links the statically built RapidOcrOnnx wrapper + onnxruntime + opencv
// libraries from lib/{os}-{arch}/. On Windows it links MinGW import
// libraries (libRapidOcrOnnx.dll.a ...) produced by
// scripts/build_clib_mingw.sh, so the dlls must ship beside the binary.
package rapidocr

/*
#cgo CXXFLAGS: -std=c++17
#cgo CFLAGS: -I${SRCDIR}/../../include
// The shipped libRapidOcrOnnx.so is self-contained (onnxruntime + opencv
// statically linked inside it). If you instead link split static libs
// (librapidocr.a + separate dep .a files), add -lonnxruntime -lopencv_*
// back via CGO_LDFLAGS.
#cgo linux,amd64   LDFLAGS: -L${SRCDIR}/../../lib/linux-x64 -lRapidOcrOnnx -lstdc++ -lm -lpthread -ldl
#cgo linux,arm64   LDFLAGS: -L${SRCDIR}/../../lib/linux-arm64 -lRapidOcrOnnx -lstdc++ -lm -lpthread -ldl
#cgo windows,amd64 LDFLAGS: -L${SRCDIR}/../../lib/windows-x64 -lRapidOcrOnnx -lstdc++
#cgo darwin,amd64  LDFLAGS: -L${SRCDIR}/../../lib/darwin-x64 -lRapidOcrOnnx -lc++
#cgo darwin,arm64  LDFLAGS: -L${SRCDIR}/../../lib/darwin-arm64 -lRapidOcrOnnx -lc++
#include <stdlib.h>
#include "rapidocr.h"
*/
import "C"

import (
	"context"
	"fmt"
	"image"
	"os"
	"unsafe"
)

// cgoAvailable reports whether the CGO backend was compiled in.
const cgoAvailable = true

// cgoEngine implements Engine over the statically linked RapidOcrOnnx.
type cgoEngine struct {
	handle C.OCR_HANDLE
}

// cLastErr fetches the C++ exception message recorded by the safe wrapper.
func cLastErr() string {
	msg := C.rapidocr_last_error()
	if msg == nil {
		return ""
	}
	return ": " + C.GoString(msg)
}

// newCGOEngine loads the models and creates the upstream OCR_HANDLE.
func newCGOEngine(config *Config) (*cgoEngine, error) {
	det, cls, rec, keys := config.modelPaths()
	for _, p := range []string{det, cls, rec, keys} {
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("model file not found: %s", p)
		}
	}

	cDet := C.CString(det)
	cCls := C.CString(cls)
	cRec := C.CString(rec)
	cKeys := C.CString(keys)
	defer C.free(unsafe.Pointer(cDet))
	defer C.free(unsafe.Pointer(cCls))
	defer C.free(unsafe.Pointer(cRec))
	defer C.free(unsafe.Pointer(cKeys))

	// rapidocr_init catches C++ exceptions inside the library; plain
	// OcrInit would propagate them across the FFI boundary and crash.
	handle := C.rapidocr_init(cDet, cCls, cRec, cKeys, C.int(config.NumThreads))
	if handle == nil {
		return nil, fmt.Errorf("OcrInit failed (check model files under %s)%s", config.ModelDir, cLastErr())
	}
	return &cgoEngine{handle: handle}, nil
}

// toCParam maps Param to the C OCR_PARAM struct.
func toCParam(p *Param) C.OCR_PARAM {
	var cp C.OCR_PARAM
	if p == nil {
		return cp
	}
	cp.padding = C.int(p.Padding)
	cp.maxSideLen = C.int(p.MaxSideLen)
	cp.boxScoreThresh = C.float(p.BoxScoreThresh)
	cp.boxThresh = C.float(p.BoxThresh)
	cp.unClipRatio = C.float(p.UnClipRatio)
	if p.DoAngle {
		cp.doAngle = 1
	}
	if p.MostAngle {
		cp.mostAngle = 1
	}
	return cp
}

// convertResult copies the C OCR_RESULT into a Go Result and frees C memory.
func convertResult(res *C.OCR_RESULT) *Result {
	out := &Result{
		DBNetTime:  float64(res.dbNetTime),
		DetectTime: float64(res.detectTime),
	}
	count := int(res.textBlocksLength)
	if count == 0 || res.textBlocks == nil {
		return out
	}
	blocks := unsafe.Slice(res.textBlocks, count)
	out.TextBlocks = make([]TextBlock, 0, count)
	for i := range blocks {
		tb := &blocks[i]
		block := TextBlock{
			BoxScore:   float32(tb.boxScore),
			AngleIndex: int(tb.angleIndex),
			AngleScore: float32(tb.angleScore),
			AngleTime:  float64(tb.angleTime),
			CrnnTime:   float64(tb.crnnTime),
			BlockTime:  float64(tb.blockTime),
		}
		if tb.text != nil && tb.textLength > 1 {
			block.Text = C.GoStringN((*C.char)(unsafe.Pointer(tb.text)), C.int(tb.textLength-1))
		}
		if tb.boxPoint != nil && tb.boxPointLength >= 4 {
			pts := unsafe.Slice(tb.boxPoint, 4)
			for j := 0; j < 4; j++ {
				block.Box[j] = image.Point{X: int(pts[j].x), Y: int(pts[j].y)}
			}
		}
		if tb.charScores != nil && tb.charScoresLength > 0 {
			scores := unsafe.Slice(tb.charScores, int(tb.charScoresLength))
			block.CharScores = make([]float32, len(scores))
			for j, s := range scores {
				block.CharScores[j] = float32(s)
			}
		}
		out.TextBlocks = append(out.TextBlocks, block)
	}
	return out
}

// cgoLastError returns the C++ message recorded by the safe wrapper.
func cgoLastError() string {
	msg := C.rapidocr_last_error()
	if msg == nil {
		return ""
	}
	return ": " + C.GoString(msg)
}

// Detect runs OCR on an image.Image by converting it to RGB pixels.
func (e *cgoEngine) Detect(ctx context.Context, img image.Image, p *Param) (*Result, error) {
	data, w, h := imageToRGB(img)
	return e.DetectPixels(ctx, data, w, h, 3, p)
}

// DetectBytes runs OCR on encoded image bytes (PNG/JPEG...).
func (e *cgoEngine) DetectBytes(ctx context.Context, data []byte, p *Param) (*Result, error) {
	if e.handle == nil {
		return nil, fmt.Errorf("engine closed")
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("empty image data")
	}
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}
	// The input struct must live in C memory: passing a Go struct that
	// contains a Go pointer violates the cgo pointer rules.
	input := (*C.OCR_INPUT)(C.malloc(C.size_t(unsafe.Sizeof(C.OCR_INPUT{}))))
	defer C.free(unsafe.Pointer(input))
	*input = C.OCR_INPUT{}
	input._type = 1
	input.channels = 3
	input.data = (*C.uint8_t)(unsafe.Pointer(&data[0]))
	input.dataLength = C.long(len(data))

	cParam := toCParam(p)
	var res C.OCR_RESULT
	ok := C.rapidocr_detect_input(e.handle, input, &cParam, &res)
	if ok == 0 {
		return nil, fmt.Errorf("ocr detect failed%s", cgoLastError())
	}
	defer C.OcrFreeResult(&res)
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}
	return convertResult(&res), nil
}

// DetectPixels runs OCR on tightly packed raw pixels.
func (e *cgoEngine) DetectPixels(ctx context.Context, data []byte, width, height, channels int, p *Param) (*Result, error) {
	if e.handle == nil {
		return nil, fmt.Errorf("engine closed")
	}
	if channels < 1 || channels > 4 {
		return nil, fmt.Errorf("unsupported channel count: %d", channels)
	}
	if len(data) < width*height*channels {
		return nil, fmt.Errorf("data too short for %dx%dx%d", width, height, channels)
	}
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}
	input := (*C.OCR_INPUT)(C.malloc(C.size_t(unsafe.Sizeof(C.OCR_INPUT{}))))
	defer C.free(unsafe.Pointer(input))
	*input = C.OCR_INPUT{}
	input._type = 0
	input.channels = C.int(channels)
	input.width = C.int(width)
	input.height = C.int(height)
	input.data = (*C.uint8_t)(unsafe.Pointer(&data[0]))
	input.dataLength = C.long(len(data))

	cParam := toCParam(p)
	var res C.OCR_RESULT
	ok := C.rapidocr_detect_input(e.handle, input, &cParam, &res)
	if ok == 0 {
		return nil, fmt.Errorf("ocr detect failed%s", cgoLastError())
	}
	defer C.OcrFreeResult(&res)
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}
	return convertResult(&res), nil
}

// Close destroys the upstream handle.
func (e *cgoEngine) Close() error {
	if e.handle != nil {
		C.OcrDestroy(e.handle)
		e.handle = nil
	}
	return nil
}

// GetBackend returns BackendCGO.
func (e *cgoEngine) GetBackend() Backend {
	return BackendCGO
}
