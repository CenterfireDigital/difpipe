package main

import (
	"context"
	"fmt"
	"os"

	"github.com/larrydiffey/difpipe/pkg/config"
	"github.com/larrydiffey/difpipe/pkg/core"
	"github.com/larrydiffey/difpipe/pkg/orchestrator"
	"github.com/larrydiffey/difpipe/pkg/output"
	"github.com/larrydiffey/difpipe/pkg/status"
	"github.com/spf13/cobra"
)

var (
	// Global flags
	configFile   string
	outputFormat string
	verbose      bool

	// Root command
	rootCmd = &cobra.Command{
		Use:   "difpipe",
		Short: "DifPipe - Intelligent data transfer orchestration",
		Long: `DifPipe is an intelligent data transfer orchestrator that automatically
selects the best transfer strategy and engine based on your data characteristics.

Supports 70+ storage backends through rclone integration, with smart strategy
selection for optimal performance.`,
		Version: "0.5.0",
	}

	// Transfer command
	transferCmd = &cobra.Command{
		Use:   "transfer [source] [destination]",
		Short: "Transfer data from source to destination",
		Long: `Transfer data using the optimal strategy and engine.

Examples:
  # Transfer with auto-detection
  difpipe transfer /data s3://backup

  # Transfer with config file
  difpipe transfer --config transfer.yaml

  # Transfer with stdin config
  echo '{"source":"/data","dest":"s3://backup"}' | difpipe transfer --config -`,
		Args: cobra.MaximumNArgs(2),
		RunE: runTransfer,
	}

	// Analyze command
	analyzeCmd = &cobra.Command{
		Use:   "analyze [source] [destination]",
		Short: "Analyze files and recommend strategy without transferring",
		Long: `Analyze the source files and recommend the best transfer strategy
without actually performing the transfer.

Returns:
- File counts and sizes
- Average file size
- File type distribution
- Recommended strategy with explanation`,
		Args: cobra.RangeArgs(1, 2),
		RunE: runAnalyze,
	}

	// Schema command
	schemaCmd = &cobra.Command{
		Use:   "schema",
		Short: "Print JSON schema for config file",
		Long: `Print the JSON schema for the configuration file format.
Useful for validation and IDE autocomplete.`,
		RunE: runSchema,
	}

	// Status command
	statusCmd = &cobra.Command{
		Use:   "status [transfer-id]",
		Short: "Show transfer status",
		Long: `Show the status of a transfer by ID.
If no ID is provided, shows all recent transfers.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runStatus,
	}

	// Version command (already handled by cobra)
)

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "config file (JSON/YAML), use '-' for stdin")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "text", "output format: text, json, yaml, csv")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	// Transfer flags
	transferCmd.Flags().String("strategy", "auto", "transfer strategy: auto, rclone, rsync, tar")
	transferCmd.Flags().Int("parallel", 4, "number of parallel transfers")
	transferCmd.Flags().Bool("checkpoint", true, "enable checkpoint/resume")
	transferCmd.Flags().String("compression", "auto", "compression: auto, none, zstd, gzip")
	transferCmd.Flags().Bool("dry-run", false, "perform dry run without actual transfer")
	transferCmd.Flags().StringSlice("include", []string{}, "include patterns")
	transferCmd.Flags().StringSlice("exclude", []string{}, "exclude patterns")
	transferCmd.Flags().Bool("stream", false, "stream progress as newline-delimited JSON")

	// Status flags
	statusCmd.Flags().String("state", "", "filter by state: queued, running, completed, failed")

	// Add commands
	rootCmd.AddCommand(transferCmd)
	rootCmd.AddCommand(analyzeCmd)
	rootCmd.AddCommand(schemaCmd)
	rootCmd.AddCommand(statusCmd)
}

func main() {
	// Initialize status tracker
	if err := status.Initialize(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to initialize status tracker: %v\n", err)
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(core.ExitGeneralError)
	}
}

// runTransfer executes the transfer command
func runTransfer(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Load configuration
	cfg, err := loadConfig(args)
	if err != nil {
		return exitWithError(core.ExitConfigError, "load config", err)
	}

	// Override with flags
	applyFlags(cmd, cfg)

	// Create orchestrator
	orch := orchestrator.New()

	// Set up progress reporting if streaming
	streamFlag, _ := cmd.Flags().GetBool("stream")
	if streamFlag {
		streamWriter := output.NewStreamWriter(os.Stdout)
		// Would set up progress reporter here
		_ = streamWriter
	}

	// Build transfer options
	opts := &core.TransferOptions{
		Source:      cfg.Transfer.Source.Path,
		Destination: cfg.Transfer.Destination.Path,
		Strategy:    core.Strategy(cfg.Transfer.Options.Strategy),
		Compression: core.Compression(cfg.Transfer.Options.Compression),
		Parallel:    cfg.Transfer.Options.Parallel,
		Checkpoint:  cfg.Transfer.Options.Checkpoint,
		DryRun:      cfg.Transfer.Options.DryRun,
		Filters: &core.FilterOptions{
			Include: cfg.Transfer.Filters.Include,
			Exclude: cfg.Transfer.Filters.Exclude,
		},
		Auth: &core.AuthOptions{
			SourceAuth: cfg.Transfer.Source.Auth,
			DestAuth:   cfg.Transfer.Destination.Auth,
		},
		Thresholds: convertThresholds(cfg.Transfer.Options.Thresholds),
	}

	// Perform transfer
	result, err := orch.Transfer(ctx, opts)
	if err != nil {
		return exitWithError(core.ExitTransferFailed, "transfer", err)
	}

	// Format output
	formatter := output.New(output.Format(cfg.Output.Format), os.Stdout)
	return formatter.Format(result)
}

// runAnalyze executes the analyze command
func runAnalyze(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	source := args[0]

	// Create orchestrator
	orch := orchestrator.New()

	// Analyze
	analysis, err := orch.Analyze(ctx, source)
	if err != nil {
		return exitWithError(core.ExitGeneralError, "analyze", err)
	}

	// Format output
	format := output.Format(outputFormat)
	formatter := output.New(format, os.Stdout)
	return formatter.Format(analysis)
}

// runSchema executes the schema command
func runSchema(cmd *cobra.Command, args []string) error {
	schema := `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "DifPipe Configuration",
  "type": "object",
  "properties": {
    "transfer": {
      "type": "object",
      "properties": {
        "source": {
          "type": "object",
          "properties": {
            "path": {"type": "string"}
          },
          "required": ["path"]
        },
        "destination": {
          "type": "object",
          "properties": {
            "path": {"type": "string"}
          },
          "required": ["path"]
        },
        "options": {
          "type": "object",
          "properties": {
            "strategy": {"type": "string", "enum": ["auto", "rclone", "rsync", "tar"]},
            "parallel": {"type": "integer", "minimum": 1},
            "checkpoint": {"type": "boolean"},
            "compression": {"type": "string", "enum": ["auto", "none", "zstd", "gzip"]},
            "dry_run": {"type": "boolean"}
          }
        }
      },
      "required": ["source", "destination"]
    },
    "output": {
      "type": "object",
      "properties": {
        "format": {"type": "string", "enum": ["text", "json", "yaml", "csv"]},
        "stream": {"type": "boolean"}
      }
    }
  },
  "required": ["transfer"]
}`

	fmt.Println(schema)
	return nil
}

// loadConfig loads configuration from file, args, or environment
func loadConfig(args []string) (*config.Config, error) {
	var cfg *config.Config
	var err error

	// Priority: config file > args > environment
	if configFile != "" {
		cfg, err = config.LoadConfig(configFile)
		if err != nil {
			return nil, err
		}
	} else if len(args) >= 2 {
		// Create config from args
		cfg = &config.Config{
			Transfer: config.TransferConfig{
				Source: config.SourceConfig{
					Path: args[0],
				},
				Destination: config.DestinationConfig{
					Path: args[1],
				},
				Options: config.TransferOptions{
					Strategy:    "auto",
					Parallel:    4,
					Checkpoint:  true,
					Compression: "auto",
				},
			},
			Output: config.OutputConfig{
				Format: outputFormat,
			},
		}
	} else {
		return nil, fmt.Errorf("either --config or [source] [destination] arguments required")
	}

	// Merge with environment variables
	envCfg := config.FromEnv()
	cfg = config.Merge(cfg, envCfg)

	return cfg, nil
}

// applyFlags overrides config with command-line flags
func applyFlags(cmd *cobra.Command, cfg *config.Config) {
	if cmd.Flags().Changed("strategy") {
		strategy, _ := cmd.Flags().GetString("strategy")
		cfg.Transfer.Options.Strategy = strategy
	}
	if cmd.Flags().Changed("parallel") {
		parallel, _ := cmd.Flags().GetInt("parallel")
		cfg.Transfer.Options.Parallel = parallel
	}
	if cmd.Flags().Changed("checkpoint") {
		checkpoint, _ := cmd.Flags().GetBool("checkpoint")
		cfg.Transfer.Options.Checkpoint = checkpoint
	}
	if cmd.Flags().Changed("compression") {
		compression, _ := cmd.Flags().GetString("compression")
		cfg.Transfer.Options.Compression = compression
	}
	if cmd.Flags().Changed("dry-run") {
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		cfg.Transfer.Options.DryRun = dryRun
	}
	if cmd.Flags().Changed("include") {
		include, _ := cmd.Flags().GetStringSlice("include")
		cfg.Transfer.Filters.Include = include
	}
	if cmd.Flags().Changed("exclude") {
		exclude, _ := cmd.Flags().GetStringSlice("exclude")
		cfg.Transfer.Filters.Exclude = exclude
	}
	if cmd.Flags().Changed("output") {
		cfg.Output.Format = outputFormat
	}
}

// exitWithError prints error and exits with appropriate code
func exitWithError(code int, context string, err error) error {
	info := core.GetExitCodeInfo(code)

	errorOutput := map[string]interface{}{
		"error":       err.Error(),
		"context":     context,
		"exit_code":   code,
		"category":    info.Category,
		"retryable":   info.Retryable,
		"suggestion":  info.Suggestion,
	}

	formatter := output.New(output.Format(outputFormat), os.Stderr)
	_ = formatter.Format(errorOutput)

	os.Exit(code)
	return nil // Never reached
}

// convertThresholds converts config thresholds to core thresholds
func convertThresholds(cfg *config.ThresholdSettings) *core.ThresholdSettings {
	if cfg == nil {
		return nil
	}
	return &core.ThresholdSettings{
		SmallFileSizeKB:  cfg.SmallFileSizeKB,
		LargeFileSizeMB:  cfg.LargeFileSizeMB,
		ManyFilesCount:   cfg.ManyFilesCount,
		SmallFilePercent: cfg.SmallFilePercent,
		LargeFilePercent: cfg.LargeFilePercent,
		FewFilesCount:    cfg.FewFilesCount,
		MaxSampleSize:    cfg.MaxSampleSize,
	}
}

// runStatus executes the status command
func runStatus(cmd *cobra.Command, args []string) error {
	// Check if tracker is initialized
	if status.GlobalTracker == nil {
		return fmt.Errorf("status tracker not initialized")
	}

	// Get state filter
	stateFilter, _ := cmd.Flags().GetString("state")

	// If transfer ID provided, show specific transfer
	if len(args) > 0 {
		transferID := args[0]
		st, err := status.GlobalTracker.Get(transferID)
		if err != nil {
			return fmt.Errorf("get status: %w", err)
		}

		formatter := output.New(output.Format(outputFormat), os.Stdout)
		return formatter.Format(st)
	}

	// Otherwise, list all transfers or filter by state
	var transfers []*status.TransferStatus

	if stateFilter != "" {
		// Parse state
		var state core.TransferState
		switch stateFilter {
		case "queued":
			state = core.StateQueued
		case "running":
			state = core.StateRunning
		case "completed":
			state = core.StateCompleted
		case "failed":
			state = core.StateFailed
		default:
			return fmt.Errorf("invalid state: %s", stateFilter)
		}

		transfers = status.GlobalTracker.ListByState(state)
	} else {
		transfers = status.GlobalTracker.List()
	}

	// Format output
	formatter := output.New(output.Format(outputFormat), os.Stdout)
	return formatter.Format(transfers)
}
