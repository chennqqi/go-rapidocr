// go-rapidocr C wrapper.
//
// The upstream OcrLiteCApi.cpp already implements OcrInit / OcrDetectInput /
// OcrFreeResult / OcrDestroy under the __CLIB__ define; this file only adds
// the small go-rapidocr specific extensions declared in include/rapidocr.h.

#include "rapidocr.h"
#include "version.h"

#include <exception>
#include <string>

extern "C" {

const char *rapidocr_version(void) {
    return VERSION;
}

// Thread-local store for the last exception message crossing the C ABI.
// ONNX Runtime/OpenCV throw on bad model files or corrupt inputs; letting a
// C++ exception escape through Go's FFI boundary crashes the process.
static thread_local std::string lastError;

const char *rapidocr_last_error(void) {
    return lastError.empty() ? nullptr : lastError.c_str();
}

OCR_HANDLE rapidocr_init(const char *szDetModel, const char *szClsModel,
                         const char *szRecModel, const char *szKeyPath,
                         int nThreads) {
    try {
        lastError.clear();
        return OcrInit(szDetModel, szClsModel, szRecModel, szKeyPath, nThreads);
    } catch (const std::exception &e) {
        lastError = e.what();
    } catch (...) {
        lastError = "unknown C++ exception";
    }
    return nullptr;
}

OCR_BOOL rapidocr_detect_input(OCR_HANDLE handle, OCR_INPUT *input,
                               OCR_PARAM *pParam, OCR_RESULT *ocrResult) {
    try {
        lastError.clear();
        return OcrDetectInput(handle, input, pParam, ocrResult);
    } catch (const std::exception &e) {
        lastError = e.what();
    } catch (...) {
        lastError = "unknown C++ exception";
    }
    return RAPIDOCR_FALSE;
}

}
