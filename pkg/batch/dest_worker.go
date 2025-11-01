package batch

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// DestWorkerPool manages workers that extract tar archives to destination
type DestWorkerPool struct {
	manifest    *Manifest
	bufferMgr   *BufferManager
	destAuth    map[string]interface{}
	destHost    string
	destPath    string
	numWorkers  int
	batchQueue  chan *Batch
	wg          sync.WaitGroup
	errorChan   chan error
	stopChan    chan struct{}
	config      *Config
}

// NewDestWorkerPool creates a new destination worker pool
func NewDestWorkerPool(manifest *Manifest, bufferMgr *BufferManager, destAuth map[string]interface{}, destHost, destPath string, config *Config) *DestWorkerPool {
	return &DestWorkerPool{
		manifest:   manifest,
		bufferMgr:  bufferMgr,
		destAuth:   destAuth,
		destHost:   destHost,
		destPath:   destPath,
		numWorkers: config.DestWorkers,
		batchQueue: make(chan *Batch, config.DestWorkers*2), // Buffered queue
		errorChan:  make(chan error, config.DestWorkers),
		stopChan:   make(chan struct{}),
		config:     config,
	}
}

// Start starts all destination workers
func (dwp *DestWorkerPool) Start() {
	for i := 0; i < dwp.numWorkers; i++ {
		dwp.wg.Add(1)
		go dwp.worker(i)
	}
}

// Stop stops all destination workers gracefully
func (dwp *DestWorkerPool) Stop() {
	close(dwp.stopChan)
	dwp.wg.Wait()
	close(dwp.errorChan)
}

// EnqueueBatch adds a batch to the processing queue
func (dwp *DestWorkerPool) EnqueueBatch(batch *Batch) error {
	select {
	case dwp.batchQueue <- batch:
		return nil
	case <-dwp.stopChan:
		return fmt.Errorf("worker pool stopped")
	}
}

// Errors returns the error channel for monitoring
func (dwp *DestWorkerPool) Errors() <-chan error {
	return dwp.errorChan
}

// worker processes batches from the queue
func (dwp *DestWorkerPool) worker(id int) {
	defer dwp.wg.Done()

	for {
		select {
		case <-dwp.stopChan:
			return
		case batch, ok := <-dwp.batchQueue:
			if !ok {
				return
			}

			if err := dwp.processBatch(batch); err != nil {
				batch.SetError(err)
				select {
				case dwp.errorChan <- fmt.Errorf("worker %d: batch %d failed: %w", id, batch.ID, err):
				default:
				}
			}
		}
	}
}

// processBatch extracts a tar archive to the destination
func (dwp *DestWorkerPool) processBatch(batch *Batch) error {
	batch.SetStatus("uploading")

	// Get batch path from buffer
	batchPath := batch.GetLocalPath()
	if batchPath == "" {
		return fmt.Errorf("batch has no local path")
	}

	// Extract tar archive to destination
	if err := dwp.extractTarArchive(batch, batchPath); err != nil {
		return fmt.Errorf("extract tar: %w", err)
	}

	// Clean up batch file from buffer
	if dwp.config.CleanupBuffer {
		if err := dwp.bufferMgr.DeleteBatch(batchPath, batch.Size); err != nil {
			// Log error but don't fail the batch
			fmt.Printf("Warning: failed to delete batch %d from buffer: %v\n", batch.ID, err)
		}
	}

	// Update batch status
	batch.SetStatus("completed")

	return nil
}

// extractTarArchive extracts a tar.gz archive to the destination
func (dwp *DestWorkerPool) extractTarArchive(batch *Batch, archivePath string) error {
	var cmd *exec.Cmd

	if dwp.destHost == "" {
		// Local filesystem - use tar directly
		cmd = exec.Command("tar", "xzf", archivePath, "-C", dwp.destPath)
	} else {
		// Remote via SSH - stream tar over SSH
		username := "root" // default
		password := ""

		if dwp.destAuth != nil {
			if u, ok := dwp.destAuth["username"].(string); ok {
				username = u
			}
			if p, ok := dwp.destAuth["password"].(string); ok {
				password = p
			}
		}

		// Build remote tar command
		// cat archive | ssh tar xzf - -C <path>
		remoteCmd := fmt.Sprintf("tar xzf - -C %s", dwp.destPath)

		if password != "" {
			// Use sshpass for password auth - use env var to avoid shell escaping issues
			cmd = exec.Command("bash", "-c",
				fmt.Sprintf("cat '%s' | SSHPASS='%s' sshpass -e ssh -o StrictHostKeyChecking=no %s@%s '%s'",
					archivePath, password, username, dwp.destHost, remoteCmd))
		} else {
			// Use SSH without password (key auth)
			cmd = exec.Command("bash", "-c",
				fmt.Sprintf("cat '%s' | ssh -o StrictHostKeyChecking=no %s@%s '%s'",
					archivePath, username, dwp.destHost, remoteCmd))
		}
	}

	// Run tar command
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tar extract failed: %w (output: %s)", err, string(output))
	}

	return nil
}

// parseDestLocation is a helper to parse destination location
func parseDestLocation(location string) (host, path string) {
	// Check for user@host:path format
	if strings.Contains(location, "@") && strings.Contains(location, ":") {
		atIndex := strings.Index(location, "@")
		colonIndex := strings.Index(location, ":")
		if colonIndex > atIndex {
			host = location[atIndex+1 : colonIndex]
			path = location[colonIndex+1:]
			return host, path
		}
	}

	// Check for host:path format (no user)
	if strings.Contains(location, ":") && !strings.HasPrefix(location, "/") {
		parts := strings.SplitN(location, ":", 2)
		if len(parts) == 2 {
			return parts[0], parts[1]
		}
	}

	// Local path
	return "", location
}
