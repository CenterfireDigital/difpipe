package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the complete configuration
type Config struct {
	Transfer TransferConfig `json:"transfer" yaml:"transfer"`
	Output   OutputConfig   `json:"output" yaml:"output"`
}

// TransferConfig contains transfer-specific settings
type TransferConfig struct {
	Source      SourceConfig      `json:"source" yaml:"source"`
	Destination DestinationConfig `json:"destination" yaml:"destination"`
	Options     TransferOptions   `json:"options" yaml:"options"`
	Filters     FilterConfig      `json:"filters,omitempty" yaml:"filters,omitempty"`
}

// SourceConfig defines the source location
type SourceConfig struct {
	Path string                 `json:"path" yaml:"path"`
	Auth map[string]interface{} `json:"auth,omitempty" yaml:"auth,omitempty"`
}

// DestinationConfig defines the destination location
type DestinationConfig struct {
	Path string                 `json:"path" yaml:"path"`
	Auth map[string]interface{} `json:"auth,omitempty" yaml:"auth,omitempty"`
}

// TransferOptions contains transfer behavior settings
type TransferOptions struct {
	Strategy    string              `json:"strategy" yaml:"strategy"`       // auto, rclone, rsync, tar
	Parallel    int                 `json:"parallel" yaml:"parallel"`
	Checkpoint  bool                `json:"checkpoint" yaml:"checkpoint"`
	Compression string              `json:"compression" yaml:"compression"` // auto, none, zstd, gzip
	DryRun      bool                `json:"dry_run" yaml:"dry_run"`
	Thresholds  *ThresholdSettings  `json:"thresholds,omitempty" yaml:"thresholds,omitempty"`
	Batching    *BatchingSettings   `json:"batching,omitempty" yaml:"batching,omitempty"`
	Buffering   *BufferingSettings  `json:"buffering,omitempty" yaml:"buffering,omitempty"`
	Workers     *WorkersSettings    `json:"workers,omitempty" yaml:"workers,omitempty"`
}

// ThresholdSettings defines thresholds for strategy selection
type ThresholdSettings struct {
	SmallFileSizeKB  int     `json:"small_file_size_kb" yaml:"small_file_size_kb"`   // Files smaller than this are "small" (default: 10)
	LargeFileSizeMB  int     `json:"large_file_size_mb" yaml:"large_file_size_mb"`   // Files larger than this are "large" (default: 100)
	ManyFilesCount   int     `json:"many_files_count" yaml:"many_files_count"`       // More than this is "many files" (default: 1000)
	SmallFilePercent float64 `json:"small_file_percent" yaml:"small_file_percent"`   // % of small files to trigger tar (default: 80)
	LargeFilePercent float64 `json:"large_file_percent" yaml:"large_file_percent"`   // % of large files to trigger rsync (default: 50)
	FewFilesCount    int     `json:"few_files_count" yaml:"few_files_count"`         // Fewer than this is "few files" (default: 10)
	MaxSampleSize    int     `json:"max_sample_size" yaml:"max_sample_size"`         // Maximum files to sample (default: 10000)
}

// BatchingSettings defines batching configuration for tar transfers
type BatchingSettings struct {
	Enabled     bool `json:"enabled" yaml:"enabled"`           // Enable batching (default: true for tar)
	ChunkSizeMB int  `json:"chunk_size_mb" yaml:"chunk_size_mb"` // Size of each batch in MB (default: 50)
}

// BufferingSettings defines disk buffering configuration
type BufferingSettings struct {
	Enabled       bool   `json:"enabled" yaml:"enabled"`               // Enable disk buffering (default: true)
	Path          string `json:"path" yaml:"path"`                     // Buffer directory (default: /tmp/difpipe-buffer)
	MaxSizeGB     int    `json:"max_size_gb" yaml:"max_size_gb"`       // Maximum buffer size in GB (default: 100)
	Cleanup       bool   `json:"cleanup" yaml:"cleanup"`               // Delete buffer after transfer (default: true)
	KeepOnFailure bool   `json:"keep_on_failure" yaml:"keep_on_failure"` // Keep buffer on failure for resume (default: true)
}

// WorkersSettings defines worker pool configuration
type WorkersSettings struct {
	Source      int  `json:"source" yaml:"source"`           // Number of source workers (default: 4)
	Destination int  `json:"destination" yaml:"destination"` // Number of destination workers (default: 2)
	Adaptive    bool `json:"adaptive" yaml:"adaptive"`       // Auto-adjust based on speeds (default: false, future)
}

// FilterConfig defines include/exclude patterns
type FilterConfig struct {
	Include []string `json:"include,omitempty" yaml:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty" yaml:"exclude,omitempty"`
}

// OutputConfig controls output formatting
type OutputConfig struct {
	Format string `json:"format" yaml:"format"` // text, json, yaml, csv
	Stream bool   `json:"stream" yaml:"stream"` // Stream progress
}

// LoadConfig loads configuration from file, stdin, or inline string
func LoadConfig(input string) (*Config, error) {
	var data []byte
	var err error

	switch {
	case input == "-":
		// Read from stdin
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
	case strings.HasPrefix(input, "{") || strings.HasPrefix(input, "---"):
		// Inline JSON/YAML string
		data = []byte(input)
	default:
		// File path
		data, err = os.ReadFile(input)
		if err != nil {
			return nil, fmt.Errorf("read file %s: %w", input, err)
		}
	}

	return ParseAuto(data)
}

