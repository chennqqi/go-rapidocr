//go:build !(linux || windows || darwin) || js

package rapidocr

import "fmt"

// newPureGoEngine is unavailable on platforms without purego support.
func newPureGoEngine(config *Config) (Engine, error) {
	return nil, fmt.Errorf("purego backend is not available on this platform")
}
