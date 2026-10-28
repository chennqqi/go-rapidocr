//go:build (linux || windows || darwin) && !js

// PureGo backend: loads the RapidOcrOnnx CLIB shared library at runtime via
// ebitengine/purego. No cgo toolchain is required to build this package.
//
// The bound symbols are the upstream C API exported by the CLIB build:
//
//	OCR_HANDLE OcrInit(const char*, const char*, const char*, const char*, int)
//	OCR_BOOL   OcrDetectInput(OCR_HANDLE, OCR_INPUT*, OCR_PARAM*, OCR_RESULT*)
//	OCR_BOOL   OcrFreeResult(OCR_RESULT*)
//	void       OcrDestroy(OCR_HANDLE)
package rapidocr

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"
)

// --- C ABI struct layouts (all fields written/read through []byte + offsets
// so layout is identical across platforms; OCR_INPUT is 32 bytes on every
// 64-bit OS since Win32 `long` is 4 bytes but sits in the same offset). ---

const (
	ocrInputSize  = 32
	ocrParamSize  = 28
	ocrResultSize = 32
	textBlockSize = 88
	ocrPointSize  = 16
)

// ocrInputOff holds field offsets of the C OCR_INPUT struct.
var ocrInputOff = struct{ data, typ, channels, width, height, dataLen int }{0, 8, 12, 16, 20, 24}

// libRapidOcr bundles the bound C functions. ocrDetectInput is optional:
// upstream prebuilt CLIB releases (<=1.2.3) only export the legacy
// OcrDetect/OcrGetLen/OcrGetResult string API, which reports plain text
// without boxes or scores.
type libRapidOcr struct {
	handle uintptr

	ocrInit        func(det, cls, rec, keys string, nThreads int32) uintptr
	ocrDetectInput func(handle uintptr, input, param, result unsafe.Pointer) int8
	ocrFreeResult  func(result unsafe.Pointer) int8
	ocrDetect      func(handle uintptr, imgPath, imgName string, param unsafe.Pointer) int8
	ocrGetLen      func(handle uintptr) int32
	ocrGetResult   func(handle uintptr, buf *byte, n int32) int8
	ocrDestroy     func(handle uintptr)

	// go-rapidocr extensions, present only in our own CLIB builds.
	safeInit    func(det, cls, rec, keys string, nThreads int32) uintptr
	safeDetect  func(handle uintptr, input, param, result unsafe.Pointer) int8
	lastError   func() *byte

	hasInputAPI bool
	hasSafeAPI  bool
}

// puregoEngine implements Engine over the CLIB shared library.
type puregoEngine struct {
	lib  *libRapidOcr
	ocrH uintptr
}

// sharedLibName returns the platform library file name.
func sharedLibName() string {
	switch runtime.GOOS {
	case "windows":
		return "RapidOcrOnnx.dll"
	case "darwin":
		return "libRapidOcrOnnx.dylib"
	default:
		return "libRapidOcrOnnx.so"
	}
}

// loadLibrary tries LibraryPath, then the executable's directory and a "lib"
// subdirectory, then the system search path.
func loadLibrary(config *Config) (*libRapidOcr, error) {
	candidates := []string{}
	if len(config.LibraryPath) > 0 {
		candidates = append(candidates, config.LibraryPath)
	}
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, sharedLibName()),
			filepath.Join(dir, "lib", sharedLibName()))
	}
	candidates = append(candidates, sharedLibName())

	var lastErr error
	for _, path := range candidates {
		h, err := dlopen(path)
		if err != nil {
			lastErr = err
			continue
		}
		lib := &libRapidOcr{handle: h}
		registerLibFunc(&lib.ocrInit, h, "OcrInit")
		registerLibFunc(&lib.ocrDestroy, h, "OcrDestroy")
		if addr := lookupFunc(h, "OcrDetectInput"); addr != 0 {
			lib.hasInputAPI = true
			registerLibFunc(&lib.ocrDetectInput, h, "OcrDetectInput")
			registerLibFunc(&lib.ocrFreeResult, h, "OcrFreeResult")
		} else {
			registerLibFunc(&lib.ocrDetect, h, "OcrDetect")
			registerLibFunc(&lib.ocrGetLen, h, "OcrGetLen")
			registerLibFunc(&lib.ocrGetResult, h, "OcrGetResult")
		}
		// go-rapidocr safe wrappers (try/catch inside the C++ lib): bound
		// only in our own builds, so each is optional.
		if addr := lookupFunc(h, "rapidocr_init"); addr != 0 {
			lib.hasSafeAPI = true
			registerLibFunc(&lib.safeInit, h, "rapidocr_init")
			registerLibFunc(&lib.safeDetect, h, "rapidocr_detect_input")
			registerLibFunc(&lib.lastError, h, "rapidocr_last_error")
		}
		return lib, nil
	}
	return nil, fmt.Errorf("failed to load %s: %w", sharedLibName(), lastErr)
}

