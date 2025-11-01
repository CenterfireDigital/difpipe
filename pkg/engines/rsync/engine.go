package rsync

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/larrydiffey/difpipe/pkg/core"
)

// Engine implements the TransferEngine interface using rsync
type Engine struct {
	binPath  string
	progress core.ProgressReporter
}

// New creates a new rsync engine
func New() *Engine {
	return &Engine{
		binPath: "rsync", // Assume rsync is in PATH
	}
}

// WithBinaryPath sets a custom rsync binary path
func (e *Engine) WithBinaryPath(path string) *Engine {
	e.binPath = path
	return e
}

// WithProgress sets a progress reporter
func (e *Engine) WithProgress(reporter core.ProgressReporter) *Engine {
	e.progress = reporter
	return e
}

// Name returns the engine name
func (e *Engine) Name() string {
	return "rsync"
}

// SupportsProtocol checks if rsync supports a protocol
func (e *Engine) SupportsProtocol(protocol string) bool {
	// Rsync supports local and SSH
	supported := map[string]bool{
		"local": true,
		"ssh":   true,
	}
	return supported[strings.ToLower(protocol)]
}

// Transfer performs the transfer using rsync
func (e *Engine) Transfer(ctx context.Context, opts *core.TransferOptions) (*core.TransferResult, error) {
	startTime := time.Now()

	result := &core.TransferResult{
		TransferID: generateTransferID(),
	}

	// Build rsync command
	args := e.buildCommand(opts)

	if e.progress != nil {
		e.progress.Start(0, "Starting rsync transfer")
	}

	// Execute command
	if opts.DryRun {
		result.Success = true
		result.Message = "Dry run completed (no actual transfer)"
		return result, nil
	}

	// Run the command with context
	cmd := exec.CommandContext(ctx, e.binPath, args...)

	// Capture stdout for progress parsing
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("create stderr pipe: %w", err)
	}

	// Start command
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start rsync: %w", err)
	}

	// Parse progress from stdout
	progressDone := make(chan struct{})
	go e.parseProgress(stdout, result, progressDone)

	// Capture errors from stderr
	var stderrOutput strings.Builder
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			stderrOutput.WriteString(scanner.Text() + "\n")
		}
	}()

	// Wait for completion
	err = cmd.Wait()
	close(progressDone)

	result.Duration = time.Since(startTime)

	if err != nil {
		result.Success = false
		result.Error = fmt.Errorf("rsync failed: %w\n%s", err, stderrOutput.String())
		if e.progress != nil {
			e.progress.Error(result.Error)
		}
		return result, result.Error
	}

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

// Estimate provides transfer estimation
func (e *Engine) Estimate(ctx context.Context, opts *core.TransferOptions) (*core.TransferEstimate, error) {
	// Run rsync with --dry-run and --stats
	dryRunOpts := *opts
	dryRunOpts.DryRun = true

	args := e.buildCommand(&dryRunOpts)
	args = append(args, "--stats")

	cmd := exec.CommandContext(ctx, e.binPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("estimate failed: %w", err)
	}

	estimate := &core.TransferEstimate{
		Recommendation:  core.StrategyRsync,
		RecommendReason: "Using rsync for efficient synchronization",
	}

	// Parse stats output
	e.parseStats(string(output), estimate)

	return estimate, nil
}

// buildCommand constructs the rsync command arguments
func (e *Engine) buildCommand(opts *core.TransferOptions) []string {
	args := []string{
		"-a",        // Archive mode (recursive, preserve attributes)
		"-v",        // Verbose
		"--progress", // Show progress
	}

	// Add compression if requested
	if opts.Compression != core.CompressionNone && opts.Compression != core.CompressionAuto {
		args = append(args, "-z")
	}

	// Add checkpoint/partial support
	if opts.Checkpoint {
		args = append(args, "--partial", "--partial-dir=.rsync-partial")
	}

	// Add dry run if requested
	if opts.DryRun {
		args = append(args, "--dry-run")
	}

	// Add filters
	if opts.Filters != nil {
		for _, include := range opts.Filters.Include {
			args = append(args, "--include", include)
		}
		for _, exclude := range opts.Filters.Exclude {
			args = append(args, "--exclude", exclude)
		}
	}

	// Add source and destination
	// Important: Add trailing slashes to ensure rsync copies contents, not directory
	source := opts.Source
	dest := opts.Destination

	// Add trailing slash to source if it's a directory (ensures contents are copied)
	if !strings.HasSuffix(source, "/") {
		source = source + "/"
	}

	// Add trailing slash to destination (ensures files go into the directory)
	if !strings.HasSuffix(dest, "/") {
		dest = dest + "/"
	}

	args = append(args, source, dest)

	return args
}

// parseProgress parses rsync progress output
func (e *Engine) parseProgress(stdout interface{}, result *core.TransferResult, done <-chan struct{}) {
	scanner := bufio.NewScanner(stdout.(interface {
		Read(p []byte) (n int, err error)
	}))

	// Rsync progress line format:
	// 1,234,567  45%  123.45kB/s    0:00:12
	progressRegex := regexp.MustCompile(`(\d+(?:,\d+)*)\s+(\d+)%\s+([\d.]+\w+/s)`)

	for {
		select {
		case <-done:
			return
		default:
			if !scanner.Scan() {
				return
			}
		}

		line := scanner.Text()

		// Parse progress line
		matches := progressRegex.FindStringSubmatch(line)
		if len(matches) >= 4 {
			// Parse bytes transferred
			bytesStr := strings.ReplaceAll(matches[1], ",", "")
			if bytes, err := strconv.ParseInt(bytesStr, 10, 64); err == nil {
				result.BytesDone = bytes
			}

			// Parse percentage
			if percent, err := strconv.ParseInt(matches[2], 10, 64); err == nil {
				_ = percent // Could use for progress reporting
			}

			// Speed is in matches[3]
			if e.progress != nil {
				e.progress.Update(result.BytesDone, line)
			}
		}
	}
}

// parseStats parses rsync stats output
func (e *Engine) parseStats(output string, estimate *core.TransferEstimate) {
	lines := strings.Split(output, "\n")

	// Parse stats from output
	// Format:
	// Number of files: 1,234 (reg: 1,000, dir: 234)
	// Number of created files: 1,234
	// Total file size: 123,456,789 bytes
	fileCountRegex := regexp.MustCompile(`Number of files: (\d+(?:,\d+)*)`)
	sizeRegex := regexp.MustCompile(`Total file size: (\d+(?:,\d+)*) bytes`)

	for _, line := range lines {
		// Parse file count
		if matches := fileCountRegex.FindStringSubmatch(line); len(matches) > 1 {
			countStr := strings.ReplaceAll(matches[1], ",", "")
			if count, err := strconv.ParseInt(countStr, 10, 64); err == nil {
				estimate.FilesTotal = count
			}
		}

		// Parse total size
		if matches := sizeRegex.FindStringSubmatch(line); len(matches) > 1 {
			sizeStr := strings.ReplaceAll(matches[1], ",", "")
			if size, err := strconv.ParseInt(sizeStr, 10, 64); err == nil {
				estimate.BytesTotal = size
			}
		}
	}

	// Estimate time based on typical rsync speed (50 MB/s)
	if estimate.BytesTotal > 0 {
		estimatedSeconds := float64(estimate.BytesTotal) / (50 * 1024 * 1024)
		estimate.EstimatedTime = time.Duration(estimatedSeconds * float64(time.Second))
		estimate.EstimatedSpeed = "~50 MB/s"
	}
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
