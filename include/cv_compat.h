// Compatibility shim for OpenCV >= 5: geometric functions (minAreaRect,
// getPerspectiveTransform, ...) moved from imgproc to geometry/2d.hpp.
// Force-included into upstream sources via -include when building against
// OpenCV 5.x; harmless on OpenCV 4.x where the header does not exist (guarded
// by __has_include).
#pragma once

#if defined(__has_include)
#if __has_include(<opencv2/geometry/2d.hpp>)
#include <opencv2/geometry/2d.hpp>
#endif
#endif
