package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// Format represents output format type
type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
	FormatYAML Format = "yaml"
	FormatCSV  Format = "csv"
)

// Formatter handles output formatting
type Formatter struct {
	format Format
	writer io.Writer
}

// New creates a new formatter
func New(format Format, writer io.Writer) *Formatter {
	return &Formatter{
		format: format,
		writer: writer,
	}
}

// Format outputs data in the specified format
func (f *Formatter) Format(data interface{}) error {
	switch f.format {
	case FormatJSON:
		return f.formatJSON(data)
	case FormatYAML:
		return f.formatYAML(data)
	case FormatCSV:
		return f.formatCSV(data)
	case FormatText:
		return f.formatText(data)
	default:
		return fmt.Errorf("unsupported format: %s", f.format)
	}
}

// formatJSON outputs data as JSON
func (f *Formatter) formatJSON(data interface{}) error {
	encoder := json.NewEncoder(f.writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

// formatYAML outputs data as YAML
func (f *Formatter) formatYAML(data interface{}) error {
	encoder := yaml.NewEncoder(f.writer)
	defer encoder.Close()
	return encoder.Encode(data)
}

// formatCSV outputs data as CSV
func (f *Formatter) formatCSV(data interface{}) error {
	writer := csv.NewWriter(f.writer)
	defer writer.Flush()

	// Convert data to [][]string for CSV
	switch v := data.(type) {
	case [][]string:
		return writer.WriteAll(v)
	case map[string]interface{}:
		// Write header
		headers := make([]string, 0, len(v))
		values := make([]string, 0, len(v))
		for key, val := range v {
			headers = append(headers, key)
			values = append(values, fmt.Sprintf("%v", val))
		}
		if err := writer.Write(headers); err != nil {
			return err
		}
		return writer.Write(values)
	default:
		return fmt.Errorf("unsupported CSV data type: %T", data)
	}
}

// formatText outputs data as human-readable text
func (f *Formatter) formatText(data interface{}) error {
	switch v := data.(type) {
	case string:
		_, err := fmt.Fprintln(f.writer, v)
		return err
	case map[string]interface{}:
		return f.formatTextMap(v)
	case []interface{}:
		return f.formatTextList(v)
	default:
		_, err := fmt.Fprintf(f.writer, "%+v\n", v)
		return err
	}
}

// formatTextMap formats a map as key-value pairs
func (f *Formatter) formatTextMap(m map[string]interface{}) error {
	maxKeyLen := 0
	for key := range m {
		if len(key) > maxKeyLen {
			maxKeyLen = len(key)
		}
	}

	for key, val := range m {
		padding := strings.Repeat(" ", maxKeyLen-len(key))
		_, err := fmt.Fprintf(f.writer, "%s:%s %v\n", key, padding, val)
		if err != nil {
			return err
		}
	}
	return nil
}

// formatTextList formats a list with bullets
func (f *Formatter) formatTextList(list []interface{}) error {
	for _, item := range list {
		_, err := fmt.Fprintf(f.writer, "  • %v\n", item)
		if err != nil {
			return err
		}
	}
	return nil
}

// Error formats an error message
func (f *Formatter) Error(err error) error {
	errorData := map[string]interface{}{
		"error": err.Error(),
	}
	return f.Format(errorData)
}

// Success formats a success message
func (f *Formatter) Success(message string) error {
	successData := map[string]interface{}{
		"status":  "success",
		"message": message,
	}
	return f.Format(successData)
}
