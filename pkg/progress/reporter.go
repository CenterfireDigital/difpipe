package progress

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// Reporter implements real-time progress reporting
type Reporter struct {
	writer     io.Writer
	mu         sync.Mutex
	startTime  time.Time
	lastUpdate time.Time
	state      *State
	format     Format
}

// State represents the current progress state
type State struct {
	BytesTotal   int64
	BytesDone    int64
	FilesTotal   int64
	FilesDone    int64
	CurrentFile  string
	Speed        int64 // bytes per second
	ETA          time.Duration
	Status       string
}

// Format represents output format for progress
type Format string

const (
	FormatSimple Format = "simple" // Simple text output
	FormatBar    Format = "bar"    // Progress bar
	FormatJSON   Format = "json"   // JSON output
	FormatNone   Format = "none"   // No output
)

// New creates a new progress reporter
func New(writer io.Writer, format Format) *Reporter {
	return &Reporter{
		writer: writer,
		format: format,
		state: &State{
			Status: "initialized",
		},
	}
}

// Start signals the beginning of a transfer
func (r *Reporter) Start(total int64, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.startTime = time.Now()
	r.lastUpdate = time.Now()
	r.state.BytesTotal = total
	r.state.Status = "running"

	r.render(message)
}

// Update reports progress
func (r *Reporter) Update(done int64, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	r.state.BytesDone = done
	r.state.CurrentFile = message

	// Calculate speed
	elapsed := now.Sub(r.startTime).Seconds()
	if elapsed > 0 {
		r.state.Speed = int64(float64(done) / elapsed)
	}

	// Calculate ETA
	if r.state.Speed > 0 && r.state.BytesTotal > 0 {
		remaining := r.state.BytesTotal - r.state.BytesDone
		if remaining > 0 {
			seconds := float64(remaining) / float64(r.state.Speed)
			r.state.ETA = time.Duration(seconds) * time.Second
		}
	}

	r.lastUpdate = now
	r.render(message)
}

// Complete signals successful completion
func (r *Reporter) Complete(message string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.state.Status = "completed"
	r.render(message)

	// Print final newline for non-JSON formats
	if r.format != FormatJSON {
		fmt.Fprintln(r.writer)
	}
}

// Error reports an error
func (r *Reporter) Error(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.state.Status = "failed"
	r.render(fmt.Sprintf("Error: %v", err))

	if r.format != FormatJSON {
		fmt.Fprintln(r.writer)
	}
}

// render outputs the progress based on format
func (r *Reporter) render(message string) {
	switch r.format {
	case FormatSimple:
		r.renderSimple(message)
	case FormatBar:
		r.renderBar(message)
	case FormatJSON:
		r.renderJSON()
	case FormatNone:
		// No output
	}
}

// renderSimple outputs simple text progress
func (r *Reporter) renderSimple(message string) {
	percent := 0.0
	if r.state.BytesTotal > 0 {
		percent = float64(r.state.BytesDone) / float64(r.state.BytesTotal) * 100
	}

	speed := formatSpeed(r.state.Speed)
	eta := formatDuration(r.state.ETA)

	fmt.Fprintf(r.writer, "\r%-50s %6.1f%% %10s ETA: %8s",
		truncate(message, 50),
		percent,
		speed,
		eta,
	)
}

// renderBar outputs a progress bar
func (r *Reporter) renderBar(message string) {
	barWidth := 40
	percent := 0.0
	if r.state.BytesTotal > 0 {
		percent = float64(r.state.BytesDone) / float64(r.state.BytesTotal) * 100
	}

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

	speed := formatSpeed(r.state.Speed)
	eta := formatDuration(r.state.ETA)
	bytes := formatBytes(r.state.BytesDone)

	fmt.Fprintf(r.writer, "\r%s %6.1f%% %10s %10s ETA: %8s",
		bar,
		percent,
		bytes,
		speed,
		eta,
	)
}

// renderJSON outputs JSON progress
func (r *Reporter) renderJSON() {
	percent := 0.0
	if r.state.BytesTotal > 0 {
		percent = float64(r.state.BytesDone) / float64(r.state.BytesTotal) * 100
	}

	fmt.Fprintf(r.writer, `{"timestamp":"%s","bytes_done":%d,"bytes_total":%d,"percent":%.2f,"speed":"%s","eta":"%s","status":"%s"}`+"\n",
		time.Now().Format(time.RFC3339),
		r.state.BytesDone,
		r.state.BytesTotal,
		percent,
		formatSpeed(r.state.Speed),
		formatDuration(r.state.ETA),
		r.state.Status,
	)
}

// GetState returns the current progress state
func (r *Reporter) GetState() *State {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Return a copy
	stateCopy := *r.state
	return &stateCopy
}

// formatSpeed formats bytes per second
func formatSpeed(bytesPerSec int64) string {
	if bytesPerSec == 0 {
		return "0 B/s"
	}

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

// formatBytes formats byte count
func formatBytes(bytes int64) string {
	if bytes == 0 {
		return "0 B"
	}

	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	units := []string{"KB", "MB", "GB", "TB"}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp])
}

// formatDuration formats duration as human-readable string
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "--:--:--"
	}

	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

// truncate truncates a string to max length
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
