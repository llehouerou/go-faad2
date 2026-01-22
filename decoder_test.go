package faad2

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/abema/go-mp4"
)

func TestNewDecoder(t *testing.T) {
	ctx := context.Background()
	dec, err := NewDecoder(ctx)
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}
	defer dec.Close(ctx)

	if dec.decoderPtr == 0 {
		t.Error("decoder pointer is nil")
	}
}

func TestDecoderInitWithoutConfig(t *testing.T) {
	ctx := context.Background()
	dec, err := NewDecoder(ctx)
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}
	defer dec.Close(ctx)

	err = dec.Init(ctx, nil)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}

	err = dec.Init(ctx, []byte{})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for empty config, got %v", err)
	}
}

func TestDecoderDecodeWithoutInit(t *testing.T) {
	ctx := context.Background()
	dec, err := NewDecoder(ctx)
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}
	defer dec.Close(ctx)

	_, err = dec.Decode(ctx, []byte{0x00, 0x01, 0x02})
	if !errors.Is(err, ErrNotInitialized) {
		t.Errorf("expected ErrNotInitialized, got %v", err)
	}
}

func TestDecoderWithM4AFile(t *testing.T) {
	ctx := context.Background()
	testFile := "testdata/mono_44100.m4a"
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("test file not found, run 'make testdata' first")
	}

	// Extract AAC config and samples from M4A file
	config, samples, err := extractAACFromM4A(testFile)
	if err != nil {
		t.Fatalf("failed to extract AAC from M4A: %v", err)
	}

	if len(config) == 0 {
		t.Fatal("empty AAC config")
	}

	if len(samples) == 0 {
		t.Fatal("no AAC samples found")
	}

	t.Logf("AAC config: %d bytes, samples: %d", len(config), len(samples))

	// Create and init decoder
	dec, err := NewDecoder(ctx)
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}
	defer dec.Close(ctx)

	err = dec.Init(ctx, config)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	t.Logf("Decoder initialized: sample rate=%d, channels=%d", dec.SampleRate(), dec.Channels())

	if dec.SampleRate() == 0 {
		t.Error("sample rate is 0")
	}
	if dec.Channels() == 0 {
		t.Error("channels is 0")
	}

	// Decode first few frames
	totalSamples := 0
	for i, sample := range samples {
		if i >= 5 { // Decode first 5 frames
			break
		}

		pcm, err := dec.Decode(ctx, sample)
		if err != nil {
			t.Errorf("Decode frame %d failed: %v", i, err)
			continue
		}

		totalSamples += len(pcm)
		t.Logf("Frame %d: %d PCM samples", i, len(pcm))
	}

	if totalSamples == 0 {
		t.Error("no PCM samples decoded")
	}

	t.Logf("Total PCM samples decoded: %d", totalSamples)
}

func TestDecoderStereo(t *testing.T) {
	ctx := context.Background()
	testFile := "testdata/stereo_48000.m4a"
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("test file not found, run 'make testdata' first")
	}

	config, samples, err := extractAACFromM4A(testFile)
	if err != nil {
		t.Fatalf("failed to extract AAC from M4A: %v", err)
	}

	dec, err := NewDecoder(ctx)
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}
	defer dec.Close(ctx)

	err = dec.Init(ctx, config)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if dec.SampleRate() != 48000 {
		t.Errorf("expected sample rate 48000, got %d", dec.SampleRate())
	}
	if dec.Channels() != 2 {
		t.Errorf("expected 2 channels, got %d", dec.Channels())
	}

	// Decode frames (first frame is typically a priming frame with 0 samples)
	totalSamples := 0
	for i := 0; i < len(samples) && i < 5; i++ {
		pcm, err := dec.Decode(ctx, samples[i])
		if err != nil {
			t.Fatalf("Decode frame %d failed: %v", i, err)
		}
		totalSamples += len(pcm)
	}

	if totalSamples == 0 {
		t.Error("no PCM samples decoded")
	}
	t.Logf("Decoded %d total PCM samples from stereo file", totalSamples)
}

