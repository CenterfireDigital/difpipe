package analyzer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/larrydiffey/difpipe/pkg/core"
)

func TestNew(t *testing.T) {
	a := New()
	if a == nil {
		t.Fatal("Expected non-nil analyzer")
	}
}

func TestDetectProtocol(t *testing.T) {
	tests := []struct {
		path     string
		expected core.Protocol
	}{
		{"/local/path", core.ProtocolLocal},
		{"s3://bucket/key", core.ProtocolS3},
		{"gs://bucket/key", core.ProtocolGCS},
		{"user@host:/path", core.ProtocolSSH},
		{"https://example.com", core.ProtocolHTTP},
		{"ftp://example.com", core.ProtocolFTP},
	}

	for _, tt := range tests {
		result := detectProtocol(tt.path)
		if result != tt.expected {
			t.Errorf("detectProtocol(%s) = %s, want %s", tt.path, result, tt.expected)
		}
	}
}

func TestAnalyze_NonExistent(t *testing.T) {
	a := New()
	_, err := a.Analyze(context.Background(), "/non/existent/path")
	// Should not error, just return empty results for non-local
	if err != nil {
		t.Errorf("Expected no error for non-existent path, got %v", err)
	}
}

func TestAnalyze_LocalDirectory(t *testing.T) {
	// Create temp directory with test files
	tmpDir, err := os.MkdirTemp("", "difpipe-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test files
	files := []struct {
		name string
		size int64
	}{
		{"small1.txt", 1024},           // 1 KB - small
		{"small2.log", 5000},           // 5 KB - small
		{"medium1.dat", 5 * 1024 * 1024}, // 5 MB - medium
		{"large1.bin", 200 * 1024 * 1024}, // 200 MB - large
	}

	for _, f := range files {
		path := filepath.Join(tmpDir, f.name)
		file, err := os.Create(path)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		if err := file.Truncate(f.size); err != nil {
			file.Close()
			t.Fatalf("Failed to truncate file: %v", err)
		}
		file.Close()
	}

	// Analyze
	a := New()
	analysis, err := a.Analyze(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Verify results
	if analysis.TotalFiles != 4 {
		t.Errorf("Expected 4 files, got %d", analysis.TotalFiles)
	}

	if analysis.SmallFiles != 2 {
		t.Errorf("Expected 2 small files, got %d", analysis.SmallFiles)
	}

	if analysis.MediumFiles != 1 {
		t.Errorf("Expected 1 medium file, got %d", analysis.MediumFiles)
	}

	if analysis.LargeFiles != 1 {
		t.Errorf("Expected 1 large file, got %d", analysis.LargeFiles)
	}

	// Check recommendation for large file
	if analysis.Recommendation != core.StrategyRsync {
		t.Errorf("Expected rsync strategy for large files, got %s", analysis.Recommendation)
	}
}

func TestAnalyze_ManySmallFiles(t *testing.T) {
	// Create temp directory with many small files
	tmpDir, err := os.MkdirTemp("", "difpipe-test-many-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create 1200 small files
	for i := 0; i < 1200; i++ {
		path := filepath.Join(tmpDir, fmt.Sprintf("file%d.txt", i))
		if err := os.WriteFile(path, []byte("small"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	// Analyze
	a := New()
	analysis, err := a.Analyze(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Should recommend tar for many small files
	if analysis.Recommendation != core.StrategyTar {
		t.Errorf("Expected tar strategy for many small files, got %s", analysis.Recommendation)
	}
}

func TestRecommendStrategy_FewFiles(t *testing.T) {
	a := New()
	analysis := &core.FileAnalysis{
		TotalFiles:  5,
		SmallFiles:  5,
		MediumFiles: 0,
		LargeFiles:  0,
	}

	strategy := a.recommendStrategy(analysis)
	if strategy != core.StrategyRsync {
		t.Errorf("Expected rsync for few files, got %s", strategy)
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{100, "100 B"},
		{1024, "1.0 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
	}

	for _, tt := range tests {
		result := formatBytes(tt.bytes)
		if result != tt.expected {
			t.Errorf("formatBytes(%d) = %s, want %s", tt.bytes, result, tt.expected)
		}
	}
}
