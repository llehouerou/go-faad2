package faad2

import (
	"context"
	"sync"
)

// Decoder represents an AAC decoder instance.
type Decoder struct {
	mu          sync.Mutex
	wctx        *wasmContext
	decoderPtr  uint32
	initialized bool
	sampleRate  uint32
	channels    uint8
}

// NewDecoder creates a new AAC decoder.
func NewDecoder() (*Decoder, error) {
	wctx, err := getWasmContext()
	if err != nil {
		return nil, err
	}

	results, err := wctx.fnCreate.Call(context.Background())
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

// Init initializes the decoder with AAC codec configuration
// (typically from MP4 esds box or ADTS header).
func (d *Decoder) Init(config []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(config) == 0 {
		return ErrInvalidConfig
	}

	// Allocate memory for config
	configPtr, err := d.wctx.malloc(uint32(len(config))) //nolint:gosec // config is small (AAC spec)
	if err != nil {
		return err
	}
	defer d.wctx.free(configPtr)

	if !d.wctx.write(configPtr, config) {
		return ErrOutOfMemory
	}

	// Allocate memory for output parameters
	sampleRatePtr, err := d.wctx.malloc(8) // unsigned long
	if err != nil {
		return err
	}
	defer d.wctx.free(sampleRatePtr)

	channelsPtr, err := d.wctx.malloc(1) // unsigned char
	if err != nil {
		return err
	}
	defer d.wctx.free(channelsPtr)

	results, err := d.wctx.fnInit.Call(context.Background(),
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

// Decode decodes a single AAC frame and returns PCM samples.
func (d *Decoder) Decode(aacFrame []byte) ([]int16, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.initialized {
		return nil, ErrNotInitialized
	}

	// Allocate input buffer
	inputPtr, err := d.wctx.malloc(uint32(len(aacFrame))) //nolint:gosec // frame size is bounded by AAC spec
	if err != nil {
		return nil, err
	}
	defer d.wctx.free(inputPtr)

	if !d.wctx.write(inputPtr, aacFrame) {
		return nil, ErrOutOfMemory
	}

	// Allocate output buffer (max samples per frame: 2048 * channels * 2 bytes)
	maxSamples := 2048 * int(d.channels)
	outputPtr, err := d.wctx.malloc(uint32(maxSamples * 2)) //nolint:gosec // bounded by AAC frame size
	if err != nil {
		return nil, err
	}
	defer d.wctx.free(outputPtr)

	// Decode
	results, err := d.wctx.fnDecode.Call(context.Background(),
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
		pcm[i] = int16(pcmBytes[i*2]) | int16(pcmBytes[i*2+1])<<8
	}

	return pcm, nil
}

// SampleRate returns the sample rate after initialization.
func (d *Decoder) SampleRate() uint32 {
	return d.sampleRate
}

// Channels returns the number of channels after initialization.
func (d *Decoder) Channels() uint8 {
	return d.channels
}

// Close releases decoder resources.
func (d *Decoder) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.decoderPtr != 0 {
		_, _ = d.wctx.fnDestroy.Call(context.Background(), uint64(d.decoderPtr))
		d.decoderPtr = 0
	}

	return nil
}
