package faad2

import (
	"bytes"
	"context"
	"errors"
	"io"
	"time"

	"github.com/abema/go-mp4"
)

// M4AReader reads and decodes audio from M4A/MP4 container files.
//
// It provides streaming access to decoded PCM audio along with metadata,
// duration, seeking, and position tracking.
//
// Create an M4AReader using [OpenM4A] and release resources with [M4AReader.Close].
type M4AReader struct {
	decoder *Decoder
	reader  io.ReadSeeker

	// Track info
	sampleRate uint32
	channels   uint8
	duration   time.Duration
	timescale  uint32

	// Sample table info
	samples    []sampleInfo
	currentIdx int

	// PCM buffer for partial reads
	pcmBuffer []int16
	pcmOffset int

	// Metadata
	metadata Metadata
}

type sampleInfo struct {
	offset   uint64
	size     uint32
	duration uint32 // in timescale units
}

// Metadata contains M4A/MP4 file metadata tags.
//
// All fields are optional and may be empty if not present in the file.
type Metadata struct {
	Title       string // Track title (©nam)
	Artist      string // Artist name (©ART)
	Album       string // Album name (©alb)
	Year        int    // Release year (©day)
	TrackNumber int    // Track number (trkn)
	Genre       string // Genre (©gen)
}

// OpenM4A opens an M4A/MP4 file for audio decoding.
//
// The reader must support seeking as M4A files require random access
// to read audio samples from various positions.
//
// Note: This function reads the entire file into memory for container parsing.
// For very large files (hundreds of MB), consider memory constraints.
//
// Returns [ErrNotM4A] if the file is not a valid MP4 container,
// [ErrNoAudioTrack] if no AAC audio track is found, or
// [ErrUnsupportedCodec] if the audio codec is not AAC.
func OpenM4A(ctx context.Context, r io.ReadSeeker) (*M4AReader, error) {
	mr := &M4AReader{
		reader: r,
	}

	// Read entire file for parsing (needed for go-mp4)
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	// Parse MP4 structure
	info, err := parseM4A(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	if len(info.config) == 0 {
		return nil, ErrNoAudioTrack
	}

	mr.sampleRate = info.sampleRate
	mr.channels = info.channels
	mr.timescale = info.timescale
	mr.samples = info.samples
	mr.metadata = info.metadata

	// Calculate duration
	var totalDuration uint64
	for _, s := range mr.samples {
		totalDuration += uint64(s.duration)
	}
	if mr.timescale > 0 {
		mr.duration = time.Duration(totalDuration) * time.Second / time.Duration(mr.timescale) //nolint:gosec // duration fits in int64
	}

	// Create and initialize decoder
	decoder, err := NewDecoder(ctx)
	if err != nil {
		return nil, err
	}

	err = decoder.Init(ctx, info.config)
	if err != nil {
		decoder.Close(ctx)
		return nil, err
	}

	mr.decoder = decoder

	// Reset reader position
	_, _ = r.Seek(0, io.SeekStart)

	return mr, nil
}

// Read reads decoded PCM samples into the provided buffer.
//
// Returns the number of samples read into pcm. For stereo audio, each sample
// pair (L, R) counts as 2 samples. Returns [io.EOF] when all audio has been read.
//
// The buffer can be any size; the reader handles internal buffering.
// Smaller buffers result in more Read calls but use less memory.
func (m *M4AReader) Read(ctx context.Context, pcm []int16) (int, error) {
	if m.decoder == nil {
		return 0, ErrNotInitialized
	}

	totalRead := 0

	for totalRead < len(pcm) {
		// First, drain any buffered samples
		if m.pcmOffset < len(m.pcmBuffer) {
			n := copy(pcm[totalRead:], m.pcmBuffer[m.pcmOffset:])
			m.pcmOffset += n
			totalRead += n
			continue
		}

		// Check if we've read all samples
		if m.currentIdx >= len(m.samples) {
			if totalRead > 0 {
				return totalRead, nil
			}
			return 0, io.EOF
		}

		// Read next sample
		sample := m.samples[m.currentIdx]
		m.currentIdx++

		// Seek to sample position
		_, err := m.reader.Seek(int64(sample.offset), io.SeekStart) //nolint:gosec // file offset fits in int64
		if err != nil {
			return totalRead, err
		}

		// Read sample data
		sampleData := make([]byte, sample.size)
		_, err = io.ReadFull(m.reader, sampleData)
		if err != nil {
			return totalRead, err
		}

		// Decode sample
		samples, err := m.decoder.Decode(ctx, sampleData)
		if err != nil {
			return totalRead, err
		}

		if len(samples) == 0 {
			continue
		}

		// Copy to output or buffer
		n := copy(pcm[totalRead:], samples)
		totalRead += n

		if n < len(samples) {
			// Buffer remaining samples
			m.pcmBuffer = samples
			m.pcmOffset = n
		} else {
			m.pcmBuffer = nil
			m.pcmOffset = 0
		}
	}

	return totalRead, nil
}

// SampleRate returns the audio sample rate in Hz (e.g., 44100, 48000).
func (m *M4AReader) SampleRate() uint32 {
	return m.sampleRate
}

// Channels returns the number of audio channels (1 for mono, 2 for stereo).
func (m *M4AReader) Channels() uint8 {
	return m.channels
}

// Duration returns the total duration of the audio track.
func (m *M4AReader) Duration() time.Duration {
	return m.duration
}

// Metadata returns the file's metadata (title, artist, album, etc.).
//
// Fields may be empty if the file does not contain the corresponding metadata.
func (m *M4AReader) Metadata() Metadata {
	return m.metadata
}

// Position returns the current playback position based on samples read so far.
func (m *M4AReader) Position() time.Duration {
	if m.timescale == 0 || m.currentIdx == 0 {
		return 0
	}

	// Sum durations of all samples up to current index
	var totalDuration uint64
	for i := 0; i < m.currentIdx && i < len(m.samples); i++ {
		totalDuration += uint64(m.samples[i].duration)
	}

	return time.Duration(totalDuration) * time.Second / time.Duration(m.timescale) //nolint:gosec // duration fits in int64
}

// Seek moves the playback position to the specified time.
//
// The actual position after seeking may differ slightly from the requested
// position due to AAC frame boundaries. Use [M4AReader.Position] to get the
// actual position after seeking.
//
// Seeking past the end of the file positions at EOF; the next [M4AReader.Read]
// will return [io.EOF].
//
// Returns [ErrSeekUnavailable] if the file lacks timing information.
func (m *M4AReader) Seek(position time.Duration) error {
	if m.timescale == 0 {
		return ErrSeekUnavailable
	}

	// Convert time to timescale units
	targetTime := uint64(position) * uint64(m.timescale) / uint64(time.Second) //nolint:gosec // time value fits in uint64

	// Find the sample index for this time
	var accumulatedTime uint64
	targetIdx := 0

	for i, sample := range m.samples {
		if accumulatedTime+uint64(sample.duration) > targetTime {
			targetIdx = i
			break
		}
		accumulatedTime += uint64(sample.duration)
		targetIdx = i + 1
	}

	// Clamp to valid range
	if targetIdx >= len(m.samples) {
		targetIdx = len(m.samples)
	}

	// Update state
	m.currentIdx = targetIdx
	m.pcmBuffer = nil
	m.pcmOffset = 0

	return nil
}

// Close releases all resources associated with the reader.
//
// After Close is called, the reader cannot be reused.
// It is safe to call Close multiple times; subsequent calls are no-ops.
//
// Note: Close does not close the underlying io.ReadSeeker passed to [OpenM4A].
func (m *M4AReader) Close(ctx context.Context) error {
	if m.decoder != nil {
		err := m.decoder.Close(ctx)
		m.decoder = nil
		return err
	}
	return nil
}

// m4aInfo contains parsed M4A information.
type m4aInfo struct {
	config     []byte
	sampleRate uint32
	channels   uint8
	timescale  uint32
	samples    []sampleInfo
	metadata   Metadata
}

// parseM4A parses the M4A file structure and extracts audio info.
func parseM4A(r io.ReadSeeker) (*m4aInfo, error) {
	info := &m4aInfo{}

	// Temporary storage during parsing
	var sampleSizes []uint32
	var chunkOffsets []uint64
	var stscEntries []mp4.StscEntry
	var sttsEntries []mp4.SttsEntry
	var audioTrackFound bool
	var currentTrackTimescale uint32 // timescale for current track being parsed

	_, err := mp4.ReadBoxStructure(r, func(h *mp4.ReadHandle) (any, error) {
		switch h.BoxInfo.Type {
		// Container boxes that need expansion
		case mp4.BoxTypeMoov(), mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStsd():
			return h.Expand()

		case mp4.BoxTypeTrak():
			// Reset per-track state when entering a new track
			audioTrackFound = false
			currentTrackTimescale = 0
			return h.Expand()

		case mp4.BoxTypeMdhd():
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			mdhd, ok := box.(*mp4.Mdhd)
			if !ok {
				return nil, nil //nolint:nilnil // go-mp4 callback: nil,nil means continue
			}
			// Save timescale - we'll use it if this turns out to be an audio track
			currentTrackTimescale = mdhd.Timescale

		case mp4.BoxTypeHdlr():
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			hdlr, ok := box.(*mp4.Hdlr)
			if !ok {
				return nil, nil //nolint:nilnil // go-mp4 callback: nil,nil means continue
			}
			// Check if this is a sound handler
			if hdlr.HandlerType == [4]byte{'s', 'o', 'u', 'n'} {
				audioTrackFound = true
				// Now we know this is an audio track, save the timescale
				info.timescale = currentTrackTimescale
			}

		case mp4.BoxTypeMp4a():
			if !audioTrackFound {
				return nil, nil //nolint:nilnil // skip non-audio track
			}
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			mp4a, ok := box.(*mp4.AudioSampleEntry)
			if !ok {
				return nil, nil //nolint:nilnil // go-mp4 callback: nil,nil means continue
			}
			info.sampleRate = mp4a.SampleRate / 65536 // Fixed point 16.16
			info.channels = uint8(mp4a.ChannelCount)  //nolint:gosec // ChannelCount is always small
			// Expand to find esds child box
			return h.Expand()

		case mp4.BoxTypeEsds():
			if !audioTrackFound {
				return nil, nil //nolint:nilnil // skip non-audio track
			}
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			esds, ok := box.(*mp4.Esds)
			if !ok {
				return nil, nil //nolint:nilnil // go-mp4 callback: nil,nil means continue
			}
			// Find DecoderSpecificInfo (tag 0x05)
			for _, desc := range esds.Descriptors {
				if desc.Tag == 0x05 && len(desc.Data) > 0 {
					info.config = desc.Data
					break
				}
			}

		case mp4.BoxTypeStsz():
			if !audioTrackFound {
				return nil, nil //nolint:nilnil // skip non-audio track
			}
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			stsz, ok := box.(*mp4.Stsz)
			if !ok {
				return nil, nil //nolint:nilnil // go-mp4 callback: nil,nil means continue
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
			if !audioTrackFound {
				return nil, nil //nolint:nilnil // skip non-audio track
			}
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			stco, ok := box.(*mp4.Stco)
			if !ok {
				return nil, nil //nolint:nilnil // go-mp4 callback: nil,nil means continue
			}
			for _, offset := range stco.ChunkOffset {
				chunkOffsets = append(chunkOffsets, uint64(offset))
			}

		case mp4.BoxTypeCo64():
			if !audioTrackFound {
				return nil, nil //nolint:nilnil // skip non-audio track
			}
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			co64, ok := box.(*mp4.Co64)
			if !ok {
				return nil, nil //nolint:nilnil // go-mp4 callback: nil,nil means continue
			}
			chunkOffsets = co64.ChunkOffset

		case mp4.BoxTypeStsc():
			if !audioTrackFound {
				return nil, nil //nolint:nilnil // skip non-audio track
			}
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			stsc, ok := box.(*mp4.Stsc)
			if !ok {
				return nil, nil //nolint:nilnil // go-mp4 callback: nil,nil means continue
			}
			stscEntries = stsc.Entries

		case mp4.BoxTypeStts():
			if !audioTrackFound {
				return nil, nil //nolint:nilnil // skip non-audio track
			}
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			stts, ok := box.(*mp4.Stts)
			if !ok {
				return nil, nil //nolint:nilnil // go-mp4 callback: nil,nil means continue
			}
			sttsEntries = stts.Entries

		case mp4.BoxTypeUdta(), mp4.BoxTypeMeta(), mp4.BoxTypeIlst():
			// Expand to find metadata items
			return h.Expand()

		case mp4.BoxType{'\xa9', 'n', 'a', 'm'}, // ©nam - title
			mp4.BoxType{'\xa9', 'A', 'R', 'T'}, // ©ART - artist
			mp4.BoxType{'\xa9', 'a', 'l', 'b'}, // ©alb - album
			mp4.BoxType{'\xa9', 'd', 'a', 'y'}, // ©day - year
			mp4.BoxType{'\xa9', 'g', 'e', 'n'}: // ©gen - genre
			// These boxes contain a "data" sub-box, expand to find it
			return h.Expand()

		case mp4.BoxTypeData():
			// Data box inside metadata item - read the actual value
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			data, ok := box.(*mp4.Data)
			if !ok {
				return nil, nil //nolint:nilnil // go-mp4 callback: nil,nil means continue
			}
			// Get parent box type to know which metadata field this is
			// h.Path is []BoxType, so h.Path[len-2] is the grandparent (the metadata item box)
			if len(h.Path) >= 2 {
				parentType := h.Path[len(h.Path)-2]
				switch parentType {
				case mp4.BoxType{'\xa9', 'n', 'a', 'm'}:
					info.metadata.Title = string(data.Data)
				case mp4.BoxType{'\xa9', 'A', 'R', 'T'}:
					info.metadata.Artist = string(data.Data)
				case mp4.BoxType{'\xa9', 'a', 'l', 'b'}:
					info.metadata.Album = string(data.Data)
				case mp4.BoxType{'\xa9', 'g', 'e', 'n'}:
					info.metadata.Genre = string(data.Data)
				}
			}
		}

		// Skip unknown boxes instead of trying to expand them
		// This prevents errors on boxes go-mp4 doesn't recognize (e.g., cover art tracks)
		return nil, nil //nolint:nilnil // go-mp4 callback: nil,nil means continue
	})

	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}

	if len(info.config) == 0 {
		return nil, ErrNoAudioTrack
	}

	// Build sample table with offsets and durations
	info.samples = buildSampleTable(sampleSizes, chunkOffsets, stscEntries, sttsEntries)

	return info, nil
}

// buildSampleTable builds the sample table from MP4 box data.
func buildSampleTable(sampleSizes []uint32, chunkOffsets []uint64, stscEntries []mp4.StscEntry, sttsEntries []mp4.SttsEntry) []sampleInfo {
	if len(sampleSizes) == 0 || len(chunkOffsets) == 0 {
		return nil
	}

	samples := make([]sampleInfo, 0, len(sampleSizes))

	// Build duration lookup from stts
	sampleDurations := make([]uint32, 0, len(sampleSizes))
	for _, entry := range sttsEntries {
		for range entry.SampleCount {
			sampleDurations = append(sampleDurations, entry.SampleDelta)
		}
	}

	// Process chunks
	sampleIdx := uint32(0)
	for chunkIdx, offset := range chunkOffsets {
		// Find how many samples in this chunk
		samplesInChunk := uint32(1)
		for i := len(stscEntries) - 1; i >= 0; i-- {
			if uint32(chunkIdx+1) >= stscEntries[i].FirstChunk { //nolint:gosec // chunk count is small
				samplesInChunk = stscEntries[i].SamplesPerChunk
				break
			}
		}
		for i := uint32(0); i < samplesInChunk && sampleIdx < uint32(len(sampleSizes)); i++ { //nolint:gosec // sample count bounded
			size := sampleSizes[sampleIdx]
			duration := uint32(1024) // default AAC frame duration
			if int(sampleIdx) < len(sampleDurations) {
				duration = sampleDurations[sampleIdx]
			}

			samples = append(samples, sampleInfo{
				offset:   offset,
				size:     size,
				duration: duration,
			})

			offset += uint64(size)
			sampleIdx++
		}
	}

	return samples
}
