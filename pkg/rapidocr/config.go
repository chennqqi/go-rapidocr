package rapidocr

import "os"

// Backend selects the engine implementation.
type Backend string

const (
	// BackendCGO links the RapidOcrOnnx static libraries via cgo.
	BackendCGO Backend = "cgo"
	// BackendPureGo loads the RapidOcrOnnx shared library at runtime
	// via purego (no cgo toolchain required).
	BackendPureGo Backend = "purego"
	// BackendAuto picks cgo when available, otherwise purego.
	BackendAuto Backend = "auto"
)

// Config configures an Engine.
type Config struct {
	// Backend selects the implementation; BackendAuto is recommended.
	Backend Backend

	// ModelDir is the directory holding the model files.
	ModelDir string
	// DetModel is the detection model file name inside ModelDir.
	DetModel string
	// ClsModel is the direction classifier model file name inside ModelDir.
	ClsModel string
	// RecModel is the recognition model file name inside ModelDir.
	RecModel string
	// KeysFile is the recognition dictionary file name inside ModelDir.
	KeysFile string

	// NumThreads sets the onnxruntime thread count; 0 lets ORT decide.
	NumThreads int

	// LibraryPath is the RapidOcrOnnx CLIB shared library path used by the
	// purego backend (e.g. RapidOcrOnnx.dll / libRapidOcrOnnx.so).
	// Empty means search standard locations next to the executable and in
	// the "lib" subdirectory.
	LibraryPath string

	// Debug enables verbose logging.
	Debug bool
}

// DefaultConfig returns a Config with upstream default model file names.
func DefaultConfig() *Config {
	return &Config{
		Backend:    BackendAuto,
		ModelDir:   "models",
		DetModel:   "ch_PP-OCRv3_det_infer.onnx",
		ClsModel:   "ch_ppocr_mobile_v2.0_cls_infer.onnx",
		RecModel:   "ch_PP-OCRv3_rec_infer.onnx",
		KeysFile:   "ppocr_keys_v1.txt",
		NumThreads: 4,
	}
}

// LoadConfigFromEnv reads RAPIDOCR_* environment variables over the defaults.
func LoadConfigFromEnv() *Config {
	config := DefaultConfig()
	if v := os.Getenv("RAPIDOCR_BACKEND"); len(v) > 0 {
		config.Backend = Backend(v)
	}
	if v := os.Getenv("RAPIDOCR_MODEL_DIR"); len(v) > 0 {
		config.ModelDir = v
	}
	if v := os.Getenv("RAPIDOCR_LIB_PATH"); len(v) > 0 {
		config.LibraryPath = v
	}
	return config
}
