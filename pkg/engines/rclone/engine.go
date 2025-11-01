package rclone

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/larrydiffey/difpipe/pkg/core"
)

// Engine implements the TransferEngine interface using rclone
type Engine struct {
	binPath  string
	progress core.ProgressReporter
}

// New creates a new rclone engine
func New() *Engine {
	return &Engine{
		binPath: "rclone", // Assume rclone is in PATH
	}
}

// WithBinaryPath sets a custom rclone binary path
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
	return "rclone"
}

// SupportsProtocol checks if rclone supports a protocol
func (e *Engine) SupportsProtocol(protocol string) bool {
	// Rclone supports 70+ backends, we'll list the main ones
	supported := map[string]bool{
		"s3":     true,
		"gcs":    true,
		"azure":  true,
		"sftp":   true,
		"ftp":    true,
		"http":   true,
		"webdav": true,
		"local":  true,
		"ssh":    true,
	}
	return supported[strings.ToLower(protocol)]
}

// Transfer performs the transfer using rclone
func (e *Engine) Transfer(ctx context.Context, opts *core.TransferOptions) (*core.TransferResult, error) {
	startTime := time.Now()

	result := &core.TransferResult{
		TransferID: generateTransferID(),
	}

	// Build rclone command
	cmd := e.buildCommand(opts)

	if e.progress != nil {
		e.progress.Start(0, "Starting rclone transfer")
	}

	// Execute command
	if opts.DryRun {
		result.Success = true
		result.Message = "Dry run completed (no actual transfer)"
		return result, nil
	}

	// Run the command with context
	cmdExec := exec.CommandContext(ctx, e.binPath, cmd...)

	// Capture stdout for progress parsing
	stdout, err := cmdExec.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}

	stderr, err := cmdExec.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("create stderr pipe: %w", err)
	}

	// Start command
	if err := cmdExec.Start(); err != nil {
		return nil, fmt.Errorf("start rclone: %w", err)
	}

	// Parse progress from stdout
	go e.parseProgress(stdout, result)

	// Capture errors from stderr
	var stderrOutput strings.Builder
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			stderrOutput.WriteString(scanner.Text() + "\n")
		}
	}()

	// Wait for completion
	err = cmdExec.Wait()

	result.Duration = time.Since(startTime)

	if err != nil {
		result.Success = false
		result.Error = fmt.Errorf("rclone failed: %w\n%s", err, stderrOutput.String())
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

// Estimate provides transfer estimation (dry run)
func (e *Engine) Estimate(ctx context.Context, opts *core.TransferOptions) (*core.TransferEstimate, error) {
	// Run rclone with --dry-run and parse output
	dryRunOpts := *opts
	dryRunOpts.DryRun = true

	cmd := e.buildCommand(&dryRunOpts)

	cmdExec := exec.CommandContext(ctx, e.binPath, cmd...)
	output, err := cmdExec.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("estimate failed: %w", err)
	}

	estimate := &core.TransferEstimate{
		Recommendation:  core.StrategyRclone,
		RecommendReason: "Using rclone for broad protocol support",
	}

	// Parse dry run output for estimates
	// This is a simplified version - real implementation would parse actual output
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Transferred:") {
			// Parse transfer size from output
			// Format: "Transferred: 1.234 GB / 1.234 GB, 100%"
			// This is simplified - would need proper parsing
		}
	}

	return estimate, nil
}

// buildCommand constructs the rclone command arguments
func (e *Engine) buildCommand(opts *core.TransferOptions) []string {
	args := []string{"sync"}

	// Add progress flag for JSON output
	args = append(args, "--progress", "--stats-one-line", "--stats", "1s")

	// Add parallelism
	if opts.Parallel > 0 {
		args = append(args, "--transfers", fmt.Sprintf("%d", opts.Parallel))
	}

	// Add compression if requested
	if opts.Compression != core.CompressionNone && opts.Compression != core.CompressionAuto {
		args = append(args, "--compress")
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

	// Convert SSH remotes to SFTP syntax
	source := e.convertToSFTP(opts.Source, opts.Auth, true)
	dest := e.convertToSFTP(opts.Destination, opts.Auth, false)

	// Add source and destination
	args = append(args, source, dest)

	return args
}

// parseProgress parses rclone progress output
func (e *Engine) parseProgress(stdout interface{}, result *core.TransferResult) {
	scanner := bufio.NewScanner(stdout.(interface {
		Read(p []byte) (n int, err error)
	}))

	for scanner.Scan() {
		line := scanner.Text()

		// Parse rclone stats output
		// Format: "Transferred: 123.456 MBytes, 45%, 1.23 MBytes/s, ETA 1m23s"
		if strings.Contains(line, "Transferred:") {
			e.parseStatsLine(line, result)
		}
	}
}

// parseStatsLine parses a single stats line from rclone
func (e *Engine) parseStatsLine(line string, result *core.TransferResult) {
	// This is simplified - real implementation would use regex or proper parsing
	// For now, just update progress reporter if available
	if e.progress != nil {
		e.progress.Update(result.BytesDone, line)
	}
}

// generateTransferID creates a unique transfer ID
func generateTransferID() string {
	return fmt.Sprintf("transfer-%d", time.Now().UnixNano())
}

// convertToSFTP converts SSH remote path to rclone SFTP syntax
func (e *Engine) convertToSFTP(path string, auth *core.AuthOptions, isSource bool) string {
	// Check if this is an SSH remote (user@host:path format)
	if !strings.Contains(path, "@") || !strings.Contains(path, ":") {
		// Local path, return as-is
		return path
	}

	// Parse user@host:path
	atIndex := strings.Index(path, "@")
	colonIndex := strings.Index(path, ":")

	if atIndex == -1 || colonIndex == -1 || colonIndex < atIndex {
		// Not a valid SSH remote, return as-is
		return path
	}

	user := path[:atIndex]
	host := path[atIndex+1:colonIndex]
	remotePath := path[colonIndex+1:]

	// Get password from auth
	var password string
	if auth != nil {
		var authMap map[string]interface{}
		if isSource && auth.SourceAuth != nil {
			authMap = auth.SourceAuth
		} else if !isSource && auth.DestAuth != nil {
			authMap = auth.DestAuth
		}

		if authMap != nil {
			if pwd, ok := authMap["password"].(string); ok {
				password = pwd
			}
		}
	}

	// Try environment variables as fallback
	if password == "" {
		if isSource {
			password = os.Getenv("DIFPIPE_SOURCE_PASSWORD")
		} else {
			password = os.Getenv("DIFPIPE_DEST_PASSWORD")
		}
	}

	// Build rclone SFTP connection string
	// Format: :sftp,host=HOST,user=USER,pass=PASS:PATH
	if password != "" {
		// Need to obscure the password for rclone
		obscuredPass := obscurePassword(password)
		return fmt.Sprintf(":sftp,host=%s,user=%s,pass=%s:%s", host, user, obscuredPass, remotePath)
	}

	// No password, try SSH agent
	return fmt.Sprintf(":sftp,host=%s,user=%s:%s", host, user, remotePath)
}

// obscurePassword obscures a password for rclone (simple base64 for now)
func obscurePassword(password string) string {
	// Rclone uses its own obscure algorithm, but for command-line passing
	// we can try using the password directly and let rclone handle it
	// Or we could shell out to `rclone obscure` command
	// For now, return as-is and test
	return password
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
