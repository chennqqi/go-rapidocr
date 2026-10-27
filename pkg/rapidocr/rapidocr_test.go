//go:build (linux || windows || darwin) && !js

package rapidocr

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

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
	cfg.LibraryPath = filepath.Join(root, "lib", "windows-x64", "RapidOcrOnnx.dll")
	if _, err := os.Stat(cfg.LibraryPath); err != nil {
		t.Skipf("shared library not present: %v", err)
	}
	for _, m := range []string{cfg.DetModel, cfg.ClsModel, cfg.RecModel, cfg.KeysFile} {
		if _, err := os.Stat(filepath.Join(cfg.ModelDir, m)); err != nil {
			t.Skipf("model not present: %v", err)
		}
	}
	return cfg
}

func TestPureGoDetectBytes(t *testing.T) {
	cfg := testConfig(t)
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
	t.Logf("got %d blocks", len(result.TextBlocks))
	for i, b := range result.TextBlocks {
		t.Logf("[%d] %q", i, b.Text)
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
