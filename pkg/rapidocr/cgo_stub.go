//go:build !cgo || !(linux || windows || darwin) || rapidocr_nocgo

package rapidocr

import "fmt"

// cgoAvailable reports whether the CGO backend was compiled in.
const cgoAvailable = false

// newCGOEngine is unavailable without cgo or when excluded via the
// rapidocr_nocgo build tag.
func newCGOEngine(config *Config) (Engine, error) {
	return nil, fmt.Errorf("CGO backend is not available (requires CGO_ENABLED=1, a C toolchain, and no -tags rapidocr_nocgo)")
}