// newPureGoEngine loads the shared library and initializes models.
func newPureGoEngine(config *Config) (Engine, error) {
	lib, err := loadLibrary(config)
	if err != nil {
		return nil, err
	}

	det, cls, rec, keys := config.modelPaths()
	// Fail fast when model files are absent: on libraries without the
	// rapidocr_init safe wrapper, upstream OcrInit throws a C++ exception
	// that would crash the process across the FFI boundary.
	for _, p := range []string{det, cls, rec, keys} {
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("model file not found: %s", p)
		}
	}
	var ocrH uintptr
	if lib.hasSafeAPI {
		ocrH = lib.safeInit(det, cls, rec, keys, int32(config.NumThreads))
	} else {
		ocrH = lib.ocrInit(det, cls, rec, keys, int32(config.NumThreads))
	}
	if ocrH == 0 {
		return nil, fmt.Errorf("OcrInit failed (check model files under %s)%s", config.ModelDir, lib.errSuffix())
	}
	return &puregoEngine{lib: lib, ocrH: ocrH}, nil
}

// errSuffix appends the C++ exception message captured by the safe wrapper.
func (l *libRapidOcr) errSuffix() string {
	if !l.hasSafeAPI {
		return ""
	}
	msg := cStringPtr(l.lastError())
	if len(msg) == 0 {
		return ""
	}
	return ": " + msg
}

// cStringPtr reads a NUL-terminated string at p.
func cStringPtr(p *byte) string {
	if p == nil {
		return ""
	}
	var buf []byte
	for i := 0; i < 4096; i++ {
		c := *(*byte)(unsafe.Add(unsafe.Pointer(p), i))
		if c == 0 {
			break
		}
		buf = append(buf, c)
	}
	return string(buf)
}

// buildParam serializes Param into the C OCR_PARAM layout.
func buildParam(p *Param) []byte {
	buf := make([]byte, ocrParamSize)
	if p == nil {
		return buf
	}
	binary.LittleEndian.PutUint32(buf[0:], uint32(int32(p.Padding)))
	binary.LittleEndian.PutUint32(buf[4:], uint32(int32(p.MaxSideLen)))
	binary.LittleEndian.PutUint32(buf[8:], math.Float32bits(p.BoxScoreThresh))
	binary.LittleEndian.PutUint32(buf[12:], math.Float32bits(p.BoxThresh))
	binary.LittleEndian.PutUint32(buf[16:], math.Float32bits(p.UnClipRatio))
	if p.DoAngle {
		binary.LittleEndian.PutUint32(buf[20:], 1)
	}
	if p.MostAngle {
		binary.LittleEndian.PutUint32(buf[24:], 1)
	}
	return buf
}

// buildInput serializes an OCR_INPUT. When encoded is true, typ=1 (image
// bytes) and channels selects gray/color handling upstream.
func buildInput(data []byte, width, height, channels int, encoded bool) []byte {
	buf := make([]byte, ocrInputSize)
	o := ocrInputOff
	binary.LittleEndian.PutUint64(buf[o.data:], uint64(uintptr(unsafe.Pointer(&data[0]))))
	if encoded {
		binary.LittleEndian.PutUint32(buf[o.typ:], 1)
	} else {
		binary.LittleEndian.PutUint32(buf[o.typ:], 0)
	}
	binary.LittleEndian.PutUint32(buf[o.channels:], uint32(int32(channels)))
	binary.LittleEndian.PutUint32(buf[o.width:], uint32(int32(width)))
	binary.LittleEndian.PutUint32(buf[o.height:], uint32(int32(height)))
	binary.LittleEndian.PutUint64(buf[o.dataLen:], uint64(len(data)))
	return buf
}

