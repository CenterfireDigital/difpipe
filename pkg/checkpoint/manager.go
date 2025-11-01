package checkpoint

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/larrydiffey/difpipe/pkg/core"
)

// Manager implements checkpoint storage and retrieval
type Manager struct {
	dir string
}

// New creates a new checkpoint manager
func New(dir string) (*Manager, error) {
	// Default to ~/.difpipe/checkpoints
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		dir = filepath.Join(home, ".difpipe", "checkpoints")
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create checkpoint dir: %w", err)
	}

	return &Manager{dir: dir}, nil
}

// Save saves a checkpoint to disk
func (m *Manager) Save(transferID string, state *core.CheckpointState) error {
	// Update last update time
	state.LastUpdate = time.Now()

	// Marshal to JSON
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}

	// Write to file
	path := m.getCheckpointPath(transferID)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write checkpoint: %w", err)
	}

	return nil
}

// Load loads a checkpoint from disk
func (m *Manager) Load(transferID string) (*core.CheckpointState, error) {
	path := m.getCheckpointPath(transferID)

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("checkpoint not found: %s", transferID)
		}
		return nil, fmt.Errorf("read checkpoint: %w", err)
	}

	// Unmarshal
	var state core.CheckpointState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal checkpoint: %w", err)
	}

	return &state, nil
}

// Delete removes a checkpoint
func (m *Manager) Delete(transferID string) error {
	path := m.getCheckpointPath(transferID)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete checkpoint: %w", err)
	}
	return nil
}

// Exists checks if a checkpoint exists
func (m *Manager) Exists(transferID string) bool {
	path := m.getCheckpointPath(transferID)
	_, err := os.Stat(path)
	return err == nil
}

// List returns all checkpoint IDs
func (m *Manager) List() ([]string, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return nil, fmt.Errorf("read checkpoint dir: %w", err)
	}

	var ids []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			// Remove .json extension
			id := entry.Name()[:len(entry.Name())-5]
			ids = append(ids, id)
		}
	}

	return ids, nil
}

// Clean removes old checkpoints older than the given duration
func (m *Manager) Clean(maxAge time.Duration) error {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return fmt.Errorf("read checkpoint dir: %w", err)
	}

	cutoff := time.Now().Add(-maxAge)
	cleaned := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			path := filepath.Join(m.dir, entry.Name())
			if err := os.Remove(path); err != nil {
				continue
			}
			cleaned++
		}
	}

	return nil
}

// GetCheckpointPath returns the file path for a checkpoint
func (m *Manager) getCheckpointPath(transferID string) string {
	return filepath.Join(m.dir, transferID+".json")
}

// GetDir returns the checkpoint directory
func (m *Manager) GetDir() string {
	return m.dir
}

// AutoSave creates a checkpoint saver that saves periodically
type AutoSaver struct {
	manager    *Manager
	interval   time.Duration
	transferID string
	state      *core.CheckpointState
	done       chan struct{}
}

// NewAutoSaver creates an auto-saver that saves checkpoints at intervals
func NewAutoSaver(manager *Manager, transferID string, state *core.CheckpointState, interval time.Duration) *AutoSaver {
	return &AutoSaver{
		manager:    manager,
		interval:   interval,
		transferID: transferID,
		state:      state,
		done:       make(chan struct{}),
	}
}

// Start begins auto-saving checkpoints
func (a *AutoSaver) Start() {
	ticker := time.NewTicker(a.interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = a.manager.Save(a.transferID, a.state)
			case <-a.done:
				// Save one last time before stopping
				_ = a.manager.Save(a.transferID, a.state)
				return
			}
		}
	}()
}

// Stop stops auto-saving
func (a *AutoSaver) Stop() {
	close(a.done)
}

// Update updates the checkpoint state
func (a *AutoSaver) Update(state *core.CheckpointState) {
	a.state = state
}
