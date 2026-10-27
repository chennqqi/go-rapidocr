// go-rapidocr C wrapper.
//
// The upstream OcrLiteCApi.cpp already implements OcrInit / OcrDetectInput /
// OcrFreeResult / OcrDestroy under the __CLIB__ define; this file only adds
// the small go-rapidocr specific extensions declared in include/rapidocr.h.

#include "rapidocr.h"
#include "version.h"

extern "C" {

const char *rapidocr_version(void) {
    return VERSION;
}

}
