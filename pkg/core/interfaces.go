package core

import (
	"context"
	"io"
)

// TransferEngine defines the interface for transfer implementations
type TransferEngine interface {
	// Name returns the engine name (rclone, rsync, tar)
	Name() string

	// Transfer performs the actual transfer
	Transfer(ctx context.Context, opts *TransferOptions) (*TransferResult, error)

	// SupportsProtocol checks if this engine supports a given protocol
	SupportsProtocol(protocol string) bool

	// Estimate estimates transfer metrics without performing the transfer
	Estimate(ctx context.Context, opts *TransferOptions) (*TransferEstimate, error)
}

// ProgressReporter receives progress updates during transfer
type ProgressReporter interface {
	// Start signals the beginning of a transfer
	Start(total int64, message string)

	// Update reports progress
	Update(done int64, message string)

	// Complete signals successful completion
	Complete(message string)

	// Error reports an error
	Error(err error)
}

// Analyzer analyzes files and recommends strategies
type Analyzer interface {
	// Analyze examines the source and returns analysis results
	Analyze(ctx context.Context, source string) (*FileAnalysis, error)

	// Recommend suggests the best transfer strategy
	Recommend(analysis *FileAnalysis) Strategy
}

// Checkpointer manages transfer checkpoints for resumption
type Checkpointer interface {
	// Save saves checkpoint state
	Save(transferID string, state *CheckpointState) error

	// Load loads checkpoint state
	Load(transferID string) (*CheckpointState, error)

	// Delete removes a checkpoint
	Delete(transferID string) error

	// Exists checks if a checkpoint exists
	Exists(transferID string) bool
}

// OutputStream represents a streamable output
type OutputStream interface {
	io.Writer
	io.Closer

	// Flush ensures all buffered data is written
	Flush() error
}
