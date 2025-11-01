package analyzer

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/larrydiffey/difpipe/pkg/core"
)

// FileAnalyzer analyzes files and recommends transfer strategies
type FileAnalyzer struct {
	smallFileThreshold int64   // Bytes
	largeFileThreshold int64   // Bytes
	manyFilesCount     int     // File count
	smallFilePercent   float64 // Percentage
	largeFilePercent   float64 // Percentage
	fewFilesCount      int     // File count
	maxSampleSize      int     // Max files to sample
}

// New creates a new file analyzer with default thresholds
func New() *FileAnalyzer {
	return &FileAnalyzer{
		smallFileThreshold: 10 * 1024,           // 10 KB
		largeFileThreshold: 100 * 1024 * 1024,   // 100 MB
		manyFilesCount:     1000,                // 1000 files
		smallFilePercent:   80.0,                // 80%
		largeFilePercent:   50.0,                // 50%
		fewFilesCount:      10,                  // 10 files
		maxSampleSize:      10000,               // 10k files
	}
}

// WithThresholds sets custom thresholds for analysis
func (a *FileAnalyzer) WithThresholds(smallKB, largeMB, manyFiles, fewFiles, maxSample int, smallPercent, largePercent float64) *FileAnalyzer {
	a.smallFileThreshold = int64(smallKB) * 1024
	a.largeFileThreshold = int64(largeMB) * 1024 * 1024
	a.manyFilesCount = manyFiles
	a.fewFilesCount = fewFiles
	a.smallFilePercent = smallPercent
	a.largeFilePercent = largePercent
	a.maxSampleSize = maxSample
	return a
}

// Analyze examines the source path and returns analysis
func (a *FileAnalyzer) Analyze(ctx context.Context, source string) (*core.FileAnalysis, error) {
	startTime := time.Now()

	analysis := &core.FileAnalysis{
		FileTypes:      make(map[string]int64),
		SourceProtocol: detectProtocol(source),
	}

	// Check if source is local
	if !isLocalPath(source) {
		// For remote sources, return basic analysis
		// Actual analysis would require connecting to remote
		analysis.Recommendation = core.StrategyRclone
		analysis.RecommendReason = "Remote source, using rclone for broad protocol support"
		return analysis, nil
	}

	// Analyze local filesystem
	err := a.analyzeLocal(ctx, source, analysis)
	if err != nil {
		return nil, fmt.Errorf("analyze local: %w", err)
	}

	// Calculate derived metrics
	if analysis.TotalFiles > 0 {
		analysis.AverageFileSize = analysis.TotalSize / analysis.TotalFiles
	}

	analysis.SampleTime = time.Since(startTime)

	// Determine recommendation
	analysis.Recommendation = a.recommendStrategy(analysis)
	analysis.RecommendReason = a.getRecommendationReason(analysis)

	return analysis, nil
}

// analyzeLocal analyzes a local filesystem path
func (a *FileAnalyzer) analyzeLocal(ctx context.Context, source string, analysis *core.FileAnalysis) error {
	fileCount := int64(0)
	sampleInterval := 1

	// Quick scan to get total file count
	err := filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if !d.IsDir() {
			fileCount++
		}
		return nil
	})

	if err != nil {
		return err
	}

	// Calculate sampling interval if needed
	if fileCount > int64(a.maxSampleSize) {
		sampleInterval = int(fileCount / int64(a.maxSampleSize))
	}

	// Analyze with sampling
	currentFile := int64(0)
	err = filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		// Check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err != nil {
			return nil // Skip errors
		}

		if d.IsDir() {
			return nil
		}

		currentFile++

		// Sample files if large directory
		if sampleInterval > 1 && currentFile%int64(sampleInterval) != 0 {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		size := info.Size()

		// Update totals (scale up if sampling)
		if sampleInterval > 1 {
			analysis.TotalSize += size * int64(sampleInterval)
			analysis.TotalFiles += int64(sampleInterval)
		} else {
			analysis.TotalSize += size
			analysis.TotalFiles++
		}

		// Classify by size
		if size < a.smallFileThreshold {
			analysis.SmallFiles++
		} else if size > a.largeFileThreshold {
			analysis.LargeFiles++
		} else {
			analysis.MediumFiles++
		}

		// Track file types
		ext := strings.ToLower(filepath.Ext(path))
		if ext == "" {
			ext = "(no extension)"
		}
		analysis.FileTypes[ext]++

		return nil
	})

	return err
}