// ParseAuto auto-detects format (JSON or YAML) and parses
func ParseAuto(data []byte) (*Config, error) {
	trimmed := strings.TrimSpace(string(data))

	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty config data")
	}

	var cfg Config

	// Try JSON first (starts with { or [)
	if trimmed[0] == '{' || trimmed[0] == '[' {
		if err := json.Unmarshal([]byte(trimmed), &cfg); err == nil {
			return &cfg, nil
		}
	}

	// Try YAML
	if err := yaml.Unmarshal([]byte(trimmed), &cfg); err == nil {
		return &cfg, nil
	}

	return nil, fmt.Errorf("couldn't parse as JSON or YAML")
}

// FromEnv loads configuration from environment variables
func FromEnv() *Config {
	cfg := &Config{
		Transfer: TransferConfig{
			Source: SourceConfig{
				Path: os.Getenv("DIFPIPE_SOURCE"),
			},
			Destination: DestinationConfig{
				Path: os.Getenv("DIFPIPE_DEST"),
			},
			Options: TransferOptions{
				Strategy:    getEnvOrDefault("DIFPIPE_STRATEGY", "auto"),
				Parallel:    getEnvInt("DIFPIPE_PARALLEL", 4),
				Checkpoint:  getEnvBool("DIFPIPE_CHECKPOINT", true),
				Compression: getEnvOrDefault("DIFPIPE_COMPRESSION", "auto"),
				DryRun:      getEnvBool("DIFPIPE_DRY_RUN", false),
			},
		},
		Output: OutputConfig{
			Format: getEnvOrDefault("DIFPIPE_OUTPUT", "text"),
			Stream: getEnvBool("DIFPIPE_PROGRESS_STREAM", false),
		},
	}

	return cfg
}

// Merge combines multiple configs with priority (earlier = higher priority)
func Merge(configs ...*Config) *Config {
	result := &Config{}

	for i := len(configs) - 1; i >= 0; i-- {
		cfg := configs[i]
		if cfg == nil {
			continue
		}

		// Merge source
		if cfg.Transfer.Source.Path != "" {
			result.Transfer.Source.Path = cfg.Transfer.Source.Path
		}
		if cfg.Transfer.Source.Auth != nil && len(cfg.Transfer.Source.Auth) > 0 {
			result.Transfer.Source.Auth = cfg.Transfer.Source.Auth
		}

		// Merge destination
		if cfg.Transfer.Destination.Path != "" {
			result.Transfer.Destination.Path = cfg.Transfer.Destination.Path
		}
		if cfg.Transfer.Destination.Auth != nil && len(cfg.Transfer.Destination.Auth) > 0 {
			result.Transfer.Destination.Auth = cfg.Transfer.Destination.Auth
		}

		// Merge options
		if cfg.Transfer.Options.Strategy != "" && cfg.Transfer.Options.Strategy != "auto" {
			result.Transfer.Options.Strategy = cfg.Transfer.Options.Strategy
		}
		if cfg.Transfer.Options.Parallel > 0 {
			result.Transfer.Options.Parallel = cfg.Transfer.Options.Parallel
		}
		if cfg.Transfer.Options.Compression != "" {
			result.Transfer.Options.Compression = cfg.Transfer.Options.Compression
		}
		result.Transfer.Options.Checkpoint = cfg.Transfer.Options.Checkpoint
		result.Transfer.Options.DryRun = cfg.Transfer.Options.DryRun
		if cfg.Transfer.Options.Thresholds != nil {
			result.Transfer.Options.Thresholds = cfg.Transfer.Options.Thresholds
		}

		// Merge filters
		if len(cfg.Transfer.Filters.Include) > 0 {
			result.Transfer.Filters.Include = cfg.Transfer.Filters.Include
		}
		if len(cfg.Transfer.Filters.Exclude) > 0 {
			result.Transfer.Filters.Exclude = cfg.Transfer.Filters.Exclude
		}

		// Merge output
		if cfg.Output.Format != "" {
			result.Output.Format = cfg.Output.Format
		}
		result.Output.Stream = cfg.Output.Stream
	}

	// Set defaults if not set
	if result.Transfer.Options.Strategy == "" {
		result.Transfer.Options.Strategy = "auto"
	}
	if result.Transfer.Options.Parallel == 0 {
		result.Transfer.Options.Parallel = 4
	}
	if result.Transfer.Options.Compression == "" {
		result.Transfer.Options.Compression = "auto"
	}
	if result.Output.Format == "" {
		result.Output.Format = "text"
	}

	return result
}

// DefaultThresholds returns the default threshold settings
func DefaultThresholds() *ThresholdSettings {
	return &ThresholdSettings{
		SmallFileSizeKB:  10,   // 10 KB
		LargeFileSizeMB:  100,  // 100 MB
		ManyFilesCount:   1000, // 1000 files
		SmallFilePercent: 80.0, // 80%
		LargeFilePercent: 50.0, // 50%
		FewFilesCount:    10,   // 10 files
		MaxSampleSize:    10000, // 10k files
	}
}

// Helper functions for environment variables
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}
