package faad2

import (
	"errors"
	"io"
)

// adtsSampleRateCount is the number of valid sample rate indices in ADTS.
const adtsSampleRateCount = 16

var (
	// ErrInvalidADTS is returned when the ADTS stream is invalid.
	ErrInvalidADTS = errors.New("faad2: invalid ADTS stream")

	// ErrADTSSyncNotFound is returned when no ADTS sync word is found.
	ErrADTSSyncNotFound = errors.New("faad2: ADTS sync word not found")
)

// Sample rate lookup table for ADTS
var adtsSampleRates = []uint32{
	96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050,
	16000, 12000, 11025, 8000, 7350, 0, 0, 0,
}

// ADTSReader reads and decodes audio from ADTS streams (raw AAC).
type ADTSReader struct {
	decoder    *Decoder
	reader     io.Reader
	sampleRate uint32
	channels   uint8

	// PCM buffer for partial reads
	pcmBuffer []int16
	pcmOffset int

	// Frame tracking
	framesRead int64

	// Header buffer for reading
	headerBuf [9]byte
}

// adtsHeader represents a parsed ADTS frame header.
type adtsHeader struct {
	syncWord          uint16 // 12 bits, should be 0xFFF
	id                uint8  // 1 bit, 0=MPEG-4, 1=MPEG-2
	layer             uint8  // 2 bits, always 0
	protectionAbsent  bool   // 1 bit, 1=no CRC
	profile           uint8  // 2 bits, AAC profile - 1
	samplingFreqIndex uint8  // 4 bits
	privateBit        bool   // 1 bit
	channelConfig     uint8  // 3 bits
	originalCopy      bool   // 1 bit
	home              bool   // 1 bit
	frameLength       uint16 // 13 bits, including header
	bufferFullness    uint16 // 11 bits
	numRawDataBlocks  uint8  // 2 bits
}

// OpenADTS opens an ADTS stream for audio decoding.
func OpenADTS(r io.Reader) (*ADTSReader, error) {
	ar := &ADTSReader{
		reader: r,
	}

	// Read and parse first header to get stream info
	header, err := ar.readHeader()
	if err != nil {
		return nil, err
	}

	// Extract sample rate and channels
	if header.samplingFreqIndex >= adtsSampleRateCount {
		return nil, ErrInvalidADTS
	}
	ar.sampleRate = adtsSampleRates[header.samplingFreqIndex]
	ar.channels = header.channelConfig

	if ar.sampleRate == 0 {
		return nil, ErrInvalidADTS
	}

	// Build AudioSpecificConfig from ADTS header
	config := buildAudioSpecificConfig(header.profile+1, header.samplingFreqIndex, header.channelConfig)

	// Create and initialize decoder
	decoder, err := NewDecoder()
	if err != nil {
		return nil, err
	}

	err = decoder.Init(config)
	if err != nil {
		decoder.Close()
		return nil, err
	}

	ar.decoder = decoder

	// Read first frame payload and decode (to prime the decoder)
	payload, err := ar.readPayload(header)
	if err != nil {
		decoder.Close()
		return nil, err
	}

	// Decode first frame (usually produces 0 samples - priming frame)
	pcm, err := decoder.Decode(payload)
	if err != nil {
		decoder.Close()
		return nil, err
	}
	ar.framesRead = 1

	// Buffer any samples from first frame
	if len(pcm) > 0 {
		ar.pcmBuffer = pcm
		ar.pcmOffset = 0
	}

	return ar, nil
}