// ptrField reinterprets an 8-byte field of a C-ABI buffer as a pointer.
func ptrField(field *byte) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(field))
}

// readResult parses the C OCR_RESULT buffer into a Go Result.
func readResult(buf []byte) *Result {
	out := &Result{
		DBNetTime:  math.Float64frombits(binary.LittleEndian.Uint64(buf[0:])),
		DetectTime: math.Float64frombits(binary.LittleEndian.Uint64(buf[24:])),
	}
	blocksPtr := ptrField(&buf[8])
	count := int(binary.LittleEndian.Uint64(buf[16:]))
	if count == 0 || blocksPtr == nil {
		return out
	}
	out.TextBlocks = make([]TextBlock, 0, count)
	for i := 0; i < count; i++ {
		tb := (*[textBlockSize]byte)(unsafe.Add(blocksPtr, i*textBlockSize))
		block := TextBlock{
			BoxScore:   math.Float32frombits(binary.LittleEndian.Uint32(tb[8:])),
			AngleIndex: int(int32(binary.LittleEndian.Uint32(tb[12:]))),
			AngleScore: math.Float32frombits(binary.LittleEndian.Uint32(tb[16:])),
			AngleTime:  math.Float64frombits(binary.LittleEndian.Uint64(tb[24:])),
			CrnnTime:   math.Float64frombits(binary.LittleEndian.Uint64(tb[72:])),
			BlockTime:  math.Float64frombits(binary.LittleEndian.Uint64(tb[80:])),
		}
		textPtr := ptrField(&tb[32])
		textLen := int(binary.LittleEndian.Uint64(tb[64:]))
		if textPtr != nil && textLen > 1 {
			raw := unsafe.Slice((*byte)(textPtr), textLen-1)
			block.Text = string(raw)
		}
		boxPtr := ptrField(&tb[0])
		boxLen := int(binary.LittleEndian.Uint64(tb[56:]))
		if boxPtr != nil && boxLen >= 4 {
			for j := 0; j < 4; j++ {
				pt := (*[ocrPointSize]byte)(unsafe.Add(boxPtr, j*ocrPointSize))
				block.Box[j] = image.Point{
					X: int(math.Float64frombits(binary.LittleEndian.Uint64(pt[0:]))),
					Y: int(math.Float64frombits(binary.LittleEndian.Uint64(pt[8:]))),
				}
			}
		}
		scoresPtr := ptrField(&tb[40])
		scoresLen := int(binary.LittleEndian.Uint64(tb[48:]))
		if scoresPtr != nil && scoresLen > 0 {
			raw := unsafe.Slice((*float32)(scoresPtr), scoresLen)
			block.CharScores = append([]float32(nil), raw...)
		}
		out.TextBlocks = append(out.TextBlocks, block)
	}
	return out
}

// detect runs OcrDetectInput with pinned input data. The FFI call itself
// is not interruptible; ctx is honoured before and after it.
func (e *puregoEngine) detect(ctx context.Context, data []byte, width, height, channels int, encoded bool, p *Param) (*Result, error) {
	if e.lib == nil || e.ocrH == 0 {
		return nil, fmt.Errorf("engine closed")
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("empty image data")
	}
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}
	if !e.lib.hasInputAPI {
		return e.detectLegacy(ctx, data, width, height, channels, encoded, p)
	}
	input := buildInput(data, width, height, channels, encoded)
	param := buildParam(p)
	result := make([]byte, ocrResultSize)

	var ok int8
	if e.lib.hasSafeAPI {
		ok = e.lib.safeDetect(e.ocrH, unsafe.Pointer(&input[0]), unsafe.Pointer(&param[0]), unsafe.Pointer(&result[0]))
	} else {
		ok = e.lib.ocrDetectInput(e.ocrH, unsafe.Pointer(&input[0]), unsafe.Pointer(&param[0]), unsafe.Pointer(&result[0]))
	}
	// Keep the caller's pixel/encoded bytes alive across the foreign call —
	// OCR_INPUT.data points into it.
	runtime.KeepAlive(data)
	if ok == 0 {
		return nil, fmt.Errorf("ocr detect failed%s", e.lib.errSuffix())
	}
	out := readResult(result)
	e.lib.ocrFreeResult(unsafe.Pointer(&result[0]))
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