// extractAACFromM4A extracts the AAC decoder config and raw AAC samples from an M4A file
func extractAACFromM4A(filename string) (config []byte, samples [][]byte, err error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	// Read the entire file for probing
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, nil, err
	}

	// Find esds box to get decoder config
	config, err = findDecoderConfig(bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}

	// Reset file position
	_, _ = f.Seek(0, io.SeekStart)

	// Extract sample data
	samples, err = extractSamples(f, data)
	if err != nil {
		return nil, nil, err
	}

	return config, samples, nil
}

// errConfigFound is a sentinel error used to stop parsing when config is found.
var errConfigFound = errors.New("config found")

// findDecoderConfig finds the AudioSpecificConfig in the esds box
func findDecoderConfig(r io.ReadSeeker) ([]byte, error) {
	var config []byte

	_, err := mp4.ReadBoxStructure(r, func(h *mp4.ReadHandle) (any, error) {
		if h.BoxInfo.Type == mp4.BoxTypeEsds() {
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			esds, ok := box.(*mp4.Esds)
			if !ok {
				return h.Expand()
			}
			// The Descriptors array contains nested descriptors
			// Tag 0x03 = ES_Descriptor, Tag 0x04 = DecoderConfigDescriptor, Tag 0x05 = DecoderSpecificInfo
			for _, desc := range esds.Descriptors {
				// Tag 0x05 is DecoderSpecificInfo which contains AudioSpecificConfig
				if desc.Tag == 0x05 && len(desc.Data) > 0 {
					config = desc.Data
					return nil, errConfigFound
				}
			}
		}
		return h.Expand()
	})

	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, errConfigFound) {
		return nil, err
	}

	return config, nil
}

// extractSamples extracts raw AAC frame data from the M4A file
func extractSamples(_ *os.File, data []byte) ([][]byte, error) {
	r := bytes.NewReader(data)

	var sampleSizes []uint32
	var chunkOffsets []uint64
	var samplesPerChunk []mp4.StscEntry

	// First pass: collect sample table info
	_, err := mp4.ReadBoxStructure(r, func(h *mp4.ReadHandle) (any, error) {
		switch h.BoxInfo.Type {
		case mp4.BoxTypeStsz():
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			stsz, ok := box.(*mp4.Stsz)
			if !ok {
				return h.Expand()
			}
			if stsz.SampleSize != 0 {
				// Fixed size samples
				for range stsz.SampleCount {
					sampleSizes = append(sampleSizes, stsz.SampleSize)
				}
			} else {
				sampleSizes = stsz.EntrySize
			}

		case mp4.BoxTypeStco():
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			stco, ok := box.(*mp4.Stco)
			if !ok {
				return h.Expand()
			}
			for _, offset := range stco.ChunkOffset {
				chunkOffsets = append(chunkOffsets, uint64(offset))
			}

		case mp4.BoxTypeCo64():
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			co64, ok := box.(*mp4.Co64)
			if !ok {
				return h.Expand()
			}
			chunkOffsets = co64.ChunkOffset

		case mp4.BoxTypeStsc():
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			stsc, ok := box.(*mp4.Stsc)
			if !ok {
				return h.Expand()
			}
			samplesPerChunk = stsc.Entries
		}
		return h.Expand()
	})

	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}

	if len(sampleSizes) == 0 || len(chunkOffsets) == 0 {
		return nil, nil
	}

	// Build sample list with offsets
	var samples [][]byte
	sampleIdx := uint32(0)

	for chunkIdx, chunkOffset := range chunkOffsets {
		// Find how many samples in this chunk
		samplesInChunk := uint32(1)
		for i := len(samplesPerChunk) - 1; i >= 0; i-- {
			if uint32(chunkIdx+1) >= samplesPerChunk[i].FirstChunk { //nolint:gosec // chunk count is small
				samplesInChunk = samplesPerChunk[i].SamplesPerChunk
				break
			}
		}

		offset := chunkOffset
		for i := uint32(0); i < samplesInChunk && sampleIdx < uint32(len(sampleSizes)); i++ { //nolint:gosec // sample count bounded
			size := sampleSizes[sampleIdx]
			if offset+uint64(size) <= uint64(len(data)) {
				sample := make([]byte, size)
				copy(sample, data[offset:offset+uint64(size)])
				samples = append(samples, sample)
			}
			offset += uint64(size)
			sampleIdx++
		}
	}

	return samples, nil
}
