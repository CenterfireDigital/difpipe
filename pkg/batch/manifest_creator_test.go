package batch

import (
	"testing"
)

func TestParseLocation(t *testing.T) {
	mc := NewManifestCreator(nil, nil, DefaultConfig())

	tests := []struct {
		name         string
		location     string
		expectedHost string
		expectedPath string
	}{
		{
			name:         "local path",
			location:     "/home/user/data",
			expectedHost: "",
			expectedPath: "/home/user/data",
		},
		{
			name:         "user@host:path",
			location:     "root@104.238.147.39:~/test-small-files",
			expectedHost: "104.238.147.39",
			expectedPath: "~/test-small-files",
		},
		{
			name:         "host:path (no user)",
			location:     "example.com:/data",
			expectedHost: "example.com",
			expectedPath: "/data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, path, err := mc.parseLocation(tt.location)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if host != tt.expectedHost {
				t.Errorf("expected host %q, got %q", tt.expectedHost, host)
			}
			if path != tt.expectedPath {
				t.Errorf("expected path %q, got %q", tt.expectedPath, path)
			}
		})
	}
}

func TestBinPackFiles(t *testing.T) {
	config := DefaultConfig()
	config.ChunkSizeMB = 10 // 10MB chunks for testing

	mc := NewManifestCreator(nil, nil, config)

	files := []FileInfo{
		{Path: "file1.txt", Size: 5 * 1024 * 1024},  // 5MB
		{Path: "file2.txt", Size: 3 * 1024 * 1024},  // 3MB
		{Path: "file3.txt", Size: 7 * 1024 * 1024},  // 7MB
		{Path: "file4.txt", Size: 2 * 1024 * 1024},  // 2MB
		{Path: "file5.txt", Size: 15 * 1024 * 1024}, // 15MB (larger than chunk)
		{Path: "file6.txt", Size: 1 * 1024 * 1024},  // 1MB
	}

	batches := mc.binPackFiles(files)

	// Expected batches:
	// Batch 0: file1 (5MB) + file2 (3MB) = 8MB
	// Batch 1: file3 (7MB) + file4 (2MB) = 9MB
	// Batch 2: file5 (15MB) = 15MB (exceeds target, but that's ok)
	// Batch 3: file6 (1MB) = 1MB

	if len(batches) != 4 {
		t.Errorf("expected 4 batches, got %d", len(batches))
	}

	// Check batch 0
	if len(batches[0].files) != 2 {
		t.Errorf("batch 0: expected 2 files, got %d", len(batches[0].files))
	}
	if batches[0].size != 8*1024*1024 {
		t.Errorf("batch 0: expected size 8MB, got %d", batches[0].size)
	}

	// Check batch 1
	if len(batches[1].files) != 2 {
		t.Errorf("batch 1: expected 2 files, got %d", len(batches[1].files))
	}
	if batches[1].size != 9*1024*1024 {
		t.Errorf("batch 1: expected size 9MB, got %d", batches[1].size)
	}

	// Check batch 2 (large file)
	if len(batches[2].files) != 1 {
		t.Errorf("batch 2: expected 1 file, got %d", len(batches[2].files))
	}
	if batches[2].size != 15*1024*1024 {
		t.Errorf("batch 2: expected size 15MB, got %d", batches[2].size)
	}

	// Check batch 3
	if len(batches[3].files) != 1 {
		t.Errorf("batch 3: expected 1 file, got %d", len(batches[3].files))
	}
	if batches[3].size != 1*1024*1024 {
		t.Errorf("batch 3: expected size 1MB, got %d", batches[3].size)
	}
}

func TestCreateManifestStructure(t *testing.T) {
	config := DefaultConfig()
	config.ChunkSizeMB = 50

	sourceAuth := map[string]interface{}{
		"username": "root",
		"password": "test123",
	}

	destAuth := map[string]interface{}{
		"username": "root",
		"password": "test456",
	}

	mc := NewManifestCreator(sourceAuth, destAuth, config)

	// Mock file enumeration (we'll test actual SSH separately)
	files := []FileInfo{
		{Path: "data/file1.txt", Size: 10 * 1024 * 1024},
		{Path: "data/file2.txt", Size: 40 * 1024 * 1024},
		{Path: "logs/app.log", Size: 5 * 1024 * 1024},
	}

	batches := mc.binPackFiles(files)

	// Should create 2 batches:
	// Batch 0: file1 (10MB) + file2 (40MB) = 50MB
	// Batch 1: app.log (5MB) = 5MB

	if len(batches) != 2 {
		t.Errorf("expected 2 batches, got %d", len(batches))
	}
}
