package faad2

import "errors"

var (
	// ErrInvalidConfig is returned when the AAC codec configuration is invalid.
	ErrInvalidConfig = errors.New("faad2: invalid codec configuration")

	// ErrDecodeFailed is returned when AAC frame decoding fails.
	ErrDecodeFailed = errors.New("faad2: decode failed")

	// ErrOutOfMemory is returned when WASM memory allocation fails.
	ErrOutOfMemory = errors.New("faad2: out of memory")

	// ErrNotInitialized is returned when trying to decode without initialization.
	ErrNotInitialized = errors.New("faad2: decoder not initialized")

	// ErrNotM4A is returned when the input is not a valid M4A/MP4 file.
	ErrNotM4A = errors.New("faad2: not an M4A/MP4 file")

	// ErrNoAudioTrack is returned when no AAC audio track is found.
	ErrNoAudioTrack = errors.New("faad2: no AAC audio track found")

	// ErrUnsupportedCodec is returned when the audio codec is not AAC.
	ErrUnsupportedCodec = errors.New("faad2: unsupported audio codec (not AAC)")
)
