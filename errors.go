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

	// ErrDecoderClosed is returned when trying to use a closed decoder.
	ErrDecoderClosed = errors.New("faad2: decoder is closed")

	// ErrEmptyFrame is returned when trying to decode an empty AAC frame.
	ErrEmptyFrame = errors.New("faad2: empty AAC frame")
)
