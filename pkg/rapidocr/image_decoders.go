// Registers the image formats DetectBytes can pre-validate. On legacy
// CLIBs (no exception-safe wrapper) undecodable input would crash the
// process across the FFI boundary, so the purego fallback path decodes
// the bytes in Go first and re-encodes to PNG before calling the engine.
package rapidocr

import (
	// stdlib
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	// golang.org/x/image
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)