// Read reads decoded PCM samples into the buffer.
// Returns the number of samples read.
func (ar *ADTSReader) Read(pcm []int16) (int, error) {
	if ar.decoder == nil {
		return 0, ErrNotInitialized
	}

	totalRead := 0

	for totalRead < len(pcm) {
		// First, drain any buffered samples
		if ar.pcmOffset < len(ar.pcmBuffer) {
			n := copy(pcm[totalRead:], ar.pcmBuffer[ar.pcmOffset:])
			ar.pcmOffset += n
			totalRead += n
			continue
		}

		// Read next frame
		header, err := ar.readHeader()
		if err != nil {
			if errors.Is(err, io.EOF) && totalRead > 0 {
				return totalRead, nil
			}
			return totalRead, err
		}

		payload, err := ar.readPayload(header)
		if err != nil {
			if errors.Is(err, io.EOF) && totalRead > 0 {
				return totalRead, nil
			}
			return totalRead, err
		}

		// Decode frame
		samples, err := ar.decoder.Decode(payload)
		if err != nil {
			return totalRead, err
		}
		ar.framesRead++

		if len(samples) == 0 {
			continue
		}

		// Copy to output or buffer
		n := copy(pcm[totalRead:], samples)
		totalRead += n

		if n < len(samples) {
			// Buffer remaining samples
			ar.pcmBuffer = samples
			ar.pcmOffset = n
		} else {
			ar.pcmBuffer = nil
			ar.pcmOffset = 0
		}
	}

	return totalRead, nil
}

// SampleRate returns the audio sample rate.
func (ar *ADTSReader) SampleRate() uint32 {
	return ar.sampleRate
}

// Channels returns the number of audio channels.
func (ar *ADTSReader) Channels() uint8 {
	return ar.channels
}

// FramesRead returns the number of AAC frames decoded so far.
func (ar *ADTSReader) FramesRead() int64 {
	return ar.framesRead
}

// Close releases all resources.
// It is safe to call Close multiple times.
func (ar *ADTSReader) Close() error {
	if ar.decoder != nil {
		err := ar.decoder.Close()
		ar.decoder = nil
		return err
	}
	return nil
}

// maxResyncBytes is the maximum number of bytes to search for a sync word
// when the stream becomes desynchronized.
const maxResyncBytes = 8192

// readHeader reads and parses an ADTS frame header.
// If the sync word is not found at the current position, it will attempt
// to resync by searching for the next valid sync word.
func (ar *ADTSReader) readHeader() (*adtsHeader, error) {
	// Read minimum header (7 bytes without CRC)
	_, err := io.ReadFull(ar.reader, ar.headerBuf[:7])
	if err != nil {
		return nil, err
	}

	// Check sync word (12 bits)
	syncWord := uint16(ar.headerBuf[0])<<4 | uint16(ar.headerBuf[1]>>4)
	if syncWord != 0xFFF {
		// Try to resync by searching for the sync word
		if err := ar.resync(); err != nil {
			return nil, err
		}
		syncWord = uint16(ar.headerBuf[0])<<4 | uint16(ar.headerBuf[1]>>4)
	}

	header := &adtsHeader{
		syncWord:          syncWord,
		id:                (ar.headerBuf[1] >> 3) & 0x01,
		layer:             (ar.headerBuf[1] >> 1) & 0x03,
		protectionAbsent:  (ar.headerBuf[1] & 0x01) == 1,
		profile:           (ar.headerBuf[2] >> 6) & 0x03,
		samplingFreqIndex: (ar.headerBuf[2] >> 2) & 0x0F,
		privateBit:        ((ar.headerBuf[2] >> 1) & 0x01) == 1,
		channelConfig:     ((ar.headerBuf[2] & 0x01) << 2) | ((ar.headerBuf[3] >> 6) & 0x03),
		originalCopy:      ((ar.headerBuf[3] >> 5) & 0x01) == 1,
		home:              ((ar.headerBuf[3] >> 4) & 0x01) == 1,
		frameLength:       (uint16(ar.headerBuf[3]&0x03) << 11) | (uint16(ar.headerBuf[4]) << 3) | (uint16(ar.headerBuf[5]>>5) & 0x07),
		bufferFullness:    (uint16(ar.headerBuf[5]&0x1F) << 6) | (uint16(ar.headerBuf[6]>>2) & 0x3F),
		numRawDataBlocks:  ar.headerBuf[6] & 0x03,
	}

	// If CRC is present, read 2 more bytes
	if !header.protectionAbsent {
		_, err := io.ReadFull(ar.reader, ar.headerBuf[7:9])
		if err != nil {
			return nil, err
		}
	}

	return header, nil
}

