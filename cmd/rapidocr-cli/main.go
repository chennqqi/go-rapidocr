// rapidocr-cli is a small command line frontend for the rapidocr package.
package main

import (
	"context"
	"flag"
	"fmt"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"time"

	"github.com/chennqqi/go-rapidocr/pkg/rapidocr"
)

func main() {
	var (
		modelDir = flag.String("models", "models", "model directory")
		det      = flag.String("det", "ch_PP-OCRv3_det_infer.onnx", "detection model file")
		cls      = flag.String("cls", "ch_ppocr_mobile_v2.0_cls_infer.onnx", "classifier model file")
		rec      = flag.String("rec", "ch_PP-OCRv3_rec_infer.onnx", "recognition model file")
		keys     = flag.String("keys", "ppocr_keys_v1.txt", "recognition keys file")
		backend  = flag.String("backend", "auto", "backend: auto|cgo|purego")
		libPath  = flag.String("lib", "", "purego backend: path to the RapidOcrOnnx shared library")
		threads  = flag.Int("threads", 4, "onnxruntime thread count")
		maxSide  = flag.Int("max-side-len", 1024, "resize the longest image side to this value (0 = engine default)")
		padding  = flag.Int("padding", 50, "white padding added around the image")
		noAngle  = flag.Bool("no-angle", false, "disable direction classification")
		outImg   = flag.String("draw", "", "optional: write result box image (not implemented)")
	)
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "usage: %s [flags] <image>\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
		os.Exit(2)
	}
	_ = outImg

	cfg := rapidocr.DefaultConfig()
	cfg.Backend = rapidocr.Backend(*backend)
	cfg.ModelDir = *modelDir
	cfg.DetModel = *det
	cfg.ClsModel = *cls
	cfg.RecModel = *rec
	cfg.KeysFile = *keys
	cfg.NumThreads = *threads
	cfg.LibraryPath = *libPath

	engine, err := rapidocr.New(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "init engine:", err)
		os.Exit(1)
	}
	defer engine.Close()
	fmt.Fprintf(os.Stderr, "backend: %s\n", engine.GetBackend())

	imgPath := flag.Arg(0)
	data, err := os.ReadFile(imgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read image:", err)
		os.Exit(1)
	}

	param := &rapidocr.Param{
		Padding:    *padding,
		MaxSideLen: *maxSide,
		DoAngle:    !*noAngle,
		MostAngle:  !*noAngle,
	}

	start := time.Now()
	result, err := engine.DetectBytes(context.Background(), data, param)
	if err != nil {
		fmt.Fprintln(os.Stderr, "detect:", err)
		os.Exit(1)
	}
	elapsed := time.Since(start)

	for i, b := range result.TextBlocks {
		fmt.Printf("[%d] %.3f %s  box=%v\n", i, b.BoxScore, b.Text, b.Box)
	}
	fmt.Fprintf(os.Stderr, "\n%d text blocks, engine=%.1fms (det %.1fms), total=%v\n",
		len(result.TextBlocks), result.DetectTime, result.DBNetTime, elapsed)
}
