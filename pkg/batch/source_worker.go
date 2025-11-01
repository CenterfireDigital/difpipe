package batch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// SourceWorkerPool manages workers that create tar archives from source
type SourceWorkerPool struct {
	manifest      *Manifest
	bufferMgr     *BufferManager
	sourceAuth    map[string]interface{}
	sourceHost    string
	sourcePath    string
	numWorkers    int
	batchQueue    chan *Batch
	wg            sync.WaitGroup
	errorChan     chan error
	stopChan      chan struct{}
	config        *Config
}

// NewSourceWorkerPool creates a new source worker pool
func NewSourceWorkerPool(manifest *Manifest, bufferMgr *BufferManager, sourceAuth map[string]interface{}, sourceHost, sourcePath string, config *Config) *SourceWorkerPool {
	return &SourceWorkerPool{
		manifest:    manifest,
		bufferMgr:   bufferMgr,
		sourceAuth:  sourceAuth,
		sourceHost:  sourceHost,
		sourcePath:  sourcePath,
		numWorkers:  config.SourceWorkers,
		batchQueue:  make(chan *Batch, config.SourceWorkers*2), // Buffered queue
		errorChan:   make(chan error, config.SourceWorkers),
		stopChan:    make(chan struct{}),
		config:      config,
	}
}

// Start starts all source workers
func (swp *SourceWorkerPool) Start() {
	for i := 0; i < swp.numWorkers; i++ {
		swp.wg.Add(1)
		go swp.worker(i)
	}
}

// Stop stops all source workers gracefully
func (swp *SourceWorkerPool) Stop() {
	close(swp.stopChan)
	swp.wg.Wait()
	close(swp.errorChan)
}

// EnqueueBatch adds a batch to the processing queue
func (swp *SourceWorkerPool) EnqueueBatch(batch *Batch) error {
	select {
	case swp.batchQueue <- batch:
		return nil
	case <-swp.stopChan:
		return fmt.Errorf("worker pool stopped")
	}
}

// Errors returns the error channel for monitoring
func (swp *SourceWorkerPool) Errors() <-chan error {
	return swp.errorChan
}

// worker processes batches from the queue
func (swp *SourceWorkerPool) worker(id int) {
	defer swp.wg.Done()

	for {
		select {
		case <-swp.stopChan:
			return
		case batch, ok := <-swp.batchQueue:
			if !ok {
				return
			}

			if err := swp.processBatch(batch); err != nil {
				batch.SetError(err)
				select {
				case swp.errorChan <- fmt.Errorf("worker %d: batch %d failed: %w", id, batch.ID, err):
				default:
				}
			}
		}
	}
}

// processBatch creates a tar archive for a batch and writes it to the buffer
func (swp *SourceWorkerPool) processBatch(batch *Batch) error {
	batch.SetStatus("downloading")

	// Ensure buffer directory exists
	if err := swp.bufferMgr.EnsureBatchDir(swp.manifest.ID); err != nil {
		return fmt.Errorf("ensure batch dir: %w", err)
	}

	// Get batch path in buffer
	batchPath := swp.bufferMgr.GetBatchPath(swp.manifest.ID, batch.ID)

	// Reserve space in buffer
	for !swp.bufferMgr.ReserveSpace(batch.Size) {
		// Buffer full - wait for space
		// TODO: Add smarter backpressure (sleep, exponential backoff, etc.)
		select {
		case <-swp.stopChan:
			return fmt.Errorf("stopped while waiting for buffer space")
		default:
			// In production, we'd sleep here or use a condition variable
			// For now, just spin (will be improved)
		}
	}

	// Create file list for tar
	fileListPath, err := swp.createFileList(batch)
	if err != nil {
		swp.bufferMgr.ReleaseSpace(batch.Size)
		return fmt.Errorf("create file list: %w", err)
	}
	defer os.Remove(fileListPath) // Clean up file list

	// Create tar archive
	if err := swp.createTarArchive(batch, fileListPath, batchPath); err != nil {
		swp.bufferMgr.ReleaseSpace(batch.Size)
		return fmt.Errorf("create tar: %w", err)
	}

	// Update batch status
	batch.SetLocalPath(batchPath)
	batch.SetStatus("buffered")

	return nil
}

// createFileList creates a temporary file with the list of files for tar
func (swp *SourceWorkerPool) createFileList(batch *Batch) (string, error) {
	// Create file list directory
	fileListDir := filepath.Join("/tmp", "difpipe-filelists", swp.manifest.ID)
	if err := os.MkdirAll(fileListDir, 0755); err != nil {
		return "", fmt.Errorf("create file list dir: %w", err)
	}

	// Create file list
	fileListPath := filepath.Join(fileListDir, fmt.Sprintf("batch_%05d.list", batch.ID))
	f, err := os.Create(fileListPath)
	if err != nil {
		return "", fmt.Errorf("create file list: %w", err)
	}
	defer f.Close()

	// Write file paths
	for _, file := range batch.Files {
		if _, err := fmt.Fprintln(f, file); err != nil {
			return "", fmt.Errorf("write file list: %w", err)
		}
	}

	return fileListPath, nil
}

// createTarArchive creates a tar.gz archive from the source files
func (swp *SourceWorkerPool) createTarArchive(batch *Batch, fileListPath, outputPath string) error {
	var cmd *exec.Cmd

	if swp.sourceHost == "" {
		// Local filesystem - use tar directly
		cmd = exec.Command("tar", "czf", outputPath, "-C", swp.sourcePath, "-T", fileListPath)
	} else {
		// Remote via SSH - stream tar over SSH
		username := "root" // default
		password := ""

		if swp.sourceAuth != nil {
			if u, ok := swp.sourceAuth["username"].(string); ok {
				username = u
			}
			if p, ok := swp.sourceAuth["password"].(string); ok {
				password = p
			}
		}

		// Build remote tar command - cd into directory first then use relative paths
		// This matches how we enumerate files (cd && find .)
		remoteCmd := fmt.Sprintf("cd %s && tar czf - -T -", swp.sourcePath)

		if password != "" {
			// Use sshpass for password auth - use env var to avoid shell escaping issues
			cmd = exec.Command("bash", "-c",
				fmt.Sprintf("cat '%s' | SSHPASS='%s' sshpass -e ssh -o StrictHostKeyChecking=no %s@%s '%s' > '%s'",
					fileListPath, password, username, swp.sourceHost, remoteCmd, outputPath))
		} else {
			// Use SSH without password (key auth)
			cmd = exec.Command("bash", "-c",
				fmt.Sprintf("cat '%s' | ssh -o StrictHostKeyChecking=no %s@%s '%s' > '%s'",
					fileListPath, username, swp.sourceHost, remoteCmd, outputPath))
		}
	}

	// Run tar command
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tar failed: %w (output: %s)", err, string(output))
	}

	// Verify archive was created
	info, err := os.Stat(outputPath)
	if err != nil {
		return fmt.Errorf("verify archive: %w", err)
	}

	// Update actual size if different from estimate
	if info.Size() != batch.Size {
		// Adjust buffer accounting
		diff := info.Size() - batch.Size
		if diff > 0 {
			swp.bufferMgr.ReserveSpace(diff)
		} else {
			swp.bufferMgr.ReleaseSpace(-diff)
		}
		batch.Size = info.Size()
	}

	return nil
}

// parseRemoteLocation parses a location string (user@host:path or just path)
func parseRemoteLocation(location string) (host, path string) {
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
