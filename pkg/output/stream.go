package output

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

// ProgressEvent represents a progress update
type ProgressEvent struct {
	Timestamp   time.Time `json:"timestamp"`
	Type        string    `json:"type"` // start, progress, complete, error
	Message     string    `json:"message,omitempty"`
	BytesTotal  int64     `json:"bytes_total,omitempty"`
	BytesDone   int64     `json:"bytes_done,omitempty"`
	FilesTotal  int64     `json:"files_total,omitempty"`
	FilesDone   int64     `json:"files_done,omitempty"`
	Percent     float64   `json:"percent,omitempty"`
	Speed       string    `json:"speed,omitempty"` // e.g., "32 MB/s"
	ETA         string    `json:"eta,omitempty"`   // e.g., "5m30s"
	CurrentFile string    `json:"current_file,omitempty"`
}

// StreamWriter writes newline-delimited JSON progress events
type StreamWriter struct {
	writer io.Writer
	mu     sync.Mutex
}

// NewStreamWriter creates a new stream writer
func NewStreamWriter(writer io.Writer) *StreamWriter {
	return &StreamWriter{
		writer: writer,
	}
}

// Write writes a progress event as newline-delimited JSON
func (s *StreamWriter) Write(event *ProgressEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Set timestamp if not already set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Encode as JSON
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	// Write with newline
	_, err = s.writer.Write(append(data, '\n'))
	return err
}

// Start writes a start event
func (s *StreamWriter) Start(message string) error {
	return s.Write(&ProgressEvent{
		Type:    "start",
		Message: message,
	})
}

// Progress writes a progress event
func (s *StreamWriter) Progress(bytesTotal, bytesDone, filesTotal, filesDone int64) error {
	percent := 0.0
	if bytesTotal > 0 {
		percent = float64(bytesDone) / float64(bytesTotal) * 100
	}

	return s.Write(&ProgressEvent{
		Type:       "progress",
		BytesTotal: bytesTotal,
		BytesDone:  bytesDone,
		FilesTotal: filesTotal,
		FilesDone:  filesDone,
		Percent:    percent,
	})
}

// Complete writes a completion event
func (s *StreamWriter) Complete(message string) error {
	return s.Write(&ProgressEvent{
		Type:    "complete",
		Message: message,
	})
}

// Error writes an error event
func (s *StreamWriter) Error(err error) error {
	return s.Write(&ProgressEvent{
		Type:    "error",
		Message: err.Error(),
	})
}

// Custom writes a custom event with all fields
func (s *StreamWriter) Custom(event *ProgressEvent) error {
	return s.Write(event)
}

// TextProgressWriter writes human-readable progress to text output
type TextProgressWriter struct {
	writer   io.Writer
	mu       sync.Mutex
	lastLine string
}

// NewTextProgressWriter creates a text progress writer
func NewTextProgressWriter(writer io.Writer) *TextProgressWriter {
	return &TextProgressWriter{
		writer: writer,
	}
}

// Update updates the progress display
func (t *TextProgressWriter) Update(message string, percent float64) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Clear previous line if in terminal
	if t.lastLine != "" {
		// Move cursor up and clear line (ANSI escape codes)
		_, _ = t.writer.Write([]byte("\r\033[K"))
	}

	// Format progress bar
	line := formatProgressBar(message, percent)
	_, err := t.writer.Write([]byte(line))
	t.lastLine = line
	return err
}

// Finish completes the progress display
func (t *TextProgressWriter) Finish(message string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	_, err := t.writer.Write([]byte("\n" + message + "\n"))
	t.lastLine = ""
	return err
}

// formatProgressBar creates a text progress bar
func formatProgressBar(message string, percent float64) string {
	barWidth := 40
	filled := int(percent / 100.0 * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}

	bar := "["
	for i := 0; i < barWidth; i++ {
		if i < filled {
			bar += "="
		} else if i == filled {
			bar += ">"
		} else {
			bar += " "
		}
	}
	bar += "]"

	percentStr := "100%"
	if percent < 100.0 {
		percentStr = formatPercent(percent)
	}

	return message + " " + bar + " " + percentStr
}

// formatPercent formats a percentage to 1 decimal place
func formatPercent(percent float64) string {
	whole := int(percent)
	decimal := int((percent - float64(whole)) * 10)

	// Handle single vs double digit whole numbers
	if whole < 10 {
		return string(rune('0'+whole)) + "." + string(rune('0'+decimal)) + "%"
	}
	return string(rune('0'+whole/10)) + string(rune('0'+whole%10)) + "." + string(rune('0'+decimal)) + "%"
}
