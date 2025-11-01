package batch

import (
	"bufio"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// FileInfo represents a file with its size
type FileInfo struct {
	Path string
	Size int64
}

// ManifestCreator creates manifests by enumerating and batching files
type ManifestCreator struct {
	sourceAuth map[string]interface{}
	destAuth   map[string]interface{}
	config     *Config
}

// NewManifestCreator creates a new manifest creator
func NewManifestCreator(sourceAuth, destAuth map[string]interface{}, config *Config) *ManifestCreator {
	return &ManifestCreator{
		sourceAuth: sourceAuth,
		destAuth:   destAuth,
		config:     config,
	}
}

// CreateManifest enumerates files and creates a batched manifest
func (mc *ManifestCreator) CreateManifest(source, destination string) (*Manifest, error) {
	// Parse source location
	sourceHost, sourcePath, err := mc.parseLocation(source)
	if err != nil {
		return nil, fmt.Errorf("parse source: %w", err)
	}

	// Enumerate files from source
	files, err := mc.enumerateFiles(sourceHost, sourcePath)
	if err != nil {
		return nil, fmt.Errorf("enumerate files: %w", err)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no files found at source")
	}

	// Create manifest
	manifest := NewManifest(source, destination, mc.config.ChunkSizeMB)

	// Bin pack files into batches
	batches := mc.binPackFiles(files)

	// Add batches to manifest
	for _, batch := range batches {
		manifest.AddBatch(batch.files, batch.size)
	}

	return manifest, nil
}

// enumerateFiles lists all files at the remote location with their sizes
func (mc *ManifestCreator) enumerateFiles(host, path string) ([]FileInfo, error) {
	var cmd *exec.Cmd

	if host == "" {
		// Local filesystem
		cmd = exec.Command("find", path, "-type", "f", "-printf", "%s %p\\n")
	} else {
		// Remote via SSH - extract username and password from auth map
		username := "root" // default
		password := ""

		if mc.sourceAuth != nil {
			if u, ok := mc.sourceAuth["username"].(string); ok {
				username = u
			}
			if p, ok := mc.sourceAuth["password"].(string); ok {
				password = p
			}
		}

		// Expand ~ in path by cd'ing into it first
		// This makes all paths relative to that directory
		findCmd := fmt.Sprintf("cd %s && find . -type f -printf '%%s %%p\\n'", path)

		if password != "" {
			// Use sshpass with env var to avoid shell escaping issues
			cmd = exec.Command("sshpass", "-e", "ssh", "-o", "StrictHostKeyChecking=no",
				fmt.Sprintf("%s@%s", username, host), findCmd)
			cmd.Env = append(cmd.Env, fmt.Sprintf("SSHPASS=%s", password))
		} else {
			cmd = exec.Command("ssh", "-o", "StrictHostKeyChecking=no",
				fmt.Sprintf("%s@%s", username, host), findCmd)
		}
	}

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("execute find: %w", err)
	}

	// Parse output
	files := []FileInfo{}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue // Skip malformed lines
		}

		size, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			continue // Skip unparseable sizes
		}

		// Make path relative (remove leading ./)
		relPath := strings.TrimPrefix(parts[1], "./")
		if relPath == "" || relPath == "." {
			continue // Skip empty or current directory
		}

		files = append(files, FileInfo{
			Path: relPath,
			Size: size,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan output: %w", err)
	}

	return files, nil
}

// batchInfo holds temporary batch information during bin packing
type batchInfo struct {
	files []string
	size  int64
}

// binPackFiles uses first-fit bin packing to group files into batches
func (mc *ManifestCreator) binPackFiles(files []FileInfo) []batchInfo {
	targetSize := int64(mc.config.ChunkSizeMB) * 1024 * 1024 // Convert MB to bytes
	batches := []batchInfo{}
	currentBatch := batchInfo{
		files: []string{},
		size:  0,
	}

	for _, file := range files {
		// If adding this file would exceed target, start new batch
		if currentBatch.size > 0 && currentBatch.size+file.Size > targetSize {
			batches = append(batches, currentBatch)
			currentBatch = batchInfo{
				files: []string{},
				size:  0,
			}
		}

		// Add file to current batch
		currentBatch.files = append(currentBatch.files, file.Path)
		currentBatch.size += file.Size
	}

	// Add final batch if not empty
	if len(currentBatch.files) > 0 {
		batches = append(batches, currentBatch)
	}

	return batches
}

// parseLocation parses a location string (host:path or just path)
func (mc *ManifestCreator) parseLocation(location string) (host, path string, err error) {
	// Check for user@host:path format
	if strings.Contains(location, "@") && strings.Contains(location, ":") {
		// Extract user@host:path
		atIndex := strings.Index(location, "@")
		colonIndex := strings.Index(location, ":")
		if colonIndex > atIndex {
			host = location[atIndex+1 : colonIndex]
			path = location[colonIndex+1:]
			return host, path, nil
		}
	}

	// Check for host:path format (no user)
	if strings.Contains(location, ":") && !strings.HasPrefix(location, "/") {
		parts := strings.SplitN(location, ":", 2)
		if len(parts) == 2 {
			host = parts[0]
			path = parts[1]
			return host, path, nil
		}
	}

	// Local path
	return "", location, nil
}
