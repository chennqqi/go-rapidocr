//go:build cgo && (linux || windows || darwin)

// Package rapidocr CGO binding for Linux, Windows and macOS.
// Links the statically built RapidOcrOnnx wrapper + onnxruntime + opencv
// libraries from lib/{os}-{arch}/. On Windows it links MinGW import
// libraries (libRapidOcrOnnx.dll.a ...) produced by
// scripts/build_clib_mingw.sh, so the dlls must ship beside the binary.
package rapidocr

/*
#cgo CXXFLAGS: -std=c++17
#cgo CFLAGS: -I${SRCDIR}/../../include
#cgo linux,amd64   LDFLAGS: -L${SRCDIR}/../../lib/linux-x64 -lrapidocr -lonnxruntime -lopencv_core -lopencv_imgproc -lopencv_imgcodecs -lstdc++ -lm -lpthread -ldl
#cgo linux,arm64   LDFLAGS: -L${SRCDIR}/../../lib/linux-arm64 -lrapidocr -lonnxruntime -lopencv_core -lopencv_imgproc -lopencv_imgcodecs -lstdc++ -lm -lpthread -ldl
#cgo windows,amd64 LDFLAGS: -L${SRCDIR}/../../lib/windows-x64 -lRapidOcrOnnx -lonnxruntime -lopencv_core -lopencv_imgproc -lopencv_imgcodecs -lstdc++
#cgo darwin,amd64  LDFLAGS: -L${SRCDIR}/../../lib/darwin-x64 -lrapidocr -lonnxruntime -lopencv_core -lopencv_imgproc -lopencv_imgcodecs -lc++
#cgo darwin,arm64  LDFLAGS: -L${SRCDIR}/../../lib/darwin-arm64 -lrapidocr -lonnxruntime -lopencv_core -lopencv_imgproc -lopencv_imgcodecs -lc++
#include <stdlib.h>
#include "rapidocr.h"
*/
import "C"

import (
	"context"
	"fmt"
	"image"
	"path/filepath"
	"unsafe"
)

// cgoAvailable reports whether the CGO backend was compiled in.
const cgoAvailable = true

// cgoEngine implements Engine over the statically linked RapidOcrOnnx.
type cgoEngine struct {
	handle C.OCR_HANDLE
}

// newCGOEngine loads the models and creates the upstream OCR_HANDLE.
func newCGOEngine(config *Config) (*cgoEngine, error) {
	det, cls, rec, keys := config.modelPaths()
	for _, p := range []string{det, cls, rec, keys} {
		if _, err := filepath.Abs(p); err != nil {
			return nil, fmt.Errorf("invalid model path %s: %w", p, err)
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

	handle := C.OcrInit(cDet, cCls, cRec, cKeys, C.int(config.NumThreads))
	if handle == nil {
		return nil, fmt.Errorf("OcrInit failed (check model files under %s)", config.ModelDir)
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
	ok := C.OcrDetectInput(e.handle, input, &cParam, &res)
	if ok == 0 {
		return &Result{}, nil
	}
	defer C.OcrFreeResult(&res)
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
	ok := C.OcrDetectInput(e.handle, input, &cParam, &res)
	if ok == 0 {
		return &Result{}, nil
	}
	defer C.OcrFreeResult(&res)
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
