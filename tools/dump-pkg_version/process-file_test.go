package main

import (
	"bytes"
	"encoding/hex"
	"io"
	"testing"
)

// MockReader generates repeated characters for testing.
type MockReader struct {
	Size    int
	Content []byte
}

func (r *MockReader) Read(p []byte) (int, error) {
	remaining := r.Size
	if remaining <= 0 {
		return 0, io.EOF // Return EOF when all content is exhausted.
	}

	readSize := min(len(p), remaining)

	// Fill the provided buffer p with enough repeated content from r.Content (a slice of bytes).
	// The idea is to handle cases where r.Content is smaller than p or where the requested
	// readSize is larger than r.Content.
	copy(p, bytes.Repeat(r.Content, readSize/len(r.Content)+1)[:readSize]) // Fill the buffer.
	r.Size -= readSize
	return readSize, nil
}

func TestProcessFile(t *testing.T) {
	// Generate 1 GB of "A" characters using MockReader.
	reader := &MockReader{
		Size:    1024 * 1024 * 1024, // 1 GB
		Content: []byte("A"),
	}

	md5Hash, xxh64Hash, size, err := processFileReader(reader)
	if err != nil {
		t.Fatalf("Failed to process reader: %v", err)
	}

	// Expected values for the test case.
	expectedMD5 := "90672a90fba312a3860b25b8861e8bd9" // Precomputed MD5 hash for 1 GB of "A".
	expectedXXH64 := "dc06caab0adc9ade"               // Precomputed XXH64 hash for 1 GB of "A".
	expectedSize := 1024 * 1024 * 1024                // 1 GB size.

	// Validate MD5 hash.
	hMd5Hash := hex.EncodeToString(md5Hash)
	if hMd5Hash != expectedMD5 {
		t.Errorf("MD5 mismatch: got %s, want %s", hMd5Hash, expectedMD5)
	}

	// Validate XXH64 hash.
	hXxh64Hash := hex.EncodeToString(xxh64Hash)
	if hXxh64Hash != expectedXXH64 {
		t.Errorf("XXH64 mismatch: got %s, want %s", hXxh64Hash, expectedXXH64)
	}

	// Validate file size.
	size32 := int(size)
	if size32 != expectedSize {
		t.Errorf("Size mismatch: got %d, want %d", size32, expectedSize)
	}
}