// detectLegacy handles upstream prebuilt CLIB libraries (<=1.2.3) that only
// export the path-based OcrDetect/OcrGetLen/OcrGetResult API. It writes the
// image to a temporary file and returns text-only results.
func (e *puregoEngine) detectLegacy(ctx context.Context, data []byte, width, height, channels int, encoded bool, p *Param) (*Result, error) {
	// The legacy lib has no exception-safe wrapper: a C++ throw inside
	// OcrDetect crosses the FFI boundary and kills the process. Decode in
	// Go first so bad input fails here, and always feed it a PNG.
	var imgData []byte
	if encoded {
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("undecodable image data: %w", err)
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return nil, fmt.Errorf("failed to re-encode image for legacy API: %w", err)
		}
		imgData = buf.Bytes()
	} else {
		img := rawToRGBA(data, width, height, channels)
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return nil, fmt.Errorf("failed to encode pixels for legacy API: %w", err)
		}
		imgData = buf.Bytes()
	}

	tmp, err := os.CreateTemp("", "rapidocr-*.png")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp image: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(imgData); err != nil {
		tmp.Close()
		return nil, fmt.Errorf("failed to write temp image: %w", err)
	}
	tmp.Close()

	dir, name := filepath.Split(tmpPath)
	if len(dir) > 0 {
		dir = filepath.ToSlash(dir)
	}
	param := buildParam(p)
	ok := e.lib.ocrDetect(e.ocrH, dir, name, unsafe.Pointer(&param[0]))
	if ok == 0 {
		return nil, fmt.Errorf("ocr detect failed")
	}
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}
	n := e.lib.ocrGetLen(e.ocrH)
	if n <= 1 {
		return &Result{}, nil
	}
	buf := make([]byte, n)
	e.lib.ocrGetResult(e.ocrH, &buf[0], n)
	text := cStringToGo(buf)
	out := &Result{}
	for _, line := range strings.Split(text, "\n") {
		if len(line) > 0 {
			out.TextBlocks = append(out.TextBlocks, TextBlock{Text: line})
		}
	}
	return out, nil
}

// cStringToGo converts a NUL-terminated buffer to a Go string.
func cStringToGo(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// rawToRGBA wraps tightly packed pixels as an image.RGBA for encoding.
func rawToRGBA(data []byte, width, height, channels int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	switch channels {
	case 4:
		copy(img.Pix, data[:width*height*4])
	case 3:
		for i := 0; i < width*height; i++ {
			img.Pix[i*4] = data[i*3]
			img.Pix[i*4+1] = data[i*3+1]
			img.Pix[i*4+2] = data[i*3+2]
			img.Pix[i*4+3] = 0xFF
		}
	case 1:
		for i := 0; i < width*height; i++ {
			img.Pix[i*4] = data[i]
			img.Pix[i*4+1] = data[i]
			img.Pix[i*4+2] = data[i]
			img.Pix[i*4+3] = 0xFF
		}
	}
	return img
}

// Detect runs OCR on an image.Image.
func (e *puregoEngine) Detect(ctx context.Context, img image.Image, p *Param) (*Result, error) {
	data, w, h := imageToRGB(img)
	return e.DetectPixels(ctx, data, w, h, 3, p)
}

// DetectBytes runs OCR on encoded image bytes.
func (e *puregoEngine) DetectBytes(ctx context.Context, data []byte, p *Param) (*Result, error) {
	return e.detect(ctx, data, 0, 0, 3, true, p)
}

// DetectPixels runs OCR on tightly packed raw pixels.
func (e *puregoEngine) DetectPixels(ctx context.Context, data []byte, width, height, channels int, p *Param) (*Result, error) {
	if channels < 1 || channels > 4 {
		return nil, fmt.Errorf("unsupported channel count: %d", channels)
	}
	if len(data) < width*height*channels {
		return nil, fmt.Errorf("data too short for %dx%dx%d", width, height, channels)
	}
	return e.detect(ctx, data, width, height, channels, false, p)
}

// Close destroys the OCR handle and unloads the library handle record.
func (e *puregoEngine) Close() error {
	if e.lib != nil && e.ocrH != 0 {
		e.lib.ocrDestroy(e.ocrH)
		e.ocrH = 0
	}
	return nil
}

// GetBackend returns BackendPureGo.
func (e *puregoEngine) GetBackend() Backend {
	return BackendPureGo
}
