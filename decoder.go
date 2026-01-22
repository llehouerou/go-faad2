// Package faad2 provides AAC audio decoding using the FAAD2 library compiled to WebAssembly.
//
// The package supports decoding AAC audio from:
//   - M4A/MP4 container files via [OpenM4A]
//   - Raw ADTS streams via [OpenADTS]
//   - Direct frame decoding via [Decoder]
//
// Basic usage with M4A files:
//
//	reader, err := faad2.OpenM4A(ctx, file)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer reader.Close(ctx)
//
//	pcm := make([]int16, 4096)
//	for {
//	    n, err := reader.Read(ctx, pcm)
//	    if err != nil {
//	        break
//	    }
//	    // Process pcm[:n] samples...
//	}
//
// The package uses a global WASM runtime that is lazily initialized on first use.
// Call [Shutdown] to release WASM resources when done.
package faad2

import (
	"context"
	"sync"
)

// Decoder is a low-level AAC decoder that decodes individual AAC frames.
//
// For most use cases, prefer [OpenM4A] or [OpenADTS] which handle container
// parsing and provide a simpler streaming interface.
//
// A Decoder must be initialized with [Decoder.Init] before calling [Decoder.Decode].
// The decoder is safe for concurrent use after initialization.
type Decoder struct {
	mu          sync.Mutex
	wctx        *wasmContext
	decoderPtr  uint32
	initialized bool
	closed      bool
	sampleRate  uint32
	channels    uint8
}

// NewDecoder creates a new AAC decoder instance.
//
// The decoder must be initialized with [Decoder.Init] before use.
// Call [Decoder.Close] when done to release resources.
func NewDecoder(ctx context.Context) (*Decoder, error) {
	wctx, err := getWasmContext(ctx)
	if err != nil {
		return nil, err
	}

	results, err := wctx.fnCreate.Call(ctx)
	if err != nil {
		return nil, err
	}

	ptr := uint32(results[0]) //nolint:gosec // WASM pointers are 32-bit
	if ptr == 0 {
		return nil, ErrOutOfMemory
	}

	return &Decoder{
		wctx:       wctx,
		decoderPtr: ptr,
	}, nil
}

// Init initializes the decoder with an AudioSpecificConfig.
//
// The config parameter is the AAC AudioSpecificConfig, typically extracted from:
//   - The esds box in M4A/MP4 files
//   - ADTS frame headers (converted via internal helper)
//
// Init must be called exactly once before [Decoder.Decode].
// Returns [ErrInvalidConfig] if the configuration is nil, empty, or invalid.
func (d *Decoder) Init(ctx context.Context, config []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return ErrDecoderClosed
	}

	if len(config) == 0 {
		return ErrInvalidConfig
	}

	// Allocate memory for config
	configPtr, err := d.wctx.malloc(ctx, uint32(len(config))) //nolint:gosec // config is small (AAC spec)
	if err != nil {
		return err
	}
	defer d.wctx.free(ctx, configPtr)

	if !d.wctx.write(configPtr, config) {
		return ErrOutOfMemory
	}

	// Allocate memory for output parameters
	sampleRatePtr, err := d.wctx.malloc(ctx, 8) // unsigned long
	if err != nil {
		return err
	}
	defer d.wctx.free(ctx, sampleRatePtr)

	channelsPtr, err := d.wctx.malloc(ctx, 1) // unsigned char
	if err != nil {
		return err
	}
	defer d.wctx.free(ctx, channelsPtr)

	results, err := d.wctx.fnInit.Call(ctx,
		uint64(d.decoderPtr),
		uint64(configPtr),
		uint64(len(config)),
		uint64(sampleRatePtr),
		uint64(channelsPtr),
	)
	if err != nil {
		return err
	}

	if int32(results[0]) < 0 { //nolint:gosec // WASM returns signed status
		return ErrInvalidConfig
	}

	// Read sample rate and channels
	srData, ok := d.wctx.read(sampleRatePtr, 4)
	if !ok {
		return ErrOutOfMemory
	}
	chData, ok := d.wctx.read(channelsPtr, 1)
	if !ok {
		return ErrOutOfMemory
	}

	d.sampleRate = uint32(srData[0]) | uint32(srData[1])<<8 | uint32(srData[2])<<16 | uint32(srData[3])<<24
	d.channels = chData[0]
	d.initialized = true

	return nil
}

