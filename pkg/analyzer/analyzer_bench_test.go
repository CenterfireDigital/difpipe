package analyzer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkAnalyze benchmarks the file analyzer
func BenchmarkAnalyze(b *testing.B) {
	// Create temp directory with test files
	tmpDir, err := os.MkdirTemp("", "difpipe-bench-*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create 100 test files
	for i := 0; i < 100; i++ {
		path := filepath.Join(tmpDir, fmt.Sprintf("file%d.txt", i))
		if err := os.WriteFile(path, []byte("test content"), 0644); err != nil {
			b.Fatalf("Failed to create test file: %v", err)
		}
	}

	analyzer := New()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = analyzer.Analyze(context.Background(), tmpDir)
	}
}

// BenchmarkDetectProtocol benchmarks protocol detection
func BenchmarkDetectProtocol(b *testing.B) {
	testCases := []string{
		"/local/path",
		"s3://bucket/key",
		"gs://bucket/key",
		"user@host:/path",
		"https://example.com",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, tc := range testCases{
			_ = detectProtocol(tc)
		}
	}
}

// BenchmarkFormatBytes benchmarks byte formatting
func BenchmarkFormatBytes(b *testing.B) {
	sizes := []int64{
		100,
		1024,
		1024 * 1024,
		1024 * 1024 * 1024,
		1024 * 1024 * 1024 * 1024,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, size := range sizes {
			_ = formatBytes(size)
		}
	}
}
