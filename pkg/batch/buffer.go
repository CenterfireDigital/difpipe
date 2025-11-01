package batch

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

// BufferManager manages disk-based buffering for batches
type BufferManager struct {
	path         string
	maxSize      int64 // Maximum size in bytes
	currentSize  atomic.Int64
	config       *Config
	mutex        sync.Mutex
	initialized  bool
}

// NewBufferManager creates a new buffer manager
func NewBufferManager(config *Config) *BufferManager {
	maxSizeBytes := int64(config.BufferMaxSizeGB) * 1024 * 1024 * 1024

	return &BufferManager{
		path:    config.BufferPath,
		maxSize: maxSizeBytes,
		config:  config,
	}
}

// Initialize creates the buffer directory
func (bm *BufferManager) Initialize() error {
	bm.mutex.Lock()
	defer bm.mutex.Unlock()

	if bm.initialized {
		return nil
	}

	// Create buffer directory
	if err := os.MkdirAll(bm.path, 0755); err != nil {
		return fmt.Errorf("create buffer directory: %w", err)
	}

	bm.initialized = true
	return nil
}

// GetBatchPath returns the path for a batch file
func (bm *BufferManager) GetBatchPath(manifestID string, batchID int) string {
	return filepath.Join(bm.path, manifestID, fmt.Sprintf("batch_%05d.tar.gz", batchID))
}

// ReserveSpace reserves space in the buffer for a batch
// Returns true if space is available, false if buffer is full
func (bm *BufferManager) ReserveSpace(size int64) bool {
	for {
		current := bm.currentSize.Load()
		if current+size > bm.maxSize {
			return false // Buffer full
		}

		// Try to atomically add the size
		if bm.currentSize.CompareAndSwap(current, current+size) {
			return true
		}
		// CAS failed, retry
	}
}

// ReleaseSpace releases space in the buffer after a batch is consumed
func (bm *BufferManager) ReleaseSpace(size int64) {
	bm.currentSize.Add(-size)
}

// GetCurrentSize returns the current buffer usage
func (bm *BufferManager) GetCurrentSize() int64 {
	return bm.currentSize.Load()
}

// GetMaxSize returns the maximum buffer size
func (bm *BufferManager) GetMaxSize() int64 {
	return bm.maxSize
}

// GetUtilization returns buffer utilization as a percentage (0.0-1.0)
func (bm *BufferManager) GetUtilization() float64 {
	current := float64(bm.currentSize.Load())
	max := float64(bm.maxSize)
	if max == 0 {
		return 0
	}
	return current / max
}

// IsFull returns true if buffer is at or above capacity
func (bm *BufferManager) IsFull() bool {
	return bm.GetUtilization() >= 0.95 // 95% threshold
}

// IsLow returns true if buffer is below threshold
func (bm *BufferManager) IsLow() bool {
	return bm.GetUtilization() <= 0.10 // 10% threshold
}

// DeleteBatch deletes a batch file from the buffer
func (bm *BufferManager) DeleteBatch(path string, size int64) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete batch: %w", err)
	}

	bm.ReleaseSpace(size)
	return nil
}

// Cleanup removes all buffer files
func (bm *BufferManager) Cleanup(manifestID string) error {
	manifestPath := filepath.Join(bm.path, manifestID)

	// Remove manifest-specific directory
	if err := os.RemoveAll(manifestPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cleanup buffer: %w", err)
	}

	// Reset current size
	bm.currentSize.Store(0)

	return nil
}

// EnsureBatchDir ensures the batch directory exists
func (bm *BufferManager) EnsureBatchDir(manifestID string) error {
	manifestPath := filepath.Join(bm.path, manifestID)
	if err := os.MkdirAll(manifestPath, 0755); err != nil {
		return fmt.Errorf("create batch directory: %w", err)
	}
	return nil
}
