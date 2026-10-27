//go:build (linux || windows || darwin) && !js

package rapidocr

import (
	"context"
	"image"
	_ "image/jpeg"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// libDirName returns the platform library directory name.
func libDirName() string {
	arch := "x64"
	if runtime.GOARCH == "arm64" {
		arch = "arm64"
	}
	switch runtime.GOOS {
	case "windows":
		return "windows-" + arch
	case "darwin":
		return "darwin-" + arch
	default:
		return "linux-" + arch
	}
}

// testConfig builds a Config pointing at repo-local models and the shared
// library under lib/. Tests skip when assets are absent.
func testConfig(t *testing.T) *Config {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.Backend = BackendPureGo
	cfg.ModelDir = filepath.Join(root, "models")
	cfg.DetModel = "ch_PP-OCRv4_det_mobile.onnx"
	cfg.ClsModel = "ch_ppocr_mobile_v2.0_cls_mobile.onnx"
	cfg.RecModel = "ch_PP-OCRv4_rec_mobile.onnx"
	// Prefer the locally built CLIB (full OcrDetectInput API); fall back to
	// the upstream prebuilt CLIB (legacy text-only API). RAPIDOCR_LIB
	// overrides the library path (e.g. a fresh build output directory).
	candidates := []string{os.Getenv("RAPIDOCR_LIB")}
	libDir := filepath.Join(root, "lib", libDirName())
	for _, name := range []string{sharedLibName(), "lib" + sharedLibName(), "RapidOcrOnnx.dll"} {
		candidates = append(candidates, filepath.Join(libDir, name))
	}
	candidates = append(candidates, filepath.Join(root, "build-clib-linux", sharedLibName()),
		filepath.Join(root, "build-clib", sharedLibName()))
	for _, p := range candidates {
		if len(p) == 0 {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			cfg.LibraryPath = p
			break
		}
	}
	if len(cfg.LibraryPath) == 0 {
		t.Skipf("shared library not present in %s", libDir)
	}
	for _, m := range []string{cfg.DetModel, cfg.ClsModel, cfg.RecModel, cfg.KeysFile} {
		if _, err := os.Stat(filepath.Join(cfg.ModelDir, m)); err != nil {
			t.Skipf("model not present: %v", err)
		}
	}
	return cfg
}

// runDetect exercises an engine on the shared test image and returns the
// result for per-backend assertions.
func runDetect(t *testing.T, cfg *Config) *Result {
	t.Helper()
	engine, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer engine.Close()

	imgPath := filepath.Join("..", "..", "testdata", "1.jpg")
	data, err := os.ReadFile(imgPath)
	if err != nil {
		t.Skipf("test image not present: %v", err)
	}

	result, err := engine.DetectBytes(context.Background(), data, &Param{DoAngle: true, MostAngle: true})
	if err != nil {
		t.Fatalf("DetectBytes: %v", err)
	}
	if len(result.TextBlocks) == 0 {
		t.Fatal("expected text blocks, got none")
	}
	return result
}

func TestPureGoDetectBytes(t *testing.T) {
	cfg := testConfig(t)
	result := runDetect(t, cfg)

	t.Logf("got %d blocks", len(result.TextBlocks))
	structured := false
	for i, b := range result.TextBlocks {
		t.Logf("[%d] %q", i, b.Text)
		if b.BoxScore > 0 && b.Box[2].X > b.Box[0].X {
			structured = true
		}
	}
	// The upstream prebuilt CLIB (legacy OcrDetect API) returns text only;
	// a locally built CLIB must provide boxes and scores.
	if strings.Contains(cfg.LibraryPath, "libRapidOcrOnnx") && !structured {
		t.Error("structured CLIB loaded but no box/score data returned")
	}
}

// TestPureGoDetectImage exercises Detect(image.Image) -> DetectPixels path,
// covering imageToRGB and the raw-pixel OCR_INPUT branch.
func TestPureGoDetectImage(t *testing.T) {
	cfg := testConfig(t)
	// The legacy path API cannot accept raw pixels directly (it re-encodes
	// through PNG), so this still works on the old prebuilt CLIB.
	engine, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer engine.Close()

	f, err := os.Open(filepath.Join("..", "..", "testdata", "1.jpg"))
	if err != nil {
		t.Skipf("test image not present: %v", err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatalf("decode test image: %v", err)
	}

	result, err := engine.Detect(context.Background(), img, nil)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(result.TextBlocks) == 0 {
		t.Fatal("expected text blocks, got none")
	}
	t.Logf("image path got %d blocks", len(result.TextBlocks))
}

func TestPureGoDetectEmptyData(t *testing.T) {
	cfg := testConfig(t)
	engine, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer engine.Close()
	if _, err := engine.DetectBytes(context.Background(), nil, nil); err == nil {
		t.Error("expected error on empty image data")
	}
}

func TestPureGoBadModelPath(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendPureGo
	cfg.LibraryPath = ""
	cfg.ModelDir = t.TempDir() // no models inside
	root, _ := filepath.Abs("../..")
	for _, name := range []string{sharedLibName(), "lib" + sharedLibName()} {
		if _, err := os.Stat(filepath.Join(root, "lib", libDirName(), name)); err == nil {
			cfg.LibraryPath = filepath.Join(root, "lib", libDirName(), name)
			break
		}
	}
	if len(cfg.LibraryPath) == 0 {
		t.Skip("shared library not present")
	}
	if _, err := New(cfg); err == nil {
		t.Error("expected OcrInit failure with missing models")
	}
}

// TestPureGoCorruptModel exercises the rapidocr_init safe wrapper: the
// C++ exception raised by ONNX Runtime on a garbage model file must come
// back as a Go error, not a crash.
func TestPureGoCorruptModel(t *testing.T) {
	root, _ := filepath.Abs("../..")
	libDir := filepath.Join(root, "lib", libDirName())
	var lib string
	for _, name := range []string{"lib" + sharedLibName(), sharedLibName()} {
		p := filepath.Join(libDir, name)
		if _, err := os.Stat(p); err == nil {
			lib = p
			break
		}
	}
	if len(lib) == 0 {
		t.Skip("structured CLIB not present")
	}

	dir := t.TempDir()
	garbage := filepath.Join(dir, "bad.onnx")
	if err := os.WriteFile(garbage, []byte("not an onnx model"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.Backend = BackendPureGo
	cfg.LibraryPath = lib
	cfg.ModelDir = dir
	cfg.DetModel = "bad.onnx"
	cfg.ClsModel = "bad.onnx"
	cfg.RecModel = "bad.onnx"
	cfg.KeysFile = "bad.onnx"
	if _, err := New(cfg); err == nil {
		t.Error("expected error for corrupt models")
	} else {
		t.Logf("init error: %v", err)
	}
}

func TestParamZeroIsValid(t *testing.T) {
	buf := buildParam(nil)
	for i, b := range buf {
		if b != 0 {
			t.Fatalf("nil param should produce all-zero C param, buf[%d]=%d", i, b)
		}
	}
}
