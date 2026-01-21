package faad2

import (
	"io"
)

// M4AReader reads and decodes audio from M4A/MP4 files.
type M4AReader struct {
	decoder *Decoder
	reader  io.ReadSeeker

	// Track info
	sampleRate uint32
	channels   uint8

	// Sample table info (from go-mp4 probing)
	samples    []sampleInfo
	currentIdx int
}

type sampleInfo struct {
	offset uint64
	size   uint32
}

// OpenM4A opens an M4A/MP4 file for audio decoding.
func OpenM4A(r io.ReadSeeker) (*M4AReader, error) {
	// 1. Probe the file to find AAC audio track
	// 2. Extract codec config from esds box
	// 3. Build sample table (offsets and sizes)
	// 4. Initialize decoder with codec config

	// Use go-mp4 to probe and extract info
	// Implementation details:
	// - Find moov/trak with audio handler (hdlr type "soun")
	// - Extract esds box for AAC decoder config
	// - Parse stts, stsc, stsz, stco/co64 for sample table

	// TODO: implement using github.com/abema/go-mp4
	return nil, ErrNotM4A
}

// Read reads decoded PCM samples into the buffer.
// Returns number of samples read, or io.EOF when done.
func (m *M4AReader) Read(pcm []int16) (int, error) {
	// 1. Read next AAC frame from file using sample table
	// 2. Decode frame
	// 3. Copy to output buffer
	// 4. Handle partial reads across frames

	// TODO: implement
	return 0, io.EOF
}

// Seek seeks to a specific sample position.
func (m *M4AReader) Seek(samplePos int64) error {
	// Use sample table to seek
	// TODO: implement
	return nil
}

// SampleRate returns the audio sample rate.
func (m *M4AReader) SampleRate() uint32 {
	return m.sampleRate
}

// Channels returns the number of audio channels.
func (m *M4AReader) Channels() uint8 {
	return m.channels
}

// Close releases all resources.
func (m *M4AReader) Close() error {
	if m.decoder != nil {
		return m.decoder.Close()
	}
	return nil
}
