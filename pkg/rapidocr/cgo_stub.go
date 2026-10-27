//go:build !cgo || !(linux || windows || darwin)

package rapidocr

import "fmt"

// cgoAvailable reports whether the CGO backend was compiled in.
const cgoAvailable = false

// newCGOEngine is unavailable without cgo.
func newCGOEngine(config *Config) (Engine, error) {
	return nil, fmt.Errorf("CGO backend is not available (requires CGO_ENABLED=1 and a C toolchain)")
}
