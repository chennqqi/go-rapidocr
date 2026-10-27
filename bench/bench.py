#!/usr/bin/env python3
"""Python-side benchmark for the go-rapidocr comparison.

Uses the same PP-OCRv4 mobile models, keys file, image and parameters as
the C++ and Go benchmarks:
    padding=50, maxSideLen=1024, unClipRatio=2.0, threads=4, doAngle=1

    python bench/bench.py [image] [loops]
"""
import sys
import time

IMG = sys.argv[1] if len(sys.argv) > 1 else "testdata/1.jpg"
LOOPS = int(sys.argv[2]) if len(sys.argv) > 2 else 10

from rapidocr_onnxruntime import RapidOCR

t0 = time.perf_counter()
ocr = RapidOCR(
    det_model_path="models/ch_PP-OCRv4_det_mobile.onnx",
    cls_model_path="models/ch_ppocr_mobile_v2.0_cls_mobile.onnx",
    rec_model_path="models/ch_PP-OCRv4_rec_mobile.onnx",
    rec_keys_path="models/ppocr_keys_v1.txt",
    intra_op_num_threads=4,
    inter_op_num_threads=1,
    unclip_ratio=2.0,
    det_limit_side_len=1024,
    det_db_box_thresh=0.3,
    det_db_thresh=0.3,
)
init_ms = (time.perf_counter() - t0) * 1000

# warmup
for _ in range(2):
    ocr(IMG)

texts, det_t, cls_t, rec_t = None, 0.0, 0.0, 0.0
t0 = time.perf_counter()
for _ in range(LOOPS):
    texts, elapse = ocr(IMG)
    if isinstance(elapse, dict):
        det_t += elapse.get("det", 0.0)
        cls_t += elapse.get("cls", 0.0)
        rec_t += elapse.get("rec", 0.0)
    elif elapse:
        e = list(elapse)
        det_t += e[0] or 0.0
        cls_t += e[1] or 0.0
        rec_t += e[2] or 0.0
wall_ms = (time.perf_counter() - t0) * 1000

for i, blk in enumerate(texts or []):
    # blk = [box, text, score]
    print(f"[{i}] {float(blk[2]):.3f} {blk[1]}")

n = max(LOOPS, 1)
print(
    f"\n{len(texts or [])} text blocks, init={init_ms:.0f}ms, "
    f"avg det={det_t / n * 1000:.1f}ms cls={cls_t / n * 1000:.1f}ms "
    f"rec={rec_t / n * 1000:.1f}ms wall={wall_ms / n:.0f}ms over {LOOPS} run(s)",
    file=sys.stderr,
)
