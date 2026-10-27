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
option(BUILD_SHARED_LIB "Build shared CLIB (e.g. MinGW-built dll for purego)" OFF)
if (BUILD_SHARED_LIB)
    set(BUILD_SHARED_LIBS ON)
else ()
    set(BUILD_SHARED_LIBS OFF CACHE BOOL "" FORCE)
endif ()

set(OCR_DIR ${CMAKE_CURRENT_SOURCE_DIR}/rapidocr-onnx)

# --- OnnxRuntime ---
# Two modes:
#   1. Static: deps/onnxruntime-static/<plat> (RapidAI/OnnxruntimeBuilder layout)
#   2. Shared: deps/onnxruntime-<plat> with include/ + onnxruntime.dll
if (EXISTS ${CMAKE_CURRENT_SOURCE_DIR}/deps/onnxruntime-static)
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
    set(ORT_LIBS ${OnnxRuntime_LIBS})
    set(ORT_INCLUDE_DIRS ${OnnxRuntime_INCLUDE_DIRS})
elseif (EXISTS ${CMAKE_CURRENT_SOURCE_DIR}/deps/onnxruntime-win-x64-1.20.1)
    # Shared mode: official Microsoft onnxruntime release (headers + dll).
    # deps/ort-compat remaps the flat release headers to the nested
    # onnxruntime/core/session/ layout upstream sources include.
    set(ORT_DIR ${CMAKE_CURRENT_SOURCE_DIR}/deps/onnxruntime-win-x64-1.20.1)
    set(ORT_INCLUDE_DIRS ${CMAKE_CURRENT_SOURCE_DIR}/deps/ort-compat)
    if (MINGW)
        # MinGW cannot link MSVC import .lib; generate a .dll.a import library.
        find_program(DLLTOOL dlltool HINTS ${CMAKE_CXX_COMPILER_ID} PATHS ENV PATH)
        if (NOT DLLTOOL)
            set(DLLTOOL dlltool)
        endif ()
        set(ORT_IMPLIB ${CMAKE_BINARY_DIR}/libonnxruntime.dll.a)
        if (NOT EXISTS ${ORT_IMPLIB})
            execute_process(COMMAND ${DLLTOOL}
                -l ${ORT_IMPLIB}
                -D ${ORT_DIR}/lib/onnxruntime.dll
                -d ${CMAKE_CURRENT_SOURCE_DIR}/cmake/onnxruntime.def)
        endif ()
        set(ORT_LIBS ${ORT_IMPLIB})
    else ()
        set(ORT_LIBS ${ORT_DIR}/lib/onnxruntime.lib)
    endif ()
else ()
    message(FATAL_ERROR "OnnxRuntime not found: place either deps/onnxruntime-static/<plat> or deps/onnxruntime-win-x64-*/")
endif ()

# --- OpenCV ---
if (NOT DEFINED OpenCV_DIR)
    if (APPLE)
        set(OpenCV_DIR ${CMAKE_CURRENT_SOURCE_DIR}/deps/opencv-static/macos/lib/cmake/opencv4)
    elseif (WIN32 AND NOT MINGW)
        set(OpenCV_DIR ${CMAKE_CURRENT_SOURCE_DIR}/deps/opencv-static/windows-x64)
    elseif (UNIX)
        set(OpenCV_DIR ${CMAKE_CURRENT_SOURCE_DIR}/deps/opencv-static/linux/lib/cmake/opencv4)
    endif ()
endif ()
find_package(OpenCV REQUIRED)

# --- Sources: upstream engine + our wrapper. Files guarded by
# __JNI__/__EXEC__ (main.cpp, getopt.cpp, OcrLiteJni.cpp) compile to nothing
# under __CLIB__, so glob the whole directory. ---
file(GLOB OCR_SRC ${OCR_DIR}/src/*.cpp src/rapidocr.cpp)
list(REMOVE_ITEM OCR_SRC ${OCR_DIR}/src/OcrResultUtils.cpp) # JNI-only

if (BUILD_SHARED_LIB)
    add_library(RapidOcrOnnx SHARED ${OCR_SRC})
else ()
    add_library(rapidocr STATIC ${OCR_SRC})
endif ()
foreach (t rapidocr RapidOcrOnnx)
    if (TARGET ${t})
        target_compile_definitions(${t} PRIVATE __CLIB__)
        target_include_directories(${t}
            PUBLIC ${CMAKE_CURRENT_SOURCE_DIR}/include
            PRIVATE ${OCR_DIR}/include ${ORT_INCLUDE_DIRS})
        if (OpenCV_VERSION VERSION_GREATER_EQUAL 5)
            # OpenCV 5 moved minAreaRect/getPerspectiveTransform to geometry/2d.hpp.
            target_compile_options(${t} PRIVATE
                -include ${CMAKE_CURRENT_SOURCE_DIR}/include/cv_compat.h)
        endif ()
        target_link_libraries(${t} PRIVATE ${ORT_LIBS} ${OpenCV_LIBS})
    endif ()
endforeach ()

set(INSTALL_TARGETS)
if (TARGET rapidocr)
    list(APPEND INSTALL_TARGETS rapidocr)
endif ()
if (TARGET RapidOcrOnnx)
    list(APPEND INSTALL_TARGETS RapidOcrOnnx)
endif ()
install(TARGETS ${INSTALL_TARGETS}
    ARCHIVE DESTINATION lib
    LIBRARY DESTINATION lib
    RUNTIME DESTINATION bin)
install(FILES include/rapidocr.h DESTINATION include)
