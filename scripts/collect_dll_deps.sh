#!/usr/bin/env bash
# Recursively copies a dll's MinGW runtime dependencies into an output dir.
#
# Usage: collect_dll_deps.sh <seed-dll> <out-dir> [mingw-bin-dir]
#
# Only dlls present under the MinGW bin dir are followed; system dlls
# (kernel32, msvcrt, api-ms-*, ...) are skipped by the presence check.
set -euo pipefail

SEED=$1
OUT=$2
# Resolve the MinGW bin dir from the toolchain itself — the msys2 install
# root differs between machines (/c/msys64 locally, D:\a\_temp\msys64 or
# /ucrt64 on CI), so never hard-code it.
BIN=${3:-$(dirname "$(command -v gcc || command -v objdump)")}
mkdir -p "$OUT"

declare -A seen
frontier=("$SEED")
while ((${#frontier[@]})); do
    next=()
    for f in "${frontier[@]}"; do
        [ -f "$f" ] || continue
        while read -r dep; do
            [ -n "${seen[$dep]:-}" ] && continue
            seen[$dep]=1
            src="$BIN/$dep"
            [ ! -f "$src" ] && src="$(dirname "$f")/$dep"
            if [ -f "$src" ]; then
                cp -n "$src" "$OUT/" 2>/dev/null || cp "$src" "$OUT/"
                next+=("$src")
            fi
        done < <(objdump -p "$f" | awk '/DLL Name:/{print $3}')
    done
    frontier=("${next[@]}")
done
