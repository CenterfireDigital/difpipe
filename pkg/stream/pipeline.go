package stream

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/larrydiffey/difpipe/pkg/buffer"
)

// Pipeline coordinates streaming data from source to destination through a buffer
type Pipeline struct {
	source      io.Reader
	destination io.Writer
	buffer      buffer.Buffer

	bytesRead    atomic.Int64
	bytesWritten atomic.Int64
	startTime    time.Time

	progressFunc func(bytesTransferred int64, speed float64)
	errorHandler func(error) error

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mutex sync.Mutex
	err   error
}

// Config contains pipeline configuration
type Config struct {
	BufferSize   int
	ProgressFunc func(bytesTransferred int64, speed float64)
	ErrorHandler func(error) error
}

// New creates a new streaming pipeline
func New(source io.Reader, destination io.Writer, config *Config) *Pipeline {
	if config == nil {
		config = &Config{}
	}

	bufferConfig := &buffer.Config{
		Size:        config.BufferSize,
		BlockOnFull: true,
	}

	if bufferConfig.Size == 0 {
		bufferConfig.Size = 1024 * 1024 // 1MB default
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Pipeline{
		source:       source,
		destination:  destination,
		buffer:       buffer.New(bufferConfig),
		progressFunc: config.ProgressFunc,
		errorHandler: config.ErrorHandler,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start starts the pipeline
func (p *Pipeline) Start(ctx context.Context) error {
	p.startTime = time.Now()

	// Use separate wait groups for data transfer and progress reporting
	dataWg := sync.WaitGroup{}

	// Start source reader goroutine
	dataWg.Add(1)
	p.wg.Add(1)
	go func() {
		p.readFromSource()
		dataWg.Done()
	}()

	// Start destination writer goroutine
	dataWg.Add(1)
	p.wg.Add(1)
	go func() {
		p.writeToDestination()
		dataWg.Done()
	}()

	// Start progress reporter goroutine if configured
	if p.progressFunc != nil {
		p.wg.Add(1)
		go p.reportProgress()
	}

	// Wait for completion or context cancellation
	done := make(chan struct{})
	go func() {
		// Wait for data transfer to complete
		dataWg.Wait()
		// Cancel context to signal progress reporter to stop
		p.cancel()
		// Wait for progress reporter to exit
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return p.err
	case <-ctx.Done():
		p.cancel()
		p.wg.Wait()
		if p.err != nil {
			return p.err
		}
		return ctx.Err()
	}
}

// Stop stops the pipeline gracefully
func (p *Pipeline) Stop() error {
	p.cancel()
	p.wg.Wait()
	return p.err
}

// Stats returns current pipeline statistics
func (p *Pipeline) Stats() *Stats {
	bytesRead := p.bytesRead.Load()
	bytesWritten := p.bytesWritten.Load()
	duration := time.Since(p.startTime)

	var speed float64
	if duration.Seconds() > 0 {
		speed = float64(bytesWritten) / duration.Seconds()
	}

	return &Stats{
		BytesRead:    bytesRead,
		BytesWritten: bytesWritten,
		BytesBuffered: int64(p.buffer.Available()),
		Speed:        speed,
		Duration:     duration,
	}
}

// readFromSource reads data from source and writes to buffer
func (p *Pipeline) readFromSource() {
	defer p.wg.Done()
	defer p.buffer.Close()

	buf := make([]byte, 32*1024) // 32KB read buffer

	for {
		select {
		case <-p.ctx.Done():
			return
		default:
		}

		n, err := p.source.Read(buf)
		if n > 0 {
			// Write to buffer
			written, writeErr := p.buffer.Write(buf[:n])
			if writeErr != nil {
				p.setError(fmt.Errorf("write to buffer: %w", writeErr))
				return
			}

			p.bytesRead.Add(int64(written))
		}

		if err != nil {
			if err == io.EOF {
				// Normal end of stream
				return
			}

			p.setError(fmt.Errorf("read from source: %w", err))
			return
		}
	}
}

// writeToDestination reads data from buffer and writes to destination
func (p *Pipeline) writeToDestination() {
	defer p.wg.Done()

	buf := make([]byte, 32*1024) // 32KB write buffer

	for {
		select {
		case <-p.ctx.Done():
			return
		default:
		}

		n, err := p.buffer.Read(buf)
		if n > 0 {
			// Write to destination
			written, writeErr := p.destination.Write(buf[:n])
			if writeErr != nil {
				p.setError(fmt.Errorf("write to destination: %w", writeErr))
				return
			}

			if written != n {
				p.setError(fmt.Errorf("short write: %d != %d", written, n))
				return
			}

			p.bytesWritten.Add(int64(written))
		}

		if err != nil {
			if err == io.EOF {
				// Normal end of stream
				return
			}

			p.setError(fmt.Errorf("read from buffer: %w", err))
			return
		}
	}
}

// reportProgress periodically reports progress
func (p *Pipeline) reportProgress() {
	defer p.wg.Done()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			stats := p.Stats()
			if p.progressFunc != nil {
				p.progressFunc(stats.BytesWritten, stats.Speed)
			}
		}
	}
}

// setError sets the pipeline error
func (p *Pipeline) setError(err error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.err == nil {
		p.err = err

		// Call error handler if configured
		if p.errorHandler != nil {
			p.err = p.errorHandler(err)
		}

		// Cancel context to stop all goroutines
		p.cancel()
	}
}

// Stats contains pipeline statistics
type Stats struct {
	BytesRead     int64
	BytesWritten  int64
	BytesBuffered int64
	Speed         float64 // bytes per second
	Duration      time.Duration
}

// SpeedString returns a human-readable speed string
func (s *Stats) SpeedString() string {
	return formatSpeed(int64(s.Speed))
}

// formatSpeed formats bytes per second as human-readable string
func formatSpeed(bytesPerSec int64) string {
	const unit = 1024
	if bytesPerSec < unit {
		return fmt.Sprintf("%d B/s", bytesPerSec)
	}
	div, exp := int64(unit), 0
	for n := bytesPerSec / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB/s", "MB/s", "GB/s", "TB/s"}
	return fmt.Sprintf("%.1f %s", float64(bytesPerSec)/float64(div), units[exp])
}
