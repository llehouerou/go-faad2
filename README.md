# go-faad2

Pure Go AAC audio decoder using FAAD2 compiled to WebAssembly.

## Features

- **Pure Go** - No CGO dependencies, cross-compiles easily
- **M4A/MP4 support** - Built-in demuxer for M4A files
- **Low-level API** - Decode raw AAC frames directly

## Installation

```bash
go get github.com/llehouerou/go-faad2
```

## Usage

### Decode M4A file (high-level)

```go
file, _ := os.Open("audio.m4a")
defer file.Close()

reader, _ := faad2.OpenM4A(file)
defer reader.Close()

fmt.Printf("Sample rate: %d, Channels: %d\n", reader.SampleRate(), reader.Channels())

pcm := make([]int16, 4096)
for {
    n, err := reader.Read(pcm)
    if err == io.EOF {
        break
    }
    // Process pcm[:n]
}
```

### Decode raw AAC frames (low-level)

```go
decoder, _ := faad2.NewDecoder()
defer decoder.Close()

// Initialize with AAC codec config (from ADTS or MP4 esds)
decoder.Init(codecConfig)

// Decode frames
pcm, _ := decoder.Decode(aacFrame)
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
- [go-mp4](https://github.com/abema/go-mp4) - MP4 parser
