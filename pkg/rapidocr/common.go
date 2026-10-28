package rapidocr

import (
	"context"
	"image"
	"image/draw"
	"path/filepath"
)

// checkCtx returns the context error if it is already done. The native
// inference call itself cannot be interrupted mid-flight, so Detect*
// checks this before entering the FFI call (fail fast) and again after it
// returns (surface a cancellation that happened during inference).
func checkCtx(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

// modelPaths resolves configured model file names against ModelDir.
func (c *Config) modelPaths() (det, cls, rec, keys string) {
	return filepath.Join(c.ModelDir, c.DetModel),
		filepath.Join(c.ModelDir, c.ClsModel),
		filepath.Join(c.ModelDir, c.RecModel),
		filepath.Join(c.ModelDir, c.KeysFile)
}

// imageToRGB converts any image.Image to tightly packed RGB bytes.
// The C++ engine converts them to BGR internally.
func imageToRGB(img image.Image) ([]byte, int, int) {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	rgb, ok := img.(*image.RGBA)
	if !ok || bounds.Min.X != 0 || bounds.Min.Y != 0 {
		rgb = image.NewRGBA(image.Rect(0, 0, w, h))
		draw.Draw(rgb, rgb.Bounds(), img, bounds.Min, draw.Src)
	}
	data := make([]byte, w*h*3)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			src := y*rgb.Stride + x*4
			dst := (y*w + x) * 3
			data[dst] = rgb.Pix[src]
			data[dst+1] = rgb.Pix[src+1]
			data[dst+2] = rgb.Pix[src+2]
		}
	}
	return data, w, h
}
