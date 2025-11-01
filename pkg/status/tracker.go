package status

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/larrydiffey/difpipe/pkg/core"
)

// Tracker tracks the status of transfers
type Tracker struct {
	mu        sync.RWMutex
	transfers map[string]*TransferStatus
	stateDir  string
}

// TransferStatus represents the current status of a transfer
type TransferStatus struct {
	ID              string              `json:"id"`
	State           core.TransferState  `json:"state"`
	Strategy        core.Strategy       `json:"strategy"`
	Source          string              `json:"source"`
	Destination     string              `json:"destination"`
	StartTime       time.Time           `json:"start_time"`
	UpdateTime      time.Time           `json:"update_time"`
	EndTime         *time.Time          `json:"end_time,omitempty"`
	BytesTotal      int64               `json:"bytes_total"`
	BytesDone       int64               `json:"bytes_done"`
	FilesTotal      int64               `json:"files_total"`
	FilesDone       int64               `json:"files_done"`
	Progress        float64             `json:"progress"` // 0-100
	Speed           string              `json:"speed"`
	ETA             string              `json:"eta"`
	CurrentFile     string              `json:"current_file,omitempty"`
	Error           string              `json:"error,omitempty"`
}

// New creates a new status tracker
func New() (*Tracker, error) {
	// Get state directory
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}

	stateDir := filepath.Join(home, ".difpipe", "status")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}

	tracker := &Tracker{
		transfers: make(map[string]*TransferStatus),
		stateDir:  stateDir,
	}

	// Load existing status files
	if err := tracker.loadAll(); err != nil {
		// Non-fatal, just log
		fmt.Fprintf(os.Stderr, "Warning: failed to load status: %v\n", err)
	}

	return tracker, nil
}

// Register registers a new transfer
func (t *Tracker) Register(id string, source, destination string, strategy core.Strategy) {
	t.mu.Lock()
	defer t.mu.Unlock()

	status := &TransferStatus{
		ID:          id,
		State:       core.StateQueued,
		Strategy:    strategy,
		Source:      source,
		Destination: destination,
		StartTime:   time.Now(),
		UpdateTime:  time.Now(),
	}

	t.transfers[id] = status
	_ = t.save(status)
}

// Start marks a transfer as started
func (t *Tracker) Start(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if status, exists := t.transfers[id]; exists {
		status.State = core.StateRunning
		status.UpdateTime = time.Now()
		_ = t.save(status)
	}
}

// Update updates transfer progress
func (t *Tracker) Update(id string, bytesDone, bytesTotal, filesDone, filesTotal int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if status, exists := t.transfers[id]; exists {
		status.BytesDone = bytesDone
		status.BytesTotal = bytesTotal
		status.FilesDone = filesDone
		status.FilesTotal = filesTotal
		status.UpdateTime = time.Now()

		// Calculate progress
		if bytesTotal > 0 {
			status.Progress = float64(bytesDone) / float64(bytesTotal) * 100
		}

		_ = t.save(status)
	}
}

// Complete marks a transfer as completed
func (t *Tracker) Complete(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if status, exists := t.transfers[id]; exists {
		status.State = core.StateCompleted
		status.Progress = 100.0
		now := time.Now()
		status.EndTime = &now
		status.UpdateTime = now
		_ = t.save(status)
	}
}

// Fail marks a transfer as failed
func (t *Tracker) Fail(id string, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if status, exists := t.transfers[id]; exists {
		status.State = core.StateFailed
		status.Error = err.Error()
		now := time.Now()
		status.EndTime = &now
		status.UpdateTime = now
		_ = t.save(status)
	}
}

// Get retrieves the status of a transfer
func (t *Tracker) Get(id string) (*TransferStatus, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	status, exists := t.transfers[id]
	if !exists {
		return nil, fmt.Errorf("transfer not found: %s", id)
	}

	// Return a copy
	statusCopy := *status
	return &statusCopy, nil
}

// List returns all transfer statuses
func (t *Tracker) List() []*TransferStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]*TransferStatus, 0, len(t.transfers))
	for _, status := range t.transfers {
		statusCopy := *status
		result = append(result, &statusCopy)
	}

	return result
}

// ListByState returns transfers in a specific state
func (t *Tracker) ListByState(state core.TransferState) []*TransferStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]*TransferStatus, 0)
	for _, status := range t.transfers {
		if status.State == state {
			statusCopy := *status
			result = append(result, &statusCopy)
		}
	}

	return result
}

// Delete removes a transfer from tracking
func (t *Tracker) Delete(id string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.transfers, id)

	// Delete state file
	path := filepath.Join(t.stateDir, id+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

// Clean removes old completed transfers
func (t *Tracker) Clean(maxAge time.Duration) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	toDelete := make([]string, 0)

	for id, status := range t.transfers {
		if status.State == core.StateCompleted || status.State == core.StateFailed {
			if status.EndTime != nil && status.EndTime.Before(cutoff) {
				toDelete = append(toDelete, id)
			}
		}
	}

	for _, id := range toDelete {
		delete(t.transfers, id)
		path := filepath.Join(t.stateDir, id+".json")
		_ = os.Remove(path)
	}

	return nil
}

// save persists a transfer status to disk
func (t *Tracker) save(status *TransferStatus) error {
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(t.stateDir, status.ID+".json")
	return os.WriteFile(path, data, 0644)
}

// loadAll loads all status files from disk
func (t *Tracker) loadAll() error {
	entries, err := os.ReadDir(t.stateDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(t.stateDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var status TransferStatus
		if err := json.Unmarshal(data, &status); err != nil {
			continue
		}

		t.transfers[status.ID] = &status
	}

	return nil
}

// GlobalTracker is the global status tracker
var GlobalTracker *Tracker

// Initialize initializes the global tracker
func Initialize() error {
	var err error
	GlobalTracker, err = New()
	return err
}
