package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/larrydiffey/difpipe/pkg/analyzer"
	"github.com/larrydiffey/difpipe/pkg/core"
	"github.com/larrydiffey/difpipe/pkg/engines/proxy"
	"github.com/larrydiffey/difpipe/pkg/engines/rclone"
	"github.com/larrydiffey/difpipe/pkg/engines/rsync"
	"github.com/larrydiffey/difpipe/pkg/engines/tarstream"
)

// Orchestrator coordinates the transfer process
type Orchestrator struct {
	analyzer *analyzer.FileAnalyzer
	engines  map[core.Strategy]core.TransferEngine
	progress core.ProgressReporter
}

// New creates a new orchestrator
func New() *Orchestrator {
	o := &Orchestrator{
		analyzer: analyzer.New(),
		engines:  make(map[core.Strategy]core.TransferEngine),
	}

	// Register all available engines
	o.RegisterEngine(core.StrategyRclone, rclone.New())
	o.RegisterEngine(core.StrategyRsync, rsync.New())
	o.RegisterEngine(core.StrategyTar, tarstream.New())
	o.RegisterEngine(core.StrategyProxy, proxy.New())

	return o
}

// WithProgress sets a progress reporter
func (o *Orchestrator) WithProgress(reporter core.ProgressReporter) *Orchestrator {
	o.progress = reporter
	return o
}

// RegisterEngine registers a transfer engine for a strategy
func (o *Orchestrator) RegisterEngine(strategy core.Strategy, engine core.TransferEngine) {
	o.engines[strategy] = engine
}

// Analyze analyzes the source and returns recommendations
func (o *Orchestrator) Analyze(ctx context.Context, source string) (*core.FileAnalysis, error) {
	return o.analyzer.Analyze(ctx, source)
}

// Transfer performs the complete transfer operation
func (o *Orchestrator) Transfer(ctx context.Context, opts *core.TransferOptions) (*core.TransferResult, error) {
	// Apply custom thresholds if provided
	if opts.Thresholds != nil {
		o.applyThresholds(opts.Thresholds)
	}

	// Select strategy if auto
	if opts.Strategy == core.StrategyAuto || opts.Strategy == "" {
		strategy, err := o.SelectStrategy(ctx, opts.Source, opts.Destination)
		if err != nil {
			return nil, fmt.Errorf("select strategy: %w", err)
		}
		opts.Strategy = strategy
	}

	// Get engine for strategy
	engine, err := o.GetEngine(opts.Strategy)
	if err != nil {
		return nil, err
	}

	// Set progress reporter on engine if it supports it
	if o.progress != nil {
		switch e := engine.(type) {
		case *rclone.Engine:
			e.WithProgress(o.progress)
		case *rsync.Engine:
			e.WithProgress(o.progress)
		case *tarstream.Engine:
			e.WithProgress(o.progress)
		case *proxy.Engine:
			e.WithProgress(o.progress)
		}
	}

	// Perform transfer
	result, err := engine.Transfer(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("transfer failed: %w", err)
	}

	return result, nil
}

// Estimate provides transfer estimates without performing the transfer
func (o *Orchestrator) Estimate(ctx context.Context, opts *core.TransferOptions) (*core.TransferEstimate, error) {
	// Analyze source
	analysis, err := o.analyzer.Analyze(ctx, opts.Source)
	if err != nil {
		return nil, fmt.Errorf("analyze source: %w", err)
	}

	// Select strategy if auto
	strategy := opts.Strategy
	if strategy == core.StrategyAuto || strategy == "" {
		strategy = analysis.Recommendation
	}

	// Get engine
	engine, err := o.GetEngine(strategy)
	if err != nil {
		return nil, err
	}

	// Get estimate from engine
	estimate, err := engine.Estimate(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("estimate: %w", err)
	}

	// Enhance estimate with analysis data
	estimate.BytesTotal = analysis.TotalSize
	estimate.FilesTotal = analysis.TotalFiles
	estimate.Recommendation = analysis.Recommendation
	estimate.RecommendReason = analysis.RecommendReason

	// Calculate estimated time (rough estimate based on typical speeds)
	if estimate.BytesTotal > 0 {
		// Assume 100 MB/s for estimation
		estimatedSeconds := float64(estimate.BytesTotal) / (100 * 1024 * 1024)
		estimate.EstimatedTime = secondsToDuration(estimatedSeconds)
		estimate.EstimatedSpeed = "~100 MB/s"
	}

	return estimate, nil
}

// SelectStrategy analyzes source and destination and selects best strategy
func (o *Orchestrator) SelectStrategy(ctx context.Context, source, destination string) (core.Strategy, error) {
	analysis, err := o.analyzer.AnalyzeTransfer(ctx, source, destination)
	if err != nil {
		return "", fmt.Errorf("analyze for strategy selection: %w", err)
	}

	return analysis.Recommendation, nil
}

// GetEngine returns the engine for a given strategy
func (o *Orchestrator) GetEngine(strategy core.Strategy) (core.TransferEngine, error) {
	engine, exists := o.engines[strategy]
	if !exists {
		return nil, &EngineNotFoundError{Strategy: strategy}
	}
	return engine, nil
}

// EngineNotFoundError is returned when no engine is registered for a strategy
type EngineNotFoundError struct {
	Strategy core.Strategy
}

func (e *EngineNotFoundError) Error() string {
	return fmt.Sprintf("no engine registered for strategy: %s", e.Strategy)
}

func (e *EngineNotFoundError) ExitCode() int {
	return core.ExitEngineNotFound
}

// secondsToDuration converts seconds to time.Duration
func secondsToDuration(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}

// applyThresholds applies custom thresholds to the analyzer
func (o *Orchestrator) applyThresholds(thresholds *core.ThresholdSettings) {
	o.analyzer.WithThresholds(
		thresholds.SmallFileSizeKB,
		thresholds.LargeFileSizeMB,
		thresholds.ManyFilesCount,
		thresholds.FewFilesCount,
		thresholds.MaxSampleSize,
		thresholds.SmallFilePercent,
		thresholds.LargeFilePercent,
	)
}
