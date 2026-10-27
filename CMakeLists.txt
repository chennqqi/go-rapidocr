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
if (UNIX AND NOT APPLE)
    # devtoolset gcc on RHEL/CentOS defaults to the gcc4-compatible
    # (pre-C++11) string ABI, but RapidAI's static dep packs and all modern
    # distro opencv/onnxruntime use the new ABI. We ship a fully static
    # libstdc++ anyway, so force the new ABI for a consistent link.
    add_definitions(-D_GLIBCXX_USE_CXX11_ABI=1)
endif ()
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
if (DEFINED OnnxRuntime_DIR OR EXISTS ${CMAKE_CURRENT_SOURCE_DIR}/deps/onnxruntime-static)
    if (NOT DEFINED OnnxRuntime_DIR)
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
    endif ()
    find_package(OnnxRuntime REQUIRED)
    set(ORT_LIBS ${OnnxRuntime_LIBS})
    set(ORT_INCLUDE_DIRS ${OnnxRuntime_INCLUDE_DIRS})
    if (UNIX AND NOT APPLE)
        # Static libonnxruntime.a pulls these system libs.
        list(APPEND ORT_LIBS dl pthread rt)
    endif ()
else ()
    # Shared mode: official Microsoft onnxruntime release (headers + lib).
    # deps/ort-compat remaps the flat release headers to the nested
    # onnxruntime/core/session/ layout upstream sources include.
    if (WIN32)
        file(GLOB ORT_CAND deps/onnxruntime-win-x64-*)
    elseif (APPLE)
        file(GLOB ORT_CAND deps/onnxruntime-osx-*)
    else ()
        file(GLOB ORT_CAND deps/onnxruntime-linux-*)
    endif ()
    list(GET ORT_CAND 0 ORT_DIR)
    if (NOT EXISTS ${ORT_DIR})
        message(FATAL_ERROR "OnnxRuntime not found: place either deps/onnxruntime-static/<plat> or deps/onnxruntime-<plat>-*/")
    endif ()
    set(ORT_INCLUDE_DIRS ${ORT_DIR}/include)
    if (MINGW)
        # MinGW cannot link MSVC import .lib; generate a .dll.a import
        # library. The import records a versioned dll name
        # (onnxruntime-1.20.dll) so the engine never binds to an older
        # onnxruntime.dll already installed on the system (e.g. System32).
        set(ORT_DLL_NAME onnxruntime-1.20.dll)
        set(ORT_IMPLIB ${CMAKE_BINARY_DIR}/libonnxruntime.dll.a)
        if (NOT EXISTS ${ORT_IMPLIB})
            find_program(DLLTOOL dlltool)
            execute_process(COMMAND ${DLLTOOL}
                -l ${ORT_IMPLIB}
                -D ${ORT_DLL_NAME}
                -d ${CMAKE_CURRENT_SOURCE_DIR}/cmake/onnxruntime.def)
        endif ()
        set(ORT_LIBS ${ORT_IMPLIB})
    elseif (APPLE)
        # Link the versioned dylib; @loader_path resolves it beside the
        # engine library at dlopen time.
        file(GLOB ORT_DYLIB ${ORT_DIR}/lib/libonnxruntime.*.dylib)
        list(GET ORT_DYLIB 0 ORT_LIBS)
        set(CMAKE_BUILD_RPATH_USE_ORIGIN ON)
        set(CMAKE_INSTALL_RPATH "@loader_path")
    elseif (UNIX)
        # Link the versioned .so directly; set RPATH so it resolves
        # beside the engine library at runtime.
        file(GLOB ORT_SO ${ORT_DIR}/lib/libonnxruntime.so.*)
        list(GET ORT_SO 0 ORT_SO_REAL)
        set(ORT_LIBS ${ORT_SO_REAL})
        set(CMAKE_BUILD_RPATH_USE_ORIGIN ON)
        set(CMAKE_INSTALL_RPATH "\$ORIGIN")
    else ()
        set(ORT_LIBS ${ORT_DIR}/lib/onnxruntime.lib)
    endif ()
endif ()

# Upstream sources include <onnxruntime/core/session/*.h>, but both the
# official release and RapidAI static packs ship flat headers under include/.
# Generate stub headers mapping the nested names onto the real files.
set(ORT_COMPAT_DIR ${CMAKE_BINARY_DIR}/ort-compat/onnxruntime/core/session)
file(MAKE_DIRECTORY ${ORT_COMPAT_DIR})
file(GLOB_RECURSE ORT_HEADERS ${ORT_INCLUDE_DIRS}/*.h)
foreach (h ${ORT_HEADERS})
    get_filename_component(hn ${h} NAME)
    set(stub ${ORT_COMPAT_DIR}/${hn})
    if (NOT EXISTS ${stub})
        file(WRITE ${stub} "#pragma once\n#include \"${h}\"\n")
    endif ()
endforeach ()
list(PREPEND ORT_INCLUDE_DIRS ${CMAKE_BINARY_DIR}/ort-compat)

# --- OpenCV ---
# Only seed OpenCV_DIR from the prebuilt deps tree when it exists; otherwise
# fall back to the toolchain/system package (e.g. apt libopencv-dev,
# MSYS2 mingw-w64 opencv).
if (NOT DEFINED OpenCV_DIR)
    if (APPLE AND EXISTS ${CMAKE_CURRENT_SOURCE_DIR}/deps/opencv-static/macos)
        set(OpenCV_DIR ${CMAKE_CURRENT_SOURCE_DIR}/deps/opencv-static/macos/lib/cmake/opencv4)
    elseif (WIN32 AND NOT MINGW AND EXISTS ${CMAKE_CURRENT_SOURCE_DIR}/deps/opencv-static/windows-x64)
        set(OpenCV_DIR ${CMAKE_CURRENT_SOURCE_DIR}/deps/opencv-static/windows-x64)
    elseif (UNIX AND EXISTS ${CMAKE_CURRENT_SOURCE_DIR}/deps/opencv-static/linux)
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
        # Upstream OcrLite.cpp uses va_start without <cstdarg>; newer
        # toolchains pull it in transitively, gcc10 does not.
        target_compile_options(${t} PRIVATE -include cstdarg)
        target_include_directories(${t}
            PUBLIC ${CMAKE_CURRENT_SOURCE_DIR}/include
            PRIVATE ${OCR_DIR}/include ${ORT_INCLUDE_DIRS})
        if (OpenCV_VERSION VERSION_GREATER_EQUAL 5)
            # OpenCV 5 moved minAreaRect/getPerspectiveTransform to geometry/2d.hpp.
            target_compile_options(${t} PRIVATE
                -include ${CMAKE_CURRENT_SOURCE_DIR}/include/cv_compat.h)
        endif ()
        target_link_libraries(${t} PRIVATE ${ORT_LIBS} ${OpenCV_LIBS})
        if (UNIX AND NOT APPLE AND DEFINED OnnxRuntime_DIR)
            # Fully self-contained .so: fold the C++ runtime in too so the
            # artifact has no distro package dependencies at all.
            target_link_options(${t} PRIVATE -static-libstdc++ -static-libgcc)
        endif ()
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
