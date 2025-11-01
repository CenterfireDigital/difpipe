package buffer

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

var (
	// ErrBufferClosed is returned when operations are attempted on a closed buffer
	ErrBufferClosed = errors.New("buffer is closed")

	// ErrTimeout is returned when an operation times out
	ErrTimeout = errors.New("operation timed out")
)

// Buffer represents a streaming buffer interface
type Buffer interface {
	io.Reader
	io.Writer
	io.Closer

	// Available returns the number of bytes available to read
	Available() int

	// Free returns the number of bytes available to write
	Free() int

	// Reset clears the buffer
	Reset()

	// Len returns the total size of the buffer
	Len() int
}

// CircularBuffer implements a thread-safe circular buffer with backpressure
type CircularBuffer struct {
	buf       []byte
	size      int
	readPos   int
	writePos  int
	available int // bytes available to read

	mutex    sync.Mutex
	notEmpty *sync.Cond
	notFull  *sync.Cond

	closed bool
	err    error

	config *Config
}

// Config contains buffer configuration
type Config struct {
	// Size is the buffer size in bytes (default: 1MB)
	Size int

	// BlockOnFull enables blocking writes when buffer is full (backpressure)
	// If false, Write will return an error when buffer is full
	BlockOnFull bool

	// Timeout for blocked operations (0 = no timeout)
	Timeout time.Duration
}

// DefaultConfig returns the default buffer configuration
func DefaultConfig() *Config {
	return &Config{
		Size:        1024 * 1024, // 1MB
		BlockOnFull: true,
		Timeout:     0, // No timeout
	}
}

// New creates a new circular buffer with the given configuration
func New(config *Config) *CircularBuffer {
	if config == nil {
		config = DefaultConfig()
	}

	if config.Size <= 0 {
		config.Size = DefaultConfig().Size
	}

	buf := &CircularBuffer{
		buf:    make([]byte, config.Size),
		size:   config.Size,
		config: config,
	}

	buf.notEmpty = sync.NewCond(&buf.mutex)
	buf.notFull = sync.NewCond(&buf.mutex)

	return buf
}

// Write writes data to the buffer
func (b *CircularBuffer) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.closed {
		return 0, ErrBufferClosed
	}

	totalWritten := 0

	for totalWritten < len(p) {
		// Check if buffer is full
		for b.available == b.size {
			if !b.config.BlockOnFull {
				return totalWritten, fmt.Errorf("buffer full")
			}

			// Wait for space to become available
			if b.config.Timeout > 0 {
				if err := b.waitWithTimeout(b.notFull, b.config.Timeout); err != nil {
					return totalWritten, err
				}
			} else {
				b.notFull.Wait()
			}

			// Check if closed while waiting
			if b.closed {
				return totalWritten, ErrBufferClosed
			}
		}

		// Calculate how much we can write in this iteration
		freeSpace := b.size - b.available
		toWrite := len(p) - totalWritten
		if toWrite > freeSpace {
			toWrite = freeSpace
		}

		// Write data (may wrap around)
		if b.writePos + toWrite <= b.size {
			// Single contiguous write
			copy(b.buf[b.writePos:], p[totalWritten:totalWritten+toWrite])
			b.writePos += toWrite
			if b.writePos == b.size {
				b.writePos = 0
			}
		} else {
			// Write wraps around
			firstPart := b.size - b.writePos
			copy(b.buf[b.writePos:], p[totalWritten:totalWritten+firstPart])
			secondPart := toWrite - firstPart
			copy(b.buf[0:], p[totalWritten+firstPart:totalWritten+toWrite])
			b.writePos = secondPart
		}

		b.available += toWrite
		totalWritten += toWrite

		// Signal that data is available
		b.notEmpty.Signal()
	}

	return totalWritten, nil
}

// Read reads data from the buffer
func (b *CircularBuffer) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	b.mutex.Lock()
	defer b.mutex.Unlock()

	// Wait for data to be available
	for b.available == 0 {
		if b.closed {
			if b.err != nil {
				return 0, b.err
			}
			return 0, io.EOF
		}

		if b.config.Timeout > 0 {
			if err := b.waitWithTimeout(b.notEmpty, b.config.Timeout); err != nil {
				return 0, err
			}
		} else {
			b.notEmpty.Wait()
		}
	}

	// Calculate how much we can read
	toRead := len(p)
	if toRead > b.available {
		toRead = b.available
	}

	// Read data (may wrap around)
	if b.readPos + toRead <= b.size {
		// Single contiguous read
		copy(p, b.buf[b.readPos:b.readPos+toRead])
		b.readPos += toRead
		if b.readPos == b.size {
			b.readPos = 0
		}
	} else {
		// Read wraps around
		firstPart := b.size - b.readPos
		copy(p, b.buf[b.readPos:])
		secondPart := toRead - firstPart
		copy(p[firstPart:], b.buf[0:secondPart])
		b.readPos = secondPart
	}

	b.available -= toRead

	// Signal that space is available
	b.notFull.Signal()

	return toRead, nil
}

// Close closes the buffer
func (b *CircularBuffer) Close() error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.closed {
		return nil
	}

	b.closed = true
	b.notEmpty.Broadcast()
	b.notFull.Broadcast()

	return nil
}

// CloseWithError closes the buffer with an error
func (b *CircularBuffer) CloseWithError(err error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.closed {
		return
	}

	b.closed = true
	b.err = err
	b.notEmpty.Broadcast()
	b.notFull.Broadcast()
}

// Available returns the number of bytes available to read
func (b *CircularBuffer) Available() int {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.available
}

// Free returns the number of bytes available to write
func (b *CircularBuffer) Free() int {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.size - b.available
}

// Len returns the total size of the buffer
func (b *CircularBuffer) Len() int {
	return b.size
}

// Reset clears the buffer
func (b *CircularBuffer) Reset() {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.readPos = 0
	b.writePos = 0
	b.available = 0
	b.closed = false
	b.err = nil
}

// waitWithTimeout waits on a condition variable with timeout
func (b *CircularBuffer) waitWithTimeout(cond *sync.Cond, timeout time.Duration) error {
	done := make(chan struct{})

	go func() {
		cond.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return ErrTimeout
	}
}

// Stats returns buffer statistics
type Stats struct {
	Size      int
	Available int
	Free      int
	Closed    bool
}

// Stats returns current buffer statistics
func (b *CircularBuffer) Stats() Stats {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	return Stats{
		Size:      b.size,
		Available: b.available,
		Free:      b.size - b.available,
		Closed:    b.closed,
	}
}
