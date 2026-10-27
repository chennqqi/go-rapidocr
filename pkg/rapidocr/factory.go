package rapidocr

import "fmt"

// New creates an Engine with the given config. When Backend is BackendAuto,
// the backend is selected at compile time: cgo when CGO_ENABLED=1 on
// linux/windows/darwin, purego (shared library via purego) otherwise.
func New(config *Config) (Engine, error) {
	if config == nil {
		config = DefaultConfig()
	}

	switch config.Backend {
	case BackendCGO:
		return NewCGO(config)
	case BackendPureGo:
		return NewPureGo(config)
	case BackendAuto:
		return newAuto(config)
	default:
		return nil, fmt.Errorf("unsupported backend: %s", config.Backend)
	}
}

// NewCGO creates a CGO backend engine. Returns an error when CGO is not
// available (CGO_ENABLED=0 or unsupported platform).
func NewCGO(config *Config) (Engine, error) {
	if config == nil {
		config = DefaultConfig()
	}
	if !cgoAvailable {
		return nil, fmt.Errorf("CGO backend is not available (requires CGO_ENABLED=1 and a C toolchain)")
	}
	return newCGOEngine(config)
}

// NewPureGo creates a purego backend engine. It loads the RapidOcrOnnx CLIB
// shared library at runtime; no C toolchain is needed to build the Go code.
func NewPureGo(config *Config) (Engine, error) {
	if config == nil {
		config = DefaultConfig()
	}
	return newPureGoEngine(config)
}

// newAuto selects the backend based on the compile-time cgoAvailable constant.
func newAuto(config *Config) (Engine, error) {
	if cgoAvailable {
		return NewCGO(config)
	}
	return NewPureGo(config)
}