// readPayload reads the AAC frame payload after the header.
func (ar *ADTSReader) readPayload(header *adtsHeader) ([]byte, error) {
	headerSize := uint16(7)
	if !header.protectionAbsent {
		headerSize = 9
	}

	if header.frameLength <= headerSize {
		return nil, ErrInvalidADTS
	}

	payloadSize := header.frameLength - headerSize
	payload := make([]byte, payloadSize)

	_, err := io.ReadFull(ar.reader, payload)
	if err != nil {
		return nil, err
	}

	return payload, nil
}

// buildAudioSpecificConfig builds the AAC AudioSpecificConfig from ADTS header info.
// This is needed to initialize the decoder.
func buildAudioSpecificConfig(objectType, samplingFreqIndex, channelConfig uint8) []byte {
	// AudioSpecificConfig structure:
	// - audioObjectType (5 bits)
	// - samplingFrequencyIndex (4 bits)
	// - channelConfiguration (4 bits)
	// - GASpecificConfig...

	// For AAC-LC (objectType=2), minimal config is 2 bytes
	config := make([]byte, 2)

	// Pack into 2 bytes:
	// Byte 0: [objectType(5)] [samplingFreqIndex high 3 bits]
	// Byte 1: [samplingFreqIndex low 1 bit] [channelConfig(4)] [0000]
	config[0] = (objectType << 3) | ((samplingFreqIndex & 0x0E) >> 1)
	config[1] = ((samplingFreqIndex & 0x01) << 7) | (channelConfig << 3)

	return config
}

// ParseADTSHeader parses an ADTS header from raw bytes.
// Useful for inspecting ADTS streams without creating a reader.
func ParseADTSHeader(data []byte) (sampleRate uint32, channels uint8, frameLength uint16, err error) {
	if len(data) < 7 {
		return 0, 0, 0, ErrInvalidADTS
	}

	// Check sync word
	syncWord := uint16(data[0])<<4 | uint16(data[1]>>4)
	if syncWord != 0xFFF {
		return 0, 0, 0, ErrADTSSyncNotFound
	}

	samplingFreqIndex := (data[2] >> 2) & 0x0F
	if int(samplingFreqIndex) >= len(adtsSampleRates) {
		return 0, 0, 0, ErrInvalidADTS
	}

	sampleRate = adtsSampleRates[samplingFreqIndex]
	channels = ((data[2] & 0x01) << 2) | ((data[3] >> 6) & 0x03)
	frameLength = (uint16(data[3]&0x03) << 11) | (uint16(data[4]) << 3) | (uint16(data[5]>>5) & 0x07)

	return sampleRate, channels, frameLength, nil
}

// resync attempts to find the next valid ADTS sync word after desynchronization.
// It searches up to maxResyncBytes bytes for a valid sync word.
// On success, ar.headerBuf contains the new header.
func (ar *ADTSReader) resync() error {
	// We already have 7 bytes in headerBuf that didn't have a valid sync.
	// Start searching from byte 1 of what we have.
	searchBuf := make([]byte, maxResyncBytes)
	copy(searchBuf, ar.headerBuf[1:7]) // Copy remaining 6 bytes
	bytesInBuf := 6

	// Read more bytes to search through
	n, err := ar.reader.Read(searchBuf[bytesInBuf:])
	if err != nil && n == 0 {
		return ErrADTSSyncNotFound
	}
	bytesInBuf += n

	// Search for sync word (0xFF followed by 0xFx where x has bit 4 set)
	for i := range bytesInBuf - 1 {
		// Skip if not a sync word
		if searchBuf[i] != 0xFF || (searchBuf[i+1]&0xF0) != 0xF0 {
			continue
		}

		// Found potential sync word, need at least 7 bytes for header
		if i+7 <= bytesInBuf {
			copy(ar.headerBuf[:7], searchBuf[i:i+7])
			return nil
		}

		// Need to read more bytes for the full header
		copy(ar.headerBuf[:], searchBuf[i:bytesInBuf])
		_, err := io.ReadFull(ar.reader, ar.headerBuf[bytesInBuf-i:7])
		if err != nil {
			return err
		}
		return nil
	}

	return ErrADTSSyncNotFound
}
