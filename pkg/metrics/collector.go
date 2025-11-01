package metrics

import (
	"sync"
	"time"
)

// Collector collects transfer metrics
type Collector struct {
	mu      sync.RWMutex
	metrics map[string]*Metric
}

// Metric represents a single metric
type Metric struct {
	Name      string
	Value     float64
	Timestamp time.Time
	Labels    map[string]string
}

// Summary contains aggregated metrics for a transfer
type Summary struct {
	TransferID       string
	Strategy         string
	BytesTransferred int64
	FilesTransferred int64
	Duration         time.Duration
	AverageSpeed     float64
	Success          bool
	ErrorCount       int
	RetryCount       int
	Timestamp        time.Time
}

// New creates a new metrics collector
func New() *Collector {
	return &Collector{
		metrics: make(map[string]*Metric),
	}
}

// Record records a metric value
func (c *Collector) Record(name string, value float64, labels map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := buildKey(name, labels)
	c.metrics[key] = &Metric{
		Name:      name,
		Value:     value,
		Timestamp: time.Now(),
		Labels:    labels,
	}
}

// Increment increments a counter metric
func (c *Collector) Increment(name string, labels map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := buildKey(name, labels)
	if m, exists := c.metrics[key]; exists {
		m.Value++
		m.Timestamp = time.Now()
	} else {
		c.metrics[key] = &Metric{
			Name:      name,
			Value:     1,
			Timestamp: time.Now(),
			Labels:    labels,
		}
	}
}

// Observe records an observation for a histogram/summary metric
func (c *Collector) Observe(name string, value float64, labels map[string]string) {
	c.Record(name, value, labels)
}

// Get retrieves a metric by name and labels
func (c *Collector) Get(name string, labels map[string]string) *Metric {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := buildKey(name, labels)
	if m, exists := c.metrics[key]; exists {
		// Return a copy
		metricCopy := *m
		return &metricCopy
	}
	return nil
}

// GetAll returns all metrics
func (c *Collector) GetAll() []*Metric {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]*Metric, 0, len(c.metrics))
	for _, m := range c.metrics {
		metricCopy := *m
		result = append(result, &metricCopy)
	}
	return result
}

// Reset clears all metrics
func (c *Collector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.metrics = make(map[string]*Metric)
}

// RecordTransfer records metrics for a complete transfer
func (c *Collector) RecordTransfer(summary *Summary) {
	labels := map[string]string{
		"transfer_id": summary.TransferID,
		"strategy":    summary.Strategy,
	}

	// Record bytes transferred
	c.Record("transfer_bytes_total", float64(summary.BytesTransferred), labels)

	// Record files transferred
	c.Record("transfer_files_total", float64(summary.FilesTransferred), labels)

	// Record duration in seconds
	c.Record("transfer_duration_seconds", summary.Duration.Seconds(), labels)

	// Record average speed
	c.Record("transfer_speed_bytes_per_second", summary.AverageSpeed, labels)

	// Record success/failure
	if summary.Success {
		c.Increment("transfer_success_total", labels)
	} else {
		c.Increment("transfer_failure_total", labels)
	}

	// Record errors
	if summary.ErrorCount > 0 {
		c.Record("transfer_errors_total", float64(summary.ErrorCount), labels)
	}

	// Record retries
	if summary.RetryCount > 0 {
		c.Record("transfer_retries_total", float64(summary.RetryCount), labels)
	}
}

// GetTransferMetrics returns metrics summary for a specific transfer
func (c *Collector) GetTransferMetrics(transferID string) *Summary {
	c.mu.RLock()
	defer c.mu.RUnlock()

	labels := map[string]string{"transfer_id": transferID}

	summary := &Summary{
		TransferID: transferID,
	}

	// Extract metrics
	if m := c.Get("transfer_bytes_total", labels); m != nil {
		summary.BytesTransferred = int64(m.Value)
	}
	if m := c.Get("transfer_files_total", labels); m != nil {
		summary.FilesTransferred = int64(m.Value)
	}
	if m := c.Get("transfer_duration_seconds", labels); m != nil {
		summary.Duration = time.Duration(m.Value) * time.Second
	}
	if m := c.Get("transfer_speed_bytes_per_second", labels); m != nil {
		summary.AverageSpeed = m.Value
	}
	if m := c.Get("transfer_success_total", labels); m != nil {
		summary.Success = m.Value > 0
	}
	if m := c.Get("transfer_errors_total", labels); m != nil {
		summary.ErrorCount = int(m.Value)
	}
	if m := c.Get("transfer_retries_total", labels); m != nil {
		summary.RetryCount = int(m.Value)
	}

	return summary
}

// buildKey creates a unique key for a metric with labels
func buildKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}

	key := name
	for k, v := range labels {
		key += ":" + k + "=" + v
	}
	return key
}

// GlobalCollector is the global metrics collector
var GlobalCollector = New()

// Record is a convenience function for recording to the global collector
func Record(name string, value float64, labels map[string]string) {
	GlobalCollector.Record(name, value, labels)
}

// Increment is a convenience function for incrementing on the global collector
func Increment(name string, labels map[string]string) {
	GlobalCollector.Increment(name, labels)
}

// RecordTransfer is a convenience function for recording transfer metrics
func RecordTransfer(summary *Summary) {
	GlobalCollector.RecordTransfer(summary)
}
