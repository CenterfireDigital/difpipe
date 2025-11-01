package batch

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// BatchedEngine coordinates batched tar transfers
type BatchedEngine struct {
	manifest     *Manifest
	bufferMgr    *BufferManager
	sourcePool   *SourceWorkerPool
	destPool     *DestWorkerPool
	config       *Config
	sourceAuth   map[string]interface{}
	destAuth     map[string]interface{}

	// Coordination
	mutex        sync.RWMutex
	stopChan     chan struct{}
	errorChan    chan error
	completed    bool
}

// NewBatchedEngine creates a new batched tar engine
func NewBatchedEngine(config *Config, sourceAuth, destAuth map[string]interface{}) *BatchedEngine {
	return &BatchedEngine{
		config:     config,
		sourceAuth: sourceAuth,
		destAuth:   destAuth,
		stopChan:   make(chan struct{}),
		errorChan:  make(chan error, 10),
	}
}

// Transfer executes a batched tar transfer
func (be *BatchedEngine) Transfer(source, destination string) error {
	// Setup signal handlers for graceful shutdown
	be.setupSignalHandlers()

	// Create manifest
	mc := NewManifestCreator(be.sourceAuth, be.destAuth, be.config)
	manifest, err := mc.CreateManifest(source, destination)
	if err != nil {
		return fmt.Errorf("create manifest: %w", err)
	}
	be.manifest = manifest
	manifest.SetStatus("in_progress")

	fmt.Printf("Created manifest: %d files, %d batches, %.2f GB total\n",
		manifest.TotalFiles, len(manifest.Batches), float64(manifest.TotalSize)/(1024*1024*1024))

	// Save initial checkpoint if enabled
	if be.config.CheckpointEnabled {
		if err := manifest.Save(be.config.CheckpointPath); err != nil {
			fmt.Printf("Warning: failed to save initial checkpoint: %v\n", err)
		}
	}

	// Initialize buffer manager
	be.bufferMgr = NewBufferManager(be.config)
	if err := be.bufferMgr.Initialize(); err != nil {
		return fmt.Errorf("initialize buffer: %w", err)
	}

	fmt.Printf("Buffer initialized: max %.2f GB at %s\n",
		float64(be.bufferMgr.GetMaxSize())/(1024*1024*1024), be.config.BufferPath)

	// Parse source and destination
	sourceHost, sourcePath := parseRemoteLocation(source)
	destHost, destPath := parseRemoteLocation(destination)

	// Create worker pools
	be.sourcePool = NewSourceWorkerPool(manifest, be.bufferMgr, be.sourceAuth, sourceHost, sourcePath, be.config)
	be.destPool = NewDestWorkerPool(manifest, be.bufferMgr, be.destAuth, destHost, destPath, be.config)

	// Start workers
	be.sourcePool.Start()
	be.destPool.Start()

	fmt.Printf("Started %d source workers and %d dest workers\n",
		be.config.SourceWorkers, be.config.DestWorkers)

	// Coordinate transfer
	if err := be.coordinateTransfer(); err != nil {
		be.cleanup(false)
		return fmt.Errorf("transfer failed: %w", err)
	}

	// Cleanup on success
	be.cleanup(true)

	manifest.SetStatus("completed")
	fmt.Printf("Transfer completed successfully: %d batches\n", len(manifest.Batches))

	return nil
}

