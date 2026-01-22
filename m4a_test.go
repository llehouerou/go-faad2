package faad2

import (
	"context"
	"os"
	"testing"
	"time"
)

const (
	testMonoM4A     = "testdata/mono_44100.m4a"
	testStereoM4A   = "testdata/stereo_48000.m4a"
	testMetadataM4A = "testdata/with_metadata.m4a"
)

func TestOpenM4A(t *testing.T) {
	ctx := context.Background()
	testFile := testMonoM4A
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("test file not found, run 'make testdata' first")
	}

	f, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer f.Close()

	reader, err := OpenM4A(ctx, f)
	if err != nil {
		t.Fatalf("OpenM4A failed: %v", err)
	}
	defer reader.Close(ctx)

	t.Logf("M4A: sampleRate=%d, channels=%d, duration=%v",
		reader.SampleRate(), reader.Channels(), reader.Duration())

	if reader.SampleRate() != 44100 {
		t.Errorf("expected sample rate 44100, got %d", reader.SampleRate())
	}
}

func TestOpenM4AStereo(t *testing.T) {
	ctx := context.Background()
	testFile := testStereoM4A
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("test file not found, run 'make testdata' first")
	}

	f, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer f.Close()

	reader, err := OpenM4A(ctx, f)
	if err != nil {
		t.Fatalf("OpenM4A failed: %v", err)
	}
	defer reader.Close(ctx)

	t.Logf("M4A stereo: sampleRate=%d, channels=%d, duration=%v",
		reader.SampleRate(), reader.Channels(), reader.Duration())

	if reader.SampleRate() != 48000 {
		t.Errorf("expected sample rate 48000, got %d", reader.SampleRate())
	}
	if reader.Channels() != 2 {
		t.Errorf("expected 2 channels, got %d", reader.Channels())
	}
}

func TestM4ARead(t *testing.T) {
	ctx := context.Background()
	testFile := testMonoM4A
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("test file not found, run 'make testdata' first")
	}

	f, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer f.Close()

	reader, err := OpenM4A(ctx, f)
	if err != nil {
		t.Fatalf("OpenM4A failed: %v", err)
	}
	defer reader.Close(ctx)

	// Read all samples
	pcm := make([]int16, 4096)
	totalSamples := 0

	for {
		n, err := reader.Read(ctx, pcm)
		if err != nil {
			break
		}
		totalSamples += n
		if n < len(pcm) {
			break
		}
	}

	t.Logf("Total samples decoded: %d", totalSamples)

	if totalSamples == 0 {
		t.Error("no samples decoded")
	}

	// For 1 second of audio with ~45 AAC frames at 2048 samples/frame
	expectedMin := 80000
	expectedMax := 100000
	if totalSamples < expectedMin || totalSamples > expectedMax {
		t.Errorf("expected between %d and %d samples, got %d", expectedMin, expectedMax, totalSamples)
	}
}

func TestM4AReadSmallBuffer(t *testing.T) {
	ctx := context.Background()
	testFile := testMonoM4A
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("test file not found, run 'make testdata' first")
	}

	f, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer f.Close()

	reader, err := OpenM4A(ctx, f)
	if err != nil {
		t.Fatalf("OpenM4A failed: %v", err)
	}
	defer reader.Close(ctx)

	// Read with small buffer to test buffering logic
	pcm := make([]int16, 512)
	totalSamples := 0
	readCount := 0

	for {
		n, err := reader.Read(ctx, pcm)
		if err != nil {
			break
		}
		totalSamples += n
		readCount++
		if n < len(pcm) {
			break
		}
	}

	t.Logf("Total samples: %d in %d reads", totalSamples, readCount)

	if totalSamples == 0 {
		t.Error("no samples decoded")
	}
	if readCount < 10 {
		t.Error("expected multiple reads with small buffer")
	}
}

