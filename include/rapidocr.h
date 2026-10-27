#ifndef RAPIDOCR_H
#define RAPIDOCR_H

// Pure-C declarations for the RapidOcrOnnx CLIB API.
// The upstream header (include/OcrLiteCApi.h) is wrapped in #ifdef __cplusplus
// and cannot be used from cgo. This header declares the same ABI so that
// both the upstream prebuilt CLIB shared library and our own static wrapper
// library export identical symbols.

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef void *OCR_HANDLE;
typedef char OCR_BOOL;

#ifndef RAPIDOCR_TRUE
#define RAPIDOCR_TRUE 1
#define RAPIDOCR_FALSE 0
#endif

#if defined(_WIN32)
#define RAPIDOCR_API __declspec(dllexport)
#else
#define RAPIDOCR_API __attribute__((visibility("default")))
#endif

// OCR_PARAM mirrors upstream __ocr_param.
typedef struct {
    int padding;
    int maxSideLen;
    float boxScoreThresh;
    float boxThresh;
    float unClipRatio;
    int doAngle;   // 1 means do
    int mostAngle; // 1 means true
} OCR_PARAM;

typedef struct {
    double x;
    double y;
} OCR_POINT;

// OCR_INPUT: type 0 = raw pixels (width/height/channels required),
//            type 1 = encoded image bytes (PNG/JPEG/...).
typedef struct {
    uint8_t *data;
    int type;
    int channels;
    int width;
    int height;
    long dataLength;
} OCR_INPUT;

typedef struct {
    OCR_POINT *boxPoint;
    float boxScore;
    int angleIndex;
    float angleScore;
    double angleTime;
    uint8_t *text;
    float *charScores;
    unsigned long long charScoresLength;
    unsigned long long boxPointLength;
    unsigned long long textLength;
    double crnnTime;
    double blockTime;
} TEXT_BLOCK;

typedef struct {
    double dbNetTime;
    TEXT_BLOCK *textBlocks;
    unsigned long long textBlocksLength;
    double detectTime;
} OCR_RESULT;

OCR_HANDLE OcrInit(const char *szDetModel, const char *szClsModel,
                   const char *szRecModel, const char *szKeyPath, int nThreads);

OCR_BOOL OcrDetect(OCR_HANDLE handle, const char *imgPath, const char *imgName,
                   OCR_PARAM *pParam);

OCR_BOOL OcrDetectInput(OCR_HANDLE handle, OCR_INPUT *input, OCR_PARAM *pParam,
                        OCR_RESULT *ocrResult);

OCR_BOOL OcrFreeResult(OCR_RESULT *result);

int OcrGetLen(OCR_HANDLE handle);

OCR_BOOL OcrGetResult(OCR_HANDLE handle, char *szBuf, int nLen);

void OcrDestroy(OCR_HANDLE handle);

// --- go-rapidocr extensions (not part of the upstream ABI) ---

// rapidocr_version returns the wrapper version string.
RAPIDOCR_API const char *rapidocr_version(void);

// rapidocr_init is OcrInit wrapped in try/catch: on a C++ exception
// (missing/corrupt model files etc.) it returns NULL and stores the
// exception message, which rapidocr_last_error returns.
RAPIDOCR_API OCR_HANDLE rapidocr_init(const char *szDetModel, const char *szClsModel,
                                      const char *szRecModel, const char *szKeyPath,
                                      int nThreads);

// rapidocr_detect_input is OcrDetectInput wrapped in try/catch with the
// same error reporting as rapidocr_init.
RAPIDOCR_API OCR_BOOL rapidocr_detect_input(OCR_HANDLE handle, OCR_INPUT *input,
                                            OCR_PARAM *pParam, OCR_RESULT *ocrResult);

// rapidocr_last_error returns the message recorded by the last failed
// rapidocr_init/rapidocr_detect_input call on this thread, or NULL.
RAPIDOCR_API const char *rapidocr_last_error(void);

#ifdef __cplusplus
}
#endif

#endif // RAPIDOCR_H
