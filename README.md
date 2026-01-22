# go-faad2

Pure Go AAC audio decoder using FAAD2 compiled to WebAssembly.

## Features

- **Pure Go** - No CGO dependencies, cross-compiles easily
- **ADTS support** - Decode raw AAC streams (ADTS format)
- **Low-level API** - Decode raw AAC frames directly

## Installation

```bash
go get github.com/llehouerou/go-faad2
```

## Usage

### Decode ADTS stream (raw AAC)

```go
ctx := context.Background()

file, _ := os.Open("audio.aac")
reader, _ := faad2.OpenADTS(ctx, file)
defer reader.Close(ctx)

pcm := make([]int16, 4096)
for {
    n, err := reader.Read(ctx, pcm)
    if err == io.EOF {
        break
    }
    // Process pcm[:n]
}
```

### Decode raw AAC frames (low-level)

```go
ctx := context.Background()

decoder, _ := faad2.NewDecoder(ctx)
defer decoder.Close(ctx)

// Initialize with AAC codec config (AudioSpecificConfig)
decoder.Init(ctx, codecConfig)

// Decode frames
pcm, _ := decoder.Decode(ctx, aacFrame)
```

## Building the WASM binary

The WASM binary is pre-built and embedded in the library. To rebuild it:

```bash
# Requires Emscripten (available in nix devShell)
make wasm
```

## Development

```bash
# Enter development shell (requires Nix with flakes)
nix develop

# Run checks (format, lint, test)
make check

# Install git hooks
make install-hooks
```

## License

GPL-2.0-or-later (required by FAAD2)

## Credits

- [FAAD2](https://github.com/knik0/faad2) - AAC decoder
- [wazero](https://github.com/tetratelabs/wazero) - WebAssembly runtime