func TestM4ADuration(t *testing.T) {
	ctx := context.Background()
	testFile := testMonoM4A
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("test file not found, run 'make testdata' first")
	}

	f, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer f.Close()

	reader, err := OpenM4A(ctx, f)
	if err != nil {
		t.Fatalf("OpenM4A failed: %v", err)
	}
	defer reader.Close(ctx)

	duration := reader.Duration()
	t.Logf("Duration: %v", duration)

	// Should be approximately 1 second
	if duration < 900*time.Millisecond || duration > 1100*time.Millisecond {
		t.Errorf("expected duration around 1s, got %v", duration)
	}
}

func TestM4AMetadata(t *testing.T) {
	ctx := context.Background()
	testFile := testMetadataM4A
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("test file not found, run 'make testdata' first")
	}

	f, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer f.Close()

	reader, err := OpenM4A(ctx, f)
	if err != nil {
		t.Fatalf("OpenM4A failed: %v", err)
	}
	defer reader.Close(ctx)

	meta := reader.Metadata()
	t.Logf("Metadata: title=%q, artist=%q, album=%q", meta.Title, meta.Artist, meta.Album)

	// Note: metadata parsing may not work perfectly with go-mp4's box structure
	// This test verifies the function doesn't crash, not necessarily that it works
}

func TestM4AWithMetadataRead(t *testing.T) {
	ctx := context.Background()
	testFile := testMetadataM4A
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("test file not found, run 'make testdata' first")
	}

	f, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer f.Close()

	reader, err := OpenM4A(ctx, f)
	if err != nil {
		t.Fatalf("OpenM4A failed: %v", err)
	}
	defer reader.Close(ctx)

	// Read all samples from 2-second file
	pcm := make([]int16, 4096)
	totalSamples := 0

	for {
		n, err := reader.Read(ctx, pcm)
		if err != nil {
			break
		}
		totalSamples += n
		if n < len(pcm) {
			break
		}
	}

	t.Logf("Total samples from metadata file: %d, duration: %v", totalSamples, reader.Duration())

	// For 2 second file, expect roughly double the samples
	expectedMin := 160000
	expectedMax := 200000
	if totalSamples < expectedMin || totalSamples > expectedMax {
		t.Errorf("expected between %d and %d samples, got %d", expectedMin, expectedMax, totalSamples)
	}

	// Duration should be around 2 seconds
	if reader.Duration() < 1900*time.Millisecond || reader.Duration() > 2100*time.Millisecond {
		t.Errorf("expected duration around 2s, got %v", reader.Duration())
	}
}

func TestM4ASeek(t *testing.T) {
	ctx := context.Background()
	testFile := testMetadataM4A
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("test file not found, run 'make testdata' first")
	}

	f, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer f.Close()

	reader, err := OpenM4A(ctx, f)
	if err != nil {
		t.Fatalf("OpenM4A failed: %v", err)
	}
	defer reader.Close(ctx)

	// Read some samples first
	pcm := make([]int16, 4096)
	_, err = reader.Read(ctx, pcm)
	if err != nil {
		t.Fatalf("initial read failed: %v", err)
	}

	// Position should be non-zero after reading
	pos1 := reader.Position()
	t.Logf("Position after first read: %v", pos1)

	// Seek to middle of file (1 second into 2 second file)
	err = reader.Seek(1 * time.Second)
	if err != nil {
		t.Fatalf("Seek failed: %v", err)
	}

	pos2 := reader.Position()
	t.Logf("Position after seeking to 1s: %v", pos2)

	// Position should be close to 1 second
	if pos2 < 900*time.Millisecond || pos2 > 1100*time.Millisecond {
		t.Errorf("expected position around 1s after seek, got %v", pos2)
	}

	// Should still be able to read after seeking
	n, err := reader.Read(ctx, pcm)
	if err != nil {
		t.Fatalf("read after seek failed: %v", err)
	}
	if n == 0 {
		t.Error("no samples read after seek")
	}

	// Seek to beginning
	err = reader.Seek(0)
	if err != nil {
		t.Fatalf("Seek to 0 failed: %v", err)
	}

	pos3 := reader.Position()
	if pos3 != 0 {
		t.Errorf("expected position 0 after seeking to start, got %v", pos3)
	}

	// Seek past end should clamp to end
	err = reader.Seek(10 * time.Second)
	if err != nil {
		t.Fatalf("Seek past end failed: %v", err)
	}

	// Should get EOF on next read
	_, err = reader.Read(ctx, pcm)
	if err == nil {
		t.Error("expected EOF after seeking past end")
	}
}

