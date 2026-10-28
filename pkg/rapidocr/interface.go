// Package rapidocr provides Go bindings for the RapidOcrOnnx C++ OCR engine.
package rapidocr

import (
	"context"
	"image"
)

// Param mirrors the upstream OCR_PARAM struct. Zero-value fields are replaced
// by the engine defaults (padding=50, maxSideLen=1024, boxScoreThresh=0.6,
// boxThresh=0.3, unClipRatio=2.0, doAngle=1, mostAngle=1).
type Param struct {
	// Padding adds a white border around the image before detection.
	Padding int
	// MaxSideLen limits the longest image side; 0 means engine default.
	MaxSideLen int
	// BoxScoreThresh is the confidence threshold for a text box.
	BoxScoreThresh float32
	// BoxThresh is the binarization threshold of the detection map.
	BoxThresh float32
	// UnClipRatio scales each detected text box.
	UnClipRatio float32
	// DoAngle enables the direction classifier.
	DoAngle bool
	// MostAngle votes for a single dominant angle over the whole image.
	MostAngle bool
}

// TextBlock is one recognized text line with its bounding box.
type TextBlock struct {
	// Box holds the four corner points of the text block.
	Box [4]image.Point
	// BoxScore is the detection confidence of the box.
	BoxScore float32
	// AngleIndex is the direction class index (0 = normal, 1 = rotated 180).
	AngleIndex int
	// AngleScore is the direction classifier confidence.
	AngleScore float32
	// AngleTime is the angle classification time in milliseconds.
	AngleTime float64
	// Text is the recognized UTF-8 text.
	Text string
	// CharScores holds the per-character recognition confidence.
	CharScores []float32
	// CrnnTime is the recognition time in milliseconds.
	CrnnTime float64
	// BlockTime is the total time for this block in milliseconds.
	BlockTime float64
}

// Result is the full OCR output for one image.
type Result struct {
	// DBNetTime is the detection stage time in milliseconds.
	DBNetTime float64
	// TextBlocks holds all recognized text blocks.
	TextBlocks []TextBlock
	// DetectTime is the whole pipeline time in milliseconds.
	DetectTime float64
}

// String concatenates all text blocks with newlines.
func (r *Result) String() string {
	if r == nil {
		return ""
	}
	var s string
	for i, b := range r.TextBlocks {
		if i > 0 {
			s += "\n"
		}
		s += b.Text
	}
	return s
}

// Engine is the OCR engine interface implemented by all backends.
//
// The context is honoured before and after the native inference call:
// a cancelled context returns ctx.Err() without running OCR, and a context
// cancelled during inference is reported once the call returns. The C++
// inference itself cannot be aborted mid-flight.
//
// A native failure (corrupt image data, engine errors) is returned as an
// error — an empty *Result with nil error means "no text found" only.
type Engine interface {
	// Detect runs OCR on an image.Image.
	Detect(ctx context.Context, img image.Image, p *Param) (*Result, error)
	// DetectBytes runs OCR on an encoded image (PNG/JPEG/BMP...).
	DetectBytes(ctx context.Context, data []byte, p *Param) (*Result, error)
	// DetectPixels runs OCR on tightly packed raw pixels.
	// channels must be 1 (gray), 3 (RGB) or 4 (RGBA).
	DetectPixels(ctx context.Context, data []byte, width, height, channels int, p *Param) (*Result, error)
	// Close releases all engine resources.
	Close() error
	// GetBackend returns the backend type in use.
	GetBackend() Backend
}
