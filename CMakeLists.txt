cmake_minimum_required(VERSION 3.17)
project(go-rapidocr)

# Builds librapidocr: the upstream RapidOcrOnnx sources (CLIB mode) plus the
# go-rapidocr extension wrapper, as a static library for cgo linking.
#
# Dependencies (download once into deps/ via scripts/fetch_deps.sh):
#   deps/onnxruntime-static/{linux,macos,windows-x64,...}   (RapidAI/OnnxruntimeBuilder)
#   deps/opencv-static/{linux,macos,windows-x64,...}        (RapidAI/OpenCVBuilder)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
option(BUILD_STATIC_LIB "Build static library" ON)
set(BUILD_SHARED_LIBS OFF CACHE BOOL "" FORCE)

set(OCR_DIR ${CMAKE_CURRENT_SOURCE_DIR}/rapidocr-onnx)

# --- OnnxRuntime (static) ---
if (APPLE)
    set(OnnxRuntime_DIR ${CMAKE_CURRENT_SOURCE_DIR}/deps/onnxruntime-static/macos)
elseif (WIN32)
    set(OnnxRuntime_DIR ${CMAKE_CURRENT_SOURCE_DIR}/deps/onnxruntime-static/windows-x64)
elseif (UNIX)
    if (CMAKE_SYSTEM_PROCESSOR MATCHES "aarch64|arm64")
        set(OnnxRuntime_DIR ${CMAKE_CURRENT_SOURCE_DIR}/deps/onnxruntime-static/linux-arm64)
    else ()
        set(OnnxRuntime_DIR ${CMAKE_CURRENT_SOURCE_DIR}/deps/onnxruntime-static/linux)
    endif ()
endif ()
find_package(OnnxRuntime REQUIRED)

# --- OpenCV (static) ---
if (APPLE)
    set(OpenCV_DIR ${CMAKE_CURRENT_SOURCE_DIR}/deps/opencv-static/macos/lib/cmake/opencv4)
elseif (WIN32)
    set(OpenCV_DIR ${CMAKE_CURRENT_SOURCE_DIR}/deps/opencv-static/windows-x64)
elseif (UNIX)
    set(OpenCV_DIR ${CMAKE_CURRENT_SOURCE_DIR}/deps/opencv-static/linux/lib/cmake/opencv4)
endif ()
find_package(OpenCV REQUIRED)

# --- Sources: upstream engine (minus main/jni/benchmark) + our wrapper ---
set(OCR_SRC
    ${OCR_DIR}/src/AngleNet.cpp
    ${OCR_DIR}/src/clipper.cpp
    ${OCR_DIR}/src/CrnnNet.cpp
    ${OCR_DIR}/src/DbNet.cpp
    ${OCR_DIR}/src/OcrLite.cpp
    ${OCR_DIR}/src/OcrLiteCApi.cpp
    ${OCR_DIR}/src/OcrLiteImpl.cpp
    ${OCR_DIR}/src/OcrUtils.cpp
    src/rapidocr.cpp)

add_library(rapidocr STATIC ${OCR_SRC})
target_compile_definitions(rapidocr PRIVATE __CLIB__)
target_include_directories(rapidocr
    PUBLIC ${CMAKE_CURRENT_SOURCE_DIR}/include
    PRIVATE ${OCR_DIR}/include)
target_link_libraries(rapidocr PRIVATE ${OnnxRuntime_LIBS} ${OpenCV_LIBS})

install(TARGETS rapidocr ARCHIVE DESTINATION lib)
install(FILES include/rapidocr.h DESTINATION include)
