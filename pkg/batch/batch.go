package batch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Manifest represents the complete transfer plan with batches
type Manifest struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	Source       string    `json:"source"`
	Destination  string    `json:"destination"`
	TotalFiles   int       `json:"total_files"`
	TotalSize    int64     `json:"total_size"`
	ChunkSizeMB  int       `json:"chunk_size_mb"`
	Batches      []*Batch  `json:"batches"`
	CompletedAt  time.Time `json:"completed_at,omitempty"`
	Status       string    `json:"status"` // pending/in_progress/completed/failed

	mutex sync.RWMutex `json:"-"`
}

// Batch represents a group of files to transfer together
type Batch struct {
	ID          int      `json:"id"`
	Files       []string `json:"files"`        // Full paths relative to source
	Size        int64    `json:"size"`         // Total size in bytes
	FileCount   int      `json:"file_count"`
	Status      string   `json:"status"`       // pending/downloading/buffered/uploading/completed/failed
	LocalPath   string   `json:"local_path"`   // Path in buffer (if buffered)
	StartedAt   time.Time `json:"started_at,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
	Error       string   `json:"error,omitempty"`
	Checksum    string   `json:"checksum,omitempty"` // For verification

	mutex sync.RWMutex `json:"-"`
}

// Config contains batching configuration
type Config struct {
	ChunkSizeMB      int    // Size of each batch in MB
	SourceWorkers    int    // Number of parallel source workers
	DestWorkers      int    // Number of parallel destination workers
	BufferEnabled    bool   // Enable disk buffering
	BufferPath       string // Path for buffer storage
	BufferMaxSizeGB  int    // Maximum buffer size in GB
	CleanupBuffer    bool   // Delete buffer after transfer
	KeepOnFailure    bool   // Keep buffer on failure for resume
	CheckpointPath   string // Path to save checkpoint state
	CheckpointEnabled bool  // Enable checkpointing
}

// DefaultConfig returns sensible defaults
func DefaultConfig() *Config {
	return &Config{
		ChunkSizeMB:      50,
		SourceWorkers:    4,
		DestWorkers:      2,
		BufferEnabled:    true,
		BufferPath:       "/tmp/difpipe-buffer",
		BufferMaxSizeGB:  100,
		CleanupBuffer:    true,
		KeepOnFailure:    true,
		CheckpointPath:   "/tmp/difpipe-checkpoint.json",
		CheckpointEnabled: true,
	}
}

// NewManifest creates a new manifest
func NewManifest(source, destination string, chunkSizeMB int) *Manifest {
	return &Manifest{
		ID:          fmt.Sprintf("manifest-%d", time.Now().UnixNano()),
		CreatedAt:   time.Now(),
		Source:      source,
		Destination: destination,
		ChunkSizeMB: chunkSizeMB,
		Batches:     []*Batch{},
		Status:      "pending",
	}
}

// AddBatch adds a batch to the manifest
func (m *Manifest) AddBatch(files []string, size int64) *Batch {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	batch := &Batch{
		ID:        len(m.Batches),
		Files:     files,
		Size:      size,
		FileCount: len(files),
		Status:    "pending",
	}

	m.Batches = append(m.Batches, batch)
	m.TotalFiles += len(files)
	m.TotalSize += size

	return batch
}

// GetPendingBatches returns all pending batches
func (m *Manifest) GetPendingBatches() []*Batch {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var pending []*Batch
	for _, batch := range m.Batches {
		if batch.GetStatus() == "pending" {
			pending = append(pending, batch)
		}
	}
	return pending
}

// GetStatus returns the current status
func (m *Manifest) GetStatus() string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.Status
}

// SetStatus sets the status
func (m *Manifest) SetStatus(status string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.Status = status
	if status == "completed" {
		m.CompletedAt = time.Now()
	}
}

// GetProgress returns completion percentage
func (m *Manifest) GetProgress() (completed, total int, percentage float64) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	total = len(m.Batches)
	for _, batch := range m.Batches {
		if batch.GetStatus() == "completed" {
			completed++
		}
	}

	if total > 0 {
		percentage = float64(completed) / float64(total) * 100.0
	}

	return completed, total, percentage
}

// Save saves the manifest to disk for checkpointing
func (m *Manifest) Save(path string) error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create checkpoint directory: %w", err)
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	// Write to file
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write checkpoint: %w", err)
	}

	return nil
}

// Load loads a manifest from disk
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read checkpoint: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("unmarshal manifest: %w", err)
	}

	return &manifest, nil
}

// Batch methods

// GetStatus returns the batch status
func (b *Batch) GetStatus() string {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.Status
}

// SetStatus sets the batch status
func (b *Batch) SetStatus(status string) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.Status = status
	if status == "downloading" && b.StartedAt.IsZero() {
		b.StartedAt = time.Now()
	}
	if status == "completed" || status == "failed" {
		b.CompletedAt = time.Now()
	}
}

// SetError sets an error message
func (b *Batch) SetError(err error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if err != nil {
		b.Error = err.Error()
		b.Status = "failed"
		b.CompletedAt = time.Now()
	}
}

// SetLocalPath sets the local buffer path
func (b *Batch) SetLocalPath(path string) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.LocalPath = path
}

// GetLocalPath returns the local buffer path
func (b *Batch) GetLocalPath() string {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.LocalPath
}
