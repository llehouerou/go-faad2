package faad2

import (
	"errors"
	"os"
	"testing"
)

const testAACFile = "testdata/test.aac"

func TestParseADTSHeader(t *testing.T) {
	testFile := testAACFile
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("test file not found, run 'make testdata' first")
	}

	f, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer f.Close()

	// Read first 7 bytes (ADTS header)
	header := make([]byte, 7)
	_, err = f.Read(header)
	if err != nil {
		t.Fatalf("failed to read header: %v", err)
	}

	sampleRate, channels, frameLength, err := ParseADTSHeader(header)
	if err != nil {
		t.Fatalf("ParseADTSHeader failed: %v", err)
	}

	t.Logf("ADTS header: sampleRate=%d, channels=%d, frameLength=%d", sampleRate, channels, frameLength)

	if sampleRate != 44100 {
		t.Errorf("expected sample rate 44100, got %d", sampleRate)
	}
	if channels != 1 {
		t.Errorf("expected 1 channel, got %d", channels)
	}
	if frameLength == 0 {
		t.Error("frame length is 0")
	}
}

func TestParseADTSHeaderInvalid(t *testing.T) {
	// Test with invalid data
	_, _, _, err := ParseADTSHeader([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	if !errors.Is(err, ErrADTSSyncNotFound) {
		t.Errorf("expected ErrADTSSyncNotFound, got %v", err)
	}

	// Test with too short data
	_, _, _, err = ParseADTSHeader([]byte{0xFF, 0xF1})
	if !errors.Is(err, ErrInvalidADTS) {
		t.Errorf("expected ErrInvalidADTS, got %v", err)
	}
}

func TestOpenADTS(t *testing.T) {
	testFile := testAACFile
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("test file not found, run 'make testdata' first")
	}

	f, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer f.Close()

	reader, err := OpenADTS(f)
	if err != nil {
		t.Fatalf("OpenADTS failed: %v", err)
	}
	defer reader.Close()

	t.Logf("ADTS stream: sampleRate=%d, channels=%d", reader.SampleRate(), reader.Channels())

	if reader.SampleRate() != 44100 {
		t.Errorf("expected sample rate 44100, got %d", reader.SampleRate())
	}
	if reader.Channels() != 1 {
		t.Errorf("expected 1 channel, got %d", reader.Channels())
	}
}

func TestADTSRead(t *testing.T) {
	testFile := testAACFile
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("test file not found, run 'make testdata' first")
	}

	f, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer f.Close()

	reader, err := OpenADTS(f)
	if err != nil {
		t.Fatalf("OpenADTS failed: %v", err)
	}
	defer reader.Close()

	// Read all samples
	pcm := make([]int16, 4096)
	totalSamples := 0

	for {
		n, err := reader.Read(pcm)
		if err != nil {
			break
		}
		totalSamples += n
		if n < len(pcm) {
			break
		}
	}

	t.Logf("Total samples decoded: %d, frames read: %d", totalSamples, reader.FramesRead())

	if totalSamples == 0 {
		t.Error("no samples decoded")
	}

	// For 1 second of audio with ~45 AAC frames at 2048 samples/frame
	// expect ~90000 samples (decoder outputs interleaved stereo)
	expectedMin := 80000
	expectedMax := 100000
	if totalSamples < expectedMin || totalSamples > expectedMax {
		t.Errorf("expected between %d and %d samples, got %d", expectedMin, expectedMax, totalSamples)
	}
}

func TestADTSReadSmallBuffer(t *testing.T) {
	testFile := testAACFile
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("test file not found, run 'make testdata' first")
	}

	f, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer f.Close()

	reader, err := OpenADTS(f)
	if err != nil {
		t.Fatalf("OpenADTS failed: %v", err)
	}
	defer reader.Close()

	// Read with small buffer to test buffering logic
	pcm := make([]int16, 512)
	totalSamples := 0
	readCount := 0

	for {
		n, err := reader.Read(pcm)
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

func TestBuildAudioSpecificConfig(t *testing.T) {
	// Test AAC-LC at 44100Hz stereo
	// objectType=2 (AAC-LC), samplingFreqIndex=4 (44100), channelConfig=2 (stereo)
	config := buildAudioSpecificConfig(2, 4, 2)

	// Expected: objectType=2 (5 bits) = 00010
	//           samplingFreqIndex=4 (4 bits) = 0100
	//           channelConfig=2 (4 bits) = 0010
	// Packed: [00010 010] [0 0010 000] = [0x12] [0x10]
	expected := []byte{0x12, 0x10}

	if len(config) != len(expected) {
		t.Fatalf("expected %d bytes, got %d", len(expected), len(config))
	}

	for i := range expected {
		if config[i] != expected[i] {
			t.Errorf("byte %d: expected %02x, got %02x", i, expected[i], config[i])
		}
	}
}