// recommendStrategy recommends the best transfer strategy
func (a *FileAnalyzer) recommendStrategy(analysis *core.FileAnalysis) core.Strategy {
	// If very few files, any strategy works
	if analysis.TotalFiles < int64(a.fewFilesCount) {
		return core.StrategyRsync
	}

	// If mostly small files, use tar streaming
	smallFileRatio := float64(analysis.SmallFiles) / float64(analysis.TotalFiles) * 100
	if smallFileRatio > a.smallFilePercent && analysis.TotalFiles > int64(a.manyFilesCount) {
		return core.StrategyTar
	}

	// If mostly large files, use rsync
	largeFileRatio := float64(analysis.LargeFiles) / float64(analysis.TotalFiles) * 100
	if largeFileRatio > a.largeFilePercent {
		return core.StrategyRsync
	}

	// Default to rclone for mixed workloads
	return core.StrategyRclone
}

// getRecommendationReason explains why a strategy was chosen
func (a *FileAnalyzer) getRecommendationReason(analysis *core.FileAnalysis) string {
	strategy := analysis.Recommendation

	switch strategy {
	case core.StrategyTar:
		return fmt.Sprintf(
			"Tar streaming recommended: %d files, %.1f%% are small (<%d KB)",
			analysis.TotalFiles,
			float64(analysis.SmallFiles)/float64(analysis.TotalFiles)*100,
			a.smallFileThreshold/1024,
		)

	case core.StrategyRsync:
		if analysis.TotalFiles < int64(a.fewFilesCount) {
			return fmt.Sprintf("Rsync recommended: only %d files to transfer", analysis.TotalFiles)
		}
		return fmt.Sprintf(
			"Rsync recommended: %.1f%% are large files (>%d MB)",
			float64(analysis.LargeFiles)/float64(analysis.TotalFiles)*100,
			a.largeFileThreshold/(1024*1024),
		)

	case core.StrategyRclone:
		return fmt.Sprintf(
			"Rclone recommended: mixed workload with %d files, avg size %s",
			analysis.TotalFiles,
			formatBytes(analysis.AverageFileSize),
		)

	default:
		return "Strategy selected based on source/destination protocols"
	}
}

// Recommend suggests the best strategy for given analysis
func (a *FileAnalyzer) Recommend(analysis *core.FileAnalysis) core.Strategy {
	return a.recommendStrategy(analysis)
}

// AnalyzeTransfer analyzes both source and destination to determine optimal strategy
func (a *FileAnalyzer) AnalyzeTransfer(ctx context.Context, source, destination string) (*core.FileAnalysis, error) {
	// Detect protocols for both source and destination
	sourceProto := detectProtocol(source)
	destProto := detectProtocol(destination)

	// Check for remote-to-remote SSH transfer
	if sourceProto == core.ProtocolSSH && destProto == core.ProtocolSSH {
		return &core.FileAnalysis{
			SourceProtocol:  sourceProto,
			DestProtocol:    destProto,
			Recommendation:  core.StrategyProxy,
			RecommendReason: "Remote-to-remote SSH transfer, using streaming proxy",
		}, nil
	}

	// For other cases, use the existing Analyze method
	analysis, err := a.Analyze(ctx, source)
	if err != nil {
		return nil, err
	}

	// Set destination protocol
	analysis.DestProtocol = destProto

	return analysis, nil
}

// detectProtocol detects the protocol from a path string
func detectProtocol(path string) core.Protocol {
	switch {
	case strings.HasPrefix(path, "s3://"):
		return core.ProtocolS3
	case strings.HasPrefix(path, "gs://"), strings.HasPrefix(path, "gcs://"):
		return core.ProtocolGCS
	case strings.HasPrefix(path, "azure://"), strings.HasPrefix(path, "wasb://"):
		return core.ProtocolAzure
	case strings.HasPrefix(path, "http://"), strings.HasPrefix(path, "https://"):
		return core.ProtocolHTTP
	case strings.HasPrefix(path, "ftp://"), strings.HasPrefix(path, "ftps://"):
		return core.ProtocolFTP
	case strings.Contains(path, "@") && strings.Contains(path, ":"):
		// Looks like user@host:path
		return core.ProtocolSSH
	default:
		return core.ProtocolLocal
	}
}

// isLocalPath checks if a path is local
func isLocalPath(path string) bool {
	protocol := detectProtocol(path)
	return protocol == core.ProtocolLocal
}

// formatBytes formats byte count as human-readable string
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp])
}
