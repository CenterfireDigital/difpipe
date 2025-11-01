package tarstream

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/larrydiffey/difpipe/pkg/core"
)

// Engine implements the TransferEngine interface using tar streaming
// This is optimal for many small files where tar's single-stream approach
// is more efficient than transferring files individually
type Engine struct {
	progress core.ProgressReporter
}

// New creates a new tar streaming engine
func New() *Engine {
	return &Engine{}
}

// WithProgress sets a progress reporter
func (e *Engine) WithProgress(reporter core.ProgressReporter) *Engine {
	e.progress = reporter
	return e
}

// Name returns the engine name
func (e *Engine) Name() string {
	return "tar"
}

// SupportsProtocol checks if tar streaming supports a protocol
func (e *Engine) SupportsProtocol(protocol string) bool {
	// Tar streaming works best for local to remote via SSH
	supported := map[string]bool{
		"local": true,
		"ssh":   true,
	}
	return supported[strings.ToLower(protocol)]
}

// Transfer performs the transfer using tar streaming
func (e *Engine) Transfer(ctx context.Context, opts *core.TransferOptions) (*core.TransferResult, error) {
	startTime := time.Now()

	result := &core.TransferResult{
		TransferID: generateTransferID(),
	}

	if e.progress != nil {
		e.progress.Start(0, "Starting tar stream transfer")
	}

	// Execute transfer
	if opts.DryRun {
		result.Success = true
		result.Message = "Dry run completed (no actual transfer)"
		return result, nil
	}

	// Check if source is local
	if !isLocalPath(opts.Source) {
		return nil, fmt.Errorf("tar streaming requires local source path")
	}

	// Determine if destination is local or remote
	if isLocalPath(opts.Destination) {
		err := e.transferLocal(ctx, opts, result)
		if err != nil {
			result.Success = false
			result.Error = err
			if e.progress != nil {
				e.progress.Error(err)
			}
			return result, err
		}
	} else {
		err := e.transferRemote(ctx, opts, result)
		if err != nil {
			result.Success = false
			result.Error = err
			if e.progress != nil {
				e.progress.Error(err)
			}
			return result, err
		}
	}

	result.Duration = time.Since(startTime)
	result.Success = true
	result.Message = "Transfer completed successfully"

	// Calculate average speed
	if result.Duration > 0 && result.BytesDone > 0 {
		bytesPerSec := float64(result.BytesDone) / result.Duration.Seconds()
		result.AverageSpeed = formatSpeed(int64(bytesPerSec))
	}

	if e.progress != nil {
		e.progress.Complete(result.Message)
	}

	return result, nil
}

// transferLocal performs local to local tar transfer
func (e *Engine) transferLocal(ctx context.Context, opts *core.TransferOptions, result *core.TransferResult) error {
	// Create destination directory if needed
	if err := os.MkdirAll(opts.Destination, 0755); err != nil {
		return fmt.Errorf("create destination: %w", err)
	}

	// Create tar file
	tarPath := filepath.Join(opts.Destination, "transfer.tar")
	if opts.Compression == core.CompressionGzip || opts.Compression == core.CompressionAuto {
		tarPath += ".gz"
	}

	outFile, err := os.Create(tarPath)
	if err != nil {
		return fmt.Errorf("create tar file: %w", err)
	}
	defer outFile.Close()

	// Create tar writer with optional compression
	var writer io.Writer = outFile
	var gzipWriter *gzip.Writer
	if opts.Compression == core.CompressionGzip || opts.Compression == core.CompressionAuto {
		gzipWriter = gzip.NewWriter(outFile)
		writer = gzipWriter
		defer gzipWriter.Close()
	}

	tarWriter := tar.NewWriter(writer)
	defer tarWriter.Close()

	// Walk source directory and add files to tar
	err = e.walkAndTar(ctx, opts.Source, tarWriter, result, opts.Filters)
	if err != nil {
		return fmt.Errorf("tar creation: %w", err)
	}

	return nil
}

// transferRemote performs local to remote tar transfer via SSH
func (e *Engine) transferRemote(ctx context.Context, opts *core.TransferOptions, result *core.TransferResult) error {
	// For now, this is a simplified implementation
	// A full implementation would use SSH to pipe tar directly
	// For v0.2.0, we'll create the tar locally and use rsync/scp to transfer
	return fmt.Errorf("remote tar streaming not yet implemented - use local destination")
}

// walkAndTar walks the source directory and adds files to tar archive
func (e *Engine) walkAndTar(ctx context.Context, source string, tarWriter *tar.Writer, result *core.TransferResult, filters *core.FilterOptions) error {
	return filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		// Check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}

		// Skip root
		if relPath == "." {
			return nil
		}

		// Apply filters
		if filters != nil {
			if !matchesFilters(relPath, filters) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		// Get file info
		info, err := d.Info()
		if err != nil {
			return err
		}

		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		// Use relative path in archive
		header.Name = relPath

		// Write header
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		// If it's a file, write contents
		if !d.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			written, err := io.Copy(tarWriter, file)
			if err != nil {
				return err
			}

			result.BytesDone += written
			result.FilesDone++

			if e.progress != nil {
				e.progress.Update(result.BytesDone, fmt.Sprintf("Adding: %s", relPath))
			}
		} else {
			result.FilesDone++
		}

		return nil
	})
}

// Estimate provides transfer estimation
func (e *Engine) Estimate(ctx context.Context, opts *core.TransferOptions) (*core.TransferEstimate, error) {
	estimate := &core.TransferEstimate{
		Recommendation:  core.StrategyTar,
		RecommendReason: "Using tar streaming for many small files",
	}

	// Quick scan to estimate
	var totalSize int64
	var fileCount int64

	err := filepath.WalkDir(opts.Source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if !d.IsDir() {
			info, err := d.Info()
			if err == nil {
				totalSize += info.Size()
				fileCount++
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	estimate.BytesTotal = totalSize
	estimate.FilesTotal = fileCount

	// Estimate time (tar is fast for small files, ~100 MB/s)
	if totalSize > 0 {
		estimatedSeconds := float64(totalSize) / (100 * 1024 * 1024)
		estimate.EstimatedTime = time.Duration(estimatedSeconds * float64(time.Second))
		estimate.EstimatedSpeed = "~100 MB/s"
	}

	return estimate, nil
}

// matchesFilters checks if a path matches include/exclude filters
func matchesFilters(path string, filters *core.FilterOptions) bool {
	// If includes are specified, must match at least one
	if len(filters.Include) > 0 {
		matched := false
		for _, pattern := range filters.Include {
			if match, _ := filepath.Match(pattern, filepath.Base(path)); match {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check excludes
	for _, pattern := range filters.Exclude {
		if match, _ := filepath.Match(pattern, filepath.Base(path)); match {
			return false
		}
	}

	return true
}

// isLocalPath checks if a path is local
func isLocalPath(path string) bool {
	// Simple check - if it contains @ or :// it's likely remote
	return !strings.Contains(path, "@") && !strings.Contains(path, "://")
}

// generateTransferID creates a unique transfer ID
func generateTransferID() string {
	return fmt.Sprintf("transfer-%d", time.Now().UnixNano())
}

// formatSpeed formats bytes per second as human-readable string
func formatSpeed(bytesPerSec int64) string {
	const unit = 1024
	if bytesPerSec < unit {
		return fmt.Sprintf("%d B/s", bytesPerSec)
	}
	div, exp := int64(unit), 0
	for n := bytesPerSec / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB/s", "MB/s", "GB/s", "TB/s"}
	return fmt.Sprintf("%.1f %s", float64(bytesPerSec)/float64(div), units[exp])
}
