#!/bin/bash
set -e

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

# Build FAAD2 with Emscripten
cd faad2
if [ ! -f "configure" ]; then
    autoreconf -i
fi
emconfigure ./configure --disable-shared --enable-static
emmake make clean
emmake make
cd ..

# Compile wrapper to WASM
emcc -O2 \
    -s WASM=1 \
    -s EXPORTED_FUNCTIONS='["_faad2_version","_faad2_decoder_create","_faad2_decoder_destroy","_faad2_decoder_init","_faad2_decoder_decode","_faad2_get_error","_malloc","_free"]' \
    -s EXPORTED_RUNTIME_METHODS='[]' \
    -s ALLOW_MEMORY_GROWTH=1 \
    -s TOTAL_MEMORY=16777216 \
    -s STANDALONE_WASM=1 \
    -I faad2/include \
    -o faad2.wasm \
    decoder.c \
    faad2/libfaad/.libs/libfaad.a

echo "Built faad2.wasm successfully"