func TestM4APosition(t *testing.T) {
	ctx := context.Background()
	testFile := testMonoM4A
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("test file not found, run 'make testdata' first")
	}

	f, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer f.Close()

	reader, err := OpenM4A(ctx, f)
	if err != nil {
		t.Fatalf("OpenM4A failed: %v", err)
	}
	defer reader.Close(ctx)

	// Initial position should be 0
	if reader.Position() != 0 {
		t.Errorf("expected initial position 0, got %v", reader.Position())
	}

	// Read samples and check position increases
	pcm := make([]int16, 4096)
	var lastPos time.Duration

	for i := range 5 {
		n, err := reader.Read(ctx, pcm)
		if err != nil {
			break
		}
		if n == 0 {
			break
		}

		currentPos := reader.Position()
		t.Logf("Read %d: position=%v", i, currentPos)

		if i > 0 && currentPos <= lastPos {
			t.Errorf("position should increase: was %v, now %v", lastPos, currentPos)
		}
		lastPos = currentPos
	}

	// Position should be less than or equal to duration
	if lastPos > reader.Duration() {
		t.Errorf("position %v exceeds duration %v", lastPos, reader.Duration())
	}
}

func TestM4AMetadataValues(t *testing.T) {
	ctx := context.Background()
	testFile := testMetadataM4A
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("test file not found, run 'make testdata' first")
	}

	f, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer f.Close()

	reader, err := OpenM4A(ctx, f)
	if err != nil {
		t.Fatalf("OpenM4A failed: %v", err)
	}
	defer reader.Close(ctx)

	meta := reader.Metadata()
	t.Logf("Metadata: title=%q, artist=%q, album=%q, genre=%q",
		meta.Title, meta.Artist, meta.Album, meta.Genre)

	// Verify expected metadata values from testdata generation
	if meta.Title != "Test Title" {
		t.Errorf("expected title 'Test Title', got %q", meta.Title)
	}
	if meta.Artist != "Test Artist" {
		t.Errorf("expected artist 'Test Artist', got %q", meta.Artist)
	}
	if meta.Album != "Test Album" {
		t.Errorf("expected album 'Test Album', got %q", meta.Album)
	}
}

func TestM4ACloseIdempotent(t *testing.T) {
	ctx := context.Background()
	testFile := testMonoM4A
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("test file not found, run 'make testdata' first")
	}

	f, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer f.Close()

	reader, err := OpenM4A(ctx, f)
	if err != nil {
		t.Fatalf("OpenM4A failed: %v", err)
	}

	// Close once
	err = reader.Close(ctx)
	if err != nil {
		t.Errorf("First Close failed: %v", err)
	}

	// Close again - should be safe (no-op)
	err = reader.Close(ctx)
	if err != nil {
		t.Errorf("Second Close failed: %v", err)
	}
}

func TestM4AReadAfterClose(t *testing.T) {
	ctx := context.Background()
	testFile := testMonoM4A
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("test file not found, run 'make testdata' first")
	}

	f, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer f.Close()

	reader, err := OpenM4A(ctx, f)
	if err != nil {
		t.Fatalf("OpenM4A failed: %v", err)
	}

	// Close the reader
	err = reader.Close(ctx)
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Try to read after close - should get ErrNotInitialized
	pcm := make([]int16, 4096)
	_, err = reader.Read(ctx, pcm)
	if err == nil {
		t.Error("expected error when reading after Close")
	}
}
