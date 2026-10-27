//go:build cgo && (linux || windows || darwin)

package rapidocr

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCGODetectBytes exercises the statically/dynamically linked cgo backend.
// On Windows the test binary needs the dependency dlls on PATH
// (lib/windows-x64, msys2 ucrt64/bin).
func TestCGODetectBytes(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.Backend = BackendCGO
	cfg.ModelDir = filepath.Join(root, "models")
	cfg.DetModel = "ch_PP-OCRv4_det_mobile.onnx"
	cfg.ClsModel = "ch_ppocr_mobile_v2.0_cls_mobile.onnx"
	cfg.RecModel = "ch_PP-OCRv4_rec_mobile.onnx"
	for _, m := range []string{cfg.DetModel, cfg.ClsModel, cfg.RecModel, cfg.KeysFile} {
		if _, err := os.Stat(filepath.Join(cfg.ModelDir, m)); err != nil {
			t.Skipf("model not present: %v", err)
		}
	}

	result := runDetect(t, cfg)
	structured := false
	for _, b := range result.TextBlocks {
		if b.BoxScore > 0 && b.Box[2].X > b.Box[0].X {
			structured = true
		}
	}
	if !structured {
		t.Error("cgo backend returned no box/score data")
	}
}
