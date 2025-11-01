package proxy

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/larrydiffey/difpipe/pkg/core"
	"github.com/larrydiffey/difpipe/pkg/stream"
	"github.com/larrydiffey/difpipe/pkg/transport"
)

// Engine implements TransferEngine for remote-to-remote streaming proxy
type Engine struct {
	transport transport.Transport
	progress  core.ProgressReporter
}

// New creates a new proxy engine
func New() *Engine {
	return &Engine{
		transport: transport.New(),
	}
}

// WithProgress sets a progress reporter
func (e *Engine) WithProgress(reporter core.ProgressReporter) *Engine {
	e.progress = reporter
	return e
}

// Name returns the engine name
func (e *Engine) Name() string {
	return "proxy"
}

// SupportsProtocol checks if proxy supports a protocol
func (e *Engine) SupportsProtocol(protocol string) bool {
	// Proxy currently only supports SSH
	return protocol == "ssh"
}

// Transfer performs the remote-to-remote transfer
func (e *Engine) Transfer(ctx context.Context, opts *core.TransferOptions) (*core.TransferResult, error) {
	startTime := time.Now()

	result := &core.TransferResult{
		TransferID: generateTransferID(),
	}

	if e.progress != nil {
		e.progress.Start(0, "Starting proxy transfer")
	}

	// Execute transfer
	if opts.DryRun {
		result.Success = true
		result.Message = "Dry run completed (no actual transfer)"
		return result, nil
	}

	// Parse source and destination
	sourceLoc, err := transport.ParseRemotePath(opts.Source)
	if err != nil {
		return nil, fmt.Errorf("parse source: %w", err)
	}

	destLoc, err := transport.ParseRemotePath(opts.Destination)
	if err != nil {
		return nil, fmt.Errorf("parse destination: %w", err)
	}

	// Get authentication
	sourceAuth, destAuth, err := e.getAuthentication(opts, sourceLoc, destLoc)
	if err != nil {
		return nil, fmt.Errorf("get authentication: %w", err)
	}

	// Connect to source
	sourceConfig := sourceLoc.SSHConfig(sourceAuth)
	sourceClient, err := e.transport.Connect(ctx, sourceConfig)
	if err != nil {
		return nil, fmt.Errorf("connect to source: %w", err)
	}
	defer e.transport.Close(sourceClient)

	// Connect to destination
	destConfig := destLoc.SSHConfig(destAuth)
	destClient, err := e.transport.Connect(ctx, destConfig)
	if err != nil {
		return nil, fmt.Errorf("connect to destination: %w", err)
	}
	defer e.transport.Close(destClient)

	// Get file size from source
	fileSize, err := transport.GetFileSize(ctx, e.transport, sourceClient, sourceLoc.Path)
	if err != nil {
		return nil, fmt.Errorf("get file size: %w", err)
	}

	result.BytesTotal = fileSize

	if e.progress != nil {
		e.progress.Start(fileSize, fmt.Sprintf("Transferring %d bytes", fileSize))
	}

	// Start source stream: cat file
	sourceCmd := fmt.Sprintf("cat %s", sourceLoc.Path)
	sourceStream, err := e.transport.StreamCommand(ctx, sourceClient, sourceCmd)
	if err != nil {
		return nil, fmt.Errorf("start source stream: %w", err)
	}
	defer sourceStream.Close()

	// Start destination stream: cat > file
	destCmd := fmt.Sprintf("cat > %s", destLoc.Path)
	destStream, err := e.transport.StreamWrite(ctx, destClient, destCmd)
	if err != nil {
		return nil, fmt.Errorf("start destination stream: %w", err)
	}
	defer destStream.Close()

	// Create streaming pipeline
	pipelineConfig := &stream.Config{
		BufferSize: 1024 * 1024, // 1MB buffer
		ProgressFunc: func(bytesTransferred int64, speed float64) {
			if e.progress != nil {
				e.progress.Update(bytesTransferred, fmt.Sprintf("%.1f MB/s", speed/(1024*1024)))
			}
		},
	}

	pipeline := stream.New(sourceStream, destStream, pipelineConfig)

	// Start pipeline
	if err := pipeline.Start(ctx); err != nil {
		result.Success = false
		result.Error = err
		if e.progress != nil {
			e.progress.Error(err)
		}
		return result, fmt.Errorf("pipeline error: %w", err)
	}

	// Get final stats
	stats := pipeline.Stats()
	result.BytesDone = stats.BytesWritten
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

// Estimate provides transfer estimation
func (e *Engine) Estimate(ctx context.Context, opts *core.TransferOptions) (*core.TransferEstimate, error) {
	estimate := &core.TransferEstimate{
		Recommendation:  core.Strategy("proxy"),
		RecommendReason: "Remote-to-remote transfer using streaming proxy",
	}

	// Parse source
	sourceLoc, err := transport.ParseRemotePath(opts.Source)
	if err != nil {
		return nil, fmt.Errorf("parse source: %w", err)
	}

	// Get authentication
	sourceAuth, _, err := e.getAuthentication(opts, sourceLoc, nil)
	if err != nil {
		return nil, fmt.Errorf("get authentication: %w", err)
	}

	// Connect to source to get file size
	sourceConfig := sourceLoc.SSHConfig(sourceAuth)
	sourceClient, err := e.transport.Connect(ctx, sourceConfig)
	if err != nil {
		return nil, fmt.Errorf("connect to source: %w", err)
	}
	defer e.transport.Close(sourceClient)

	// Get file size
	fileSize, err := transport.GetFileSize(ctx, e.transport, sourceClient, sourceLoc.Path)
	if err != nil {
		return nil, fmt.Errorf("get file size: %w", err)
	}

	estimate.BytesTotal = fileSize
	estimate.FilesTotal = 1

	// Estimate time (assume 50 MB/s for SSH transfers)
	if fileSize > 0 {
		estimatedSeconds := float64(fileSize) / (50 * 1024 * 1024)
		estimate.EstimatedTime = time.Duration(estimatedSeconds * float64(time.Second))
		estimate.EstimatedSpeed = "~50 MB/s"
	}

	return estimate, nil
}

// getAuthentication gets authentication methods for source and destination
func (e *Engine) getAuthentication(opts *core.TransferOptions, sourceLoc, destLoc *transport.RemoteLocation) (transport.AuthMethod, transport.AuthMethod, error) {
	var sourceAuth, destAuth transport.AuthMethod

	// Try environment variable for source password
	if sourcePassword := os.Getenv("DIFPIPE_SOURCE_PASSWORD"); sourcePassword != "" {
		sourceAuth = transport.NewPasswordAuth(sourcePassword)
	} else if opts.Auth != nil && opts.Auth.SourceAuth != nil && len(opts.Auth.SourceAuth) > 0 {
		var err error
		sourceAuth, err = transport.AuthFromConfig(opts.Auth.SourceAuth)
		if err != nil {
			return nil, nil, fmt.Errorf("source auth: %w", err)
		}
	} else {
		// Try default authentication methods
		sourceAuth = transport.NewMultiAuth(
			transport.NewAgentAuth(),
		)
	}

	// Get dest auth
	if destLoc != nil {
		// Try environment variable for dest password
		if destPassword := os.Getenv("DIFPIPE_DEST_PASSWORD"); destPassword != "" {
			destAuth = transport.NewPasswordAuth(destPassword)
		} else if opts.Auth != nil && opts.Auth.DestAuth != nil && len(opts.Auth.DestAuth) > 0 {
			var err error
			destAuth, err = transport.AuthFromConfig(opts.Auth.DestAuth)
			if err != nil {
				return nil, nil, fmt.Errorf("dest auth: %w", err)
			}
		} else {
			// Try default authentication methods
			destAuth = transport.NewMultiAuth(
				transport.NewAgentAuth(),
			)
		}
	}

	return sourceAuth, destAuth, nil
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
