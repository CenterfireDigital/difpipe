package core

import (
	"time"
)

// Strategy represents a transfer strategy
type Strategy string

const (
	StrategyAuto      Strategy = "auto"      // Auto-detect best strategy
	StrategyRclone    Strategy = "rclone"    // Use rclone
	StrategyRsync     Strategy = "rsync"     // Use rsync
	StrategyTar       Strategy = "tar"       // Use tar streaming
	StrategyProxy     Strategy = "proxy"     // Remote-to-remote streaming proxy
	StrategySkopeo    Strategy = "skopeo"    // Container images
	StrategyRestic    Strategy = "restic"    // Deduplicated backups
)

// Compression represents compression algorithm
type Compression string

const (
	CompressionAuto Compression = "auto" // Auto-detect
	CompressionNone Compression = "none" // No compression
	CompressionZstd Compression = "zstd" // Zstandard
	CompressionGzip Compression = "gzip" // Gzip
	CompressionLz4  Compression = "lz4"  // LZ4
)

// Protocol represents a transfer protocol
type Protocol string

const (
	ProtocolLocal  Protocol = "local"  // Local filesystem
	ProtocolSSH    Protocol = "ssh"    // SSH/SFTP
	ProtocolS3     Protocol = "s3"     // Amazon S3
	ProtocolGCS    Protocol = "gcs"    // Google Cloud Storage
	ProtocolAzure  Protocol = "azure"  // Azure Blob Storage
	ProtocolHTTP   Protocol = "http"   // HTTP/HTTPS
	ProtocolFTP    Protocol = "ftp"    // FTP
	ProtocolWebDAV Protocol = "webdav" // WebDAV
)

// TransferOptions contains all transfer parameters
type TransferOptions struct {
	Source      string
	Destination string
	Strategy    Strategy
	Compression Compression
	Parallel    int
	Checkpoint  bool
	DryRun      bool
	Filters     *FilterOptions
	Auth        *AuthOptions
	Thresholds  *ThresholdSettings
}

// ThresholdSettings defines thresholds for strategy selection
type ThresholdSettings struct {
	SmallFileSizeKB  int
	LargeFileSizeMB  int
	ManyFilesCount   int
	SmallFilePercent float64
	LargeFilePercent float64
	FewFilesCount    int
	MaxSampleSize    int
}

// FilterOptions defines include/exclude patterns
type FilterOptions struct {
	Include []string
	Exclude []string
}

// AuthOptions contains authentication credentials
type AuthOptions struct {
	SourceAuth map[string]interface{}
	DestAuth   map[string]interface{}
}

// TransferResult contains the outcome of a transfer
type TransferResult struct {
	Success      bool
	TransferID   string
	BytesTotal   int64
	BytesDone    int64
	FilesTotal   int64
	FilesDone    int64
	Duration     time.Duration
	AverageSpeed string // e.g., "32 MB/s"
	Message      string
	Error        error
}

// TransferEstimate provides transfer estimates
type TransferEstimate struct {
	BytesTotal      int64
	FilesTotal      int64
	EstimatedTime   time.Duration
	EstimatedSpeed  string
	Recommendation  Strategy
	RecommendReason string
}

// FileAnalysis contains analysis results
type FileAnalysis struct {
	TotalSize       int64
	TotalFiles      int64
	AverageFileSize int64
	SmallFiles      int64  // < 10 KB
	MediumFiles     int64  // 10 KB - 100 MB
	LargeFiles      int64  // > 100 MB
	FileTypes       map[string]int64
	Compressibility float64 // 0.0 = incompressible, 1.0 = highly compressible
	SourceProtocol  Protocol
	DestProtocol    Protocol
	SampleTime      time.Duration
	Recommendation  Strategy
	RecommendReason string
}

// CheckpointState stores state for resuming transfers
type CheckpointState struct {
	TransferID      string
	Source          string
	Destination     string
	Strategy        Strategy
	StartTime       time.Time
	LastUpdate      time.Time
	BytesDone       int64
	BytesTotal      int64
	FilesDone       int64
	FilesTotal      int64
	CurrentFile     string
	CompletedFiles  []string
	FailedFiles     map[string]string // file -> error
}

// TransferStatus represents the current status of a transfer
type TransferStatus struct {
	ID              string
	State           TransferState
	Progress        float64 // 0.0 - 100.0
	BytesDone       int64
	BytesTotal      int64
	FilesDone       int64
	FilesTotal      int64
	Speed           string
	ETA             string
	CurrentFile     string
	Error           error
}

// TransferState represents transfer lifecycle state
type TransferState string

const (
	StateQueued     TransferState = "queued"
	StateRunning    TransferState = "running"
	StateCompleted  TransferState = "completed"
	StateFailed     TransferState = "failed"
	StatePaused     TransferState = "paused"
	StateCanceled   TransferState = "canceled"
)
