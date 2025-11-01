package transport

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHClient wraps an SSH client with additional management features
type SSHClient struct {
	config        *SSHConfig
	client        *ssh.Client
	keepaliveStop chan struct{}
	keepaliveDone chan struct{}
	mutex         sync.Mutex
}

// SSHConfig contains SSH connection configuration
type SSHConfig struct {
	Host              string
	Port              int
	User              string
	Auth              AuthMethod
	Timeout           time.Duration
	Keepalive         time.Duration
	HostKeyCallback   ssh.HostKeyCallback
}

// Validate checks if the configuration is valid
func (c *SSHConfig) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("host is required")
	}

	if c.Port == 0 {
		c.Port = 22 // Default SSH port
	}

	if c.User == "" {
		return fmt.Errorf("user is required")
	}

	if c.Auth == nil {
		return fmt.Errorf("auth method is required")
	}

	if c.Timeout == 0 {
		c.Timeout = 30 * time.Second // Default timeout
	}

	if c.HostKeyCallback == nil {
		// Use insecure callback for MVP
		// TODO: Replace with proper host key verification
		c.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	}

	return nil
}

// Client returns the underlying SSH client
func (c *SSHClient) Client() *ssh.Client {
	return c.client
}

// Config returns the SSH configuration
func (c *SSHClient) Config() *SSHConfig {
	return c.config
}

// IsConnected checks if the client is connected
func (c *SSHClient) IsConnected() bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.client == nil {
		return false
	}

	// Try to create a session to verify connection
	session, err := c.client.NewSession()
	if err != nil {
		return false
	}
	session.Close()

	return true
}

// startKeepalive starts sending keepalive packets
func (c *SSHClient) startKeepalive() {
	c.keepaliveStop = make(chan struct{})
	c.keepaliveDone = make(chan struct{})

	go func() {
		defer close(c.keepaliveDone)

		ticker := time.NewTicker(c.config.Keepalive)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Send keepalive request
				_, _, err := c.client.SendRequest("keepalive@openssh.com", true, nil)
				if err != nil {
					// Connection lost, stop keepalive
					return
				}
			case <-c.keepaliveStop:
				return
			}
		}
	}()
}

// stopKeepalive stops sending keepalive packets
func (c *SSHClient) stopKeepalive() {
	if c.keepaliveStop != nil {
		close(c.keepaliveStop)
		<-c.keepaliveDone
	}
}

// ParseRemotePath parses a remote path in the format user@host:path
func ParseRemotePath(remotePath string) (*RemoteLocation, error) {
	// Expected format: user@host:path or user@host:/path
	var location RemoteLocation

	// Find @ separator
	atIndex := -1
	for i, c := range remotePath {
		if c == '@' {
			atIndex = i
			break
		}
	}

	if atIndex == -1 {
		return nil, fmt.Errorf("invalid remote path format: expected user@host:path, got %s", remotePath)
	}

	location.User = remotePath[:atIndex]

	// Find : separator
	colonIndex := -1
	for i := atIndex + 1; i < len(remotePath); i++ {
		if remotePath[i] == ':' {
			colonIndex = i
			break
		}
	}

	if colonIndex == -1 {
		return nil, fmt.Errorf("invalid remote path format: expected user@host:path, got %s", remotePath)
	}

	location.Host = remotePath[atIndex+1 : colonIndex]
	location.Path = remotePath[colonIndex+1:]
	location.Port = 22 // Default SSH port

	if location.User == "" {
		return nil, fmt.Errorf("user is empty")
	}

	if location.Host == "" {
		return nil, fmt.Errorf("host is empty")
	}

	if location.Path == "" {
		return nil, fmt.Errorf("path is empty")
	}

	return &location, nil
}

// RemoteLocation represents a parsed remote location
type RemoteLocation struct {
	User string
	Host string
	Port int
	Path string
}

// String returns a string representation of the remote location
func (r *RemoteLocation) String() string {
	return fmt.Sprintf("%s@%s:%s", r.User, r.Host, r.Path)
}

// SSHConfig returns an SSHConfig for this location
func (r *RemoteLocation) SSHConfig(auth AuthMethod) *SSHConfig {
	return &SSHConfig{
		Host:      r.Host,
		Port:      r.Port,
		User:      r.User,
		Auth:      auth,
		Timeout:   30 * time.Second,
		Keepalive: 30 * time.Second,
	}
}

// GetFileSize gets the size of a remote file using stat
func GetFileSize(ctx context.Context, transport Transport, client *SSHClient, path string) (int64, error) {
	cmd := fmt.Sprintf("stat -c%%s %s 2>/dev/null || stat -f%%z %s", path, path)

	result, err := transport.ExecuteCommand(ctx, client, cmd)
	if err != nil {
		return 0, fmt.Errorf("execute stat command: %w", err)
	}

	if result.ExitCode != 0 {
		return 0, fmt.Errorf("stat failed: %s", string(result.Stderr))
	}

	var size int64
	_, err = fmt.Sscanf(string(result.Stdout), "%d", &size)
	if err != nil {
		return 0, fmt.Errorf("parse size: %w", err)
	}

	return size, nil
}