// Decode decodes a single AAC frame and returns interleaved PCM samples.
//
// The returned slice contains 16-bit signed PCM samples. For stereo audio,
// samples are interleaved (L, R, L, R, ...). The number of samples per frame
// is typically 1024 or 2048 per channel, depending on the AAC profile.
//
// Returns [ErrNotInitialized] if [Decoder.Init] has not been called,
// [ErrEmptyFrame] if aacFrame is empty, or [ErrDecodeFailed] on decode error.
func (d *Decoder) Decode(ctx context.Context, aacFrame []byte) ([]int16, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil, ErrDecoderClosed
	}

	if !d.initialized {
		return nil, ErrNotInitialized
	}

	if len(aacFrame) == 0 {
		return nil, ErrEmptyFrame
	}

	if d.channels == 0 {
		return nil, ErrInvalidConfig
	}

	// Allocate input buffer
	inputPtr, err := d.wctx.malloc(ctx, uint32(len(aacFrame))) //nolint:gosec // frame size is bounded by AAC spec
	if err != nil {
		return nil, err
	}
	defer d.wctx.free(ctx, inputPtr)

	if !d.wctx.write(inputPtr, aacFrame) {
		return nil, ErrOutOfMemory
	}

	// Allocate output buffer (max samples per frame: 2048 * channels * 2 bytes)
	maxSamples := 2048 * int(d.channels)
	outputPtr, err := d.wctx.malloc(ctx, uint32(maxSamples*2)) //nolint:gosec // bounded by AAC frame size
	if err != nil {
		return nil, err
	}
	defer d.wctx.free(ctx, outputPtr)

	// Decode
	results, err := d.wctx.fnDecode.Call(ctx,
		uint64(d.decoderPtr),
		uint64(inputPtr),
		uint64(len(aacFrame)),
		uint64(outputPtr),
		uint64(maxSamples*2), //nolint:gosec // bounded by AAC frame size
	)
	if err != nil {
		return nil, err
	}

	numSamples := int32(results[0]) //nolint:gosec // WASM returns signed sample count
	if numSamples < 0 {
		return nil, ErrDecodeFailed
	}

	// Read PCM output
	pcmBytes, ok := d.wctx.read(outputPtr, uint32(numSamples*2)) //nolint:gosec // bounded by AAC frame size
	if !ok {
		return nil, ErrOutOfMemory
	}

	pcm := make([]int16, numSamples)
	for i := range pcm {
		// Build uint16 from little-endian bytes, then reinterpret as int16
		pcm[i] = int16(uint16(pcmBytes[i*2]) | uint16(pcmBytes[i*2+1])<<8) //nolint:gosec // intentional bit reinterpretation
	}

	return pcm, nil
}

// SampleRate returns the audio sample rate in Hz (e.g., 44100, 48000).
//
// Returns 0 if the decoder has not been initialized.
func (d *Decoder) SampleRate() uint32 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.sampleRate
}

// Channels returns the number of audio channels (1 for mono, 2 for stereo).
//
// Returns 0 if the decoder has not been initialized.
func (d *Decoder) Channels() uint8 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.channels
}

// Close releases decoder resources.
//
// After Close is called, the decoder cannot be reused.
// It is safe to call Close multiple times; subsequent calls are no-ops.
func (d *Decoder) Close(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil
	}

	if d.decoderPtr != 0 {
		_, _ = d.wctx.fnDestroy.Call(ctx, uint64(d.decoderPtr))
		d.decoderPtr = 0
	}

	d.closed = true
	return nil
}