// coordinateTransfer manages the flow of batches through the pipeline
func (be *BatchedEngine) coordinateTransfer() error {
	// Start error monitor
	errWg := sync.WaitGroup{}
	errWg.Add(1)
	transferErr := make(chan error, 1)

	go func() {
		defer errWg.Done()
		for {
			select {
			case err := <-be.sourcePool.Errors():
				transferErr <- fmt.Errorf("source error: %w", err)
				return
			case err := <-be.destPool.Errors():
				transferErr <- fmt.Errorf("dest error: %w", err)
				return
			case <-be.stopChan:
				return
			}
		}
	}()

	// Feed source workers with pending batches
	sourceWg := sync.WaitGroup{}
	sourceWg.Add(1)
	go func() {
		defer sourceWg.Done()
		for _, batch := range be.manifest.Batches {
			select {
			case <-be.stopChan:
				return
			default:
				if err := be.sourcePool.EnqueueBatch(batch); err != nil {
					transferErr <- fmt.Errorf("enqueue source batch: %w", err)
					return
				}
			}
		}
	}()

	// Monitor for buffered batches and feed to destination workers
	// Track which batches have been enqueued to avoid double-enqueueing
	enqueuedToDest := make(map[int]bool)
	var enqueuedMutex sync.Mutex

	destWg := sync.WaitGroup{}
	destWg.Add(1)
	go func() {
		defer destWg.Done()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-be.stopChan:
				return
			case <-ticker.C:
				// Check for buffered batches that haven't been enqueued yet
				enqueuedMutex.Lock()
				for _, batch := range be.manifest.Batches {
					if batch.GetStatus() == "buffered" && !enqueuedToDest[batch.ID] {
						if err := be.destPool.EnqueueBatch(batch); err != nil {
							enqueuedMutex.Unlock()
							transferErr <- fmt.Errorf("enqueue dest batch: %w", err)
							return
						}
						enqueuedToDest[batch.ID] = true
					}
				}
				enqueuedMutex.Unlock()

				// Check if all batches are completed
				if be.allBatchesCompleted() {
					return
				}
			}
		}
	}()

	// Wait for completion or error
	done := make(chan struct{})
	go func() {
		sourceWg.Wait()
		destWg.Wait()
		close(done)
	}()

	select {
	case err := <-transferErr:
		close(be.stopChan)
		return err
	case <-done:
		close(be.stopChan)
		errWg.Wait()
		return nil
	}
}

// allBatchesCompleted checks if all batches are completed
func (be *BatchedEngine) allBatchesCompleted() bool {
	be.mutex.RLock()
	defer be.mutex.RUnlock()

	for _, batch := range be.manifest.Batches {
		status := batch.GetStatus()
		if status != "completed" && status != "failed" {
			return false
		}
	}
	return true
}

// cleanup performs cleanup based on success/failure
func (be *BatchedEngine) cleanup(success bool) {
	// Stop workers
	if be.sourcePool != nil {
		be.sourcePool.Stop()
	}
	if be.destPool != nil {
		be.destPool.Stop()
	}

	// Clean up buffer
	if be.bufferMgr != nil && be.manifest != nil {
		if success && be.config.CleanupBuffer {
			if err := be.bufferMgr.Cleanup(be.manifest.ID); err != nil {
				fmt.Printf("Warning: failed to cleanup buffer: %v\n", err)
			} else {
				fmt.Println("Buffer cleaned up successfully")
			}
		} else if !success && !be.config.KeepOnFailure {
			if err := be.bufferMgr.Cleanup(be.manifest.ID); err != nil {
				fmt.Printf("Warning: failed to cleanup buffer: %v\n", err)
			}
		} else if !success {
			fmt.Printf("Buffer preserved at: %s/%s\n", be.config.BufferPath, be.manifest.ID)
			fmt.Printf("Checkpoint: %s\n", be.config.CheckpointPath)
		}
	}

	// Save final checkpoint
	if be.config.CheckpointEnabled && be.manifest != nil {
		if err := be.manifest.Save(be.config.CheckpointPath); err != nil {
			fmt.Printf("Warning: failed to save final checkpoint: %v\n", err)
		}
	}
}

// setupSignalHandlers sets up handlers for graceful shutdown on Ctrl+C
func (be *BatchedEngine) setupSignalHandlers() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nInterrupt received, shutting down gracefully...")
		be.emergencyCleanup()
		os.Exit(130)
	}()
}

// emergencyCleanup performs emergency cleanup on signal
func (be *BatchedEngine) emergencyCleanup() {
	close(be.stopChan)

	// Save checkpoint
	if be.config.CheckpointEnabled && be.manifest != nil {
		if err := be.manifest.Save(be.config.CheckpointPath); err != nil {
			fmt.Printf("Error saving checkpoint: %v\n", err)
		} else {
			fmt.Printf("Checkpoint saved: %s\n", be.config.CheckpointPath)
		}
	}

	// Clean up or preserve buffer based on config
	if be.bufferMgr != nil && be.manifest != nil {
		if !be.config.KeepOnFailure {
			if err := be.bufferMgr.Cleanup(be.manifest.ID); err != nil {
				fmt.Printf("Error cleaning buffer: %v\n", err)
			}
		} else {
			fmt.Printf("Buffer preserved for resume: %s/%s\n", be.config.BufferPath, be.manifest.ID)
		}
	}
}

// GetProgress returns the current progress
func (be *BatchedEngine) GetProgress() (completed, total int, percentage float64) {
	if be.manifest == nil {
		return 0, 0, 0.0
	}
	return be.manifest.GetProgress()
}
