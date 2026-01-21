#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Ensure Emscripten is available
if ! command -v emcc &> /dev/null; then
    echo "Emscripten (emcc) not found. Please install and activate emsdk."
    exit 1
fi

# Download FAAD2 if not present
if [ ! -d "faad2" ]; then
    echo "Downloading FAAD2..."
    git clone --depth 1 https://github.com/knik0/faad2.git
fi

# Build FAAD2 with Emscripten using CMake
echo "Building FAAD2 with Emscripten..."
mkdir -p faad2/build-wasm
cd faad2/build-wasm

emcmake cmake .. \
    -DCMAKE_BUILD_TYPE=Release \
    -DBUILD_SHARED_LIBS=OFF \
    -DFAAD_BUILD_CLI=OFF

emmake make -j$(nproc)

cd "$SCRIPT_DIR"

# Compile wrapper to WASM
echo "Compiling wrapper to WASM..."
emcc -O2 \
    --no-entry \
    -s WASM=1 \
    -s EXPORTED_FUNCTIONS='["_faad2_version","_faad2_decoder_create","_faad2_decoder_destroy","_faad2_decoder_init","_faad2_decoder_decode","_faad2_get_error","_malloc","_free"]' \
    -s EXPORTED_RUNTIME_METHODS='[]' \
    -s ALLOW_MEMORY_GROWTH=1 \
    -s INITIAL_MEMORY=16777216 \
    -s STANDALONE_WASM=1 \
    -s ERROR_ON_UNDEFINED_SYMBOLS=0 \
    -I faad2/build-wasm/include \
    -I faad2/include \
    -o faad2.wasm \
    decoder.c \
    faad2/build-wasm/libfaad.a

echo "Built faad2.wasm successfully ($(du -h faad2.wasm | cut -f1))"
