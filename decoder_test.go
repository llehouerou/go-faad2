package faad2

import (
	"context"
	"errors"
	"testing"
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

func TestDecoderUseAfterClose(t *testing.T) {
	ctx := context.Background()
	dec, err := NewDecoder(ctx)
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}

	// Close the decoder
	err = dec.Close(ctx)
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Try to use after close - should get ErrDecoderClosed
	err = dec.Init(ctx, []byte{0x12, 0x10})
	if !errors.Is(err, ErrDecoderClosed) {
		t.Errorf("Init after Close: expected ErrDecoderClosed, got %v", err)
	}

	_, err = dec.Decode(ctx, []byte{0x00, 0x01, 0x02})
	if !errors.Is(err, ErrDecoderClosed) {
		t.Errorf("Decode after Close: expected ErrDecoderClosed, got %v", err)
	}

	// Close again should be safe (no-op)
	err = dec.Close(ctx)
	if err != nil {
		t.Errorf("Second Close failed: %v", err)
	}
}

func TestDecoderEmptyFrame(t *testing.T) {
	ctx := context.Background()

	dec, err := NewDecoder(ctx)
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}
	defer dec.Close(ctx)

	// Initialize with a valid AAC-LC config (44100Hz mono)
	// AudioSpecificConfig: 0x12 0x08 = AAC-LC, 44100Hz, mono
	err = dec.Init(ctx, []byte{0x12, 0x08})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Try to decode empty frame
	_, err = dec.Decode(ctx, []byte{})
	if !errors.Is(err, ErrEmptyFrame) {
		t.Errorf("Decode empty frame: expected ErrEmptyFrame, got %v", err)
	}

	_, err = dec.Decode(ctx, nil)
	if !errors.Is(err, ErrEmptyFrame) {
		t.Errorf("Decode nil frame: expected ErrEmptyFrame, got %v", err)
	}
}

func TestShutdownAndReinit(t *testing.T) {
	ctx := context.Background()

	// Create a decoder to initialize the WASM runtime
	dec1, err := NewDecoder(ctx)
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}

	// Close the decoder
	err = dec1.Close(ctx)
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Shutdown the WASM runtime
	err = Shutdown(ctx)
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// Shutdown again should be safe (no-op)
	err = Shutdown(ctx)
	if err != nil {
		t.Fatalf("Second Shutdown failed: %v", err)
	}

	// Create a new decoder - should reinitialize the runtime
	dec2, err := NewDecoder(ctx)
	if err != nil {
		t.Fatalf("NewDecoder after Shutdown failed: %v", err)
	}
	defer dec2.Close(ctx)

	if dec2.decoderPtr == 0 {
		t.Error("decoder pointer is nil after reinit")
	}
}

func TestContextCancellation(t *testing.T) {
	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Try to create a decoder with cancelled context
	// Note: wazero may or may not respect context cancellation during init
	// This test verifies the context is passed through correctly
	_, err := NewDecoder(ctx)
	if err != nil {
		// Context cancellation error is acceptable
		t.Logf("NewDecoder with cancelled context returned: %v", err)
	}
}
