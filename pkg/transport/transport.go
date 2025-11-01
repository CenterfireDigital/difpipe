package transport

import (
	"context"
	"fmt"
	"io"

	"golang.org/x/crypto/ssh"
)

// Transport manages SSH connections and command execution
type Transport interface {
	// Connect establishes an SSH connection
	Connect(ctx context.Context, config *SSHConfig) (*SSHClient, error)

	// ExecuteCommand executes a command and returns the result
	ExecuteCommand(ctx context.Context, client *SSHClient, cmd string) (*CommandResult, error)

	// StreamCommand executes a command and returns a stream for reading output
	StreamCommand(ctx context.Context, client *SSHClient, cmd string) (io.ReadCloser, error)

	// StreamWrite executes a command and returns a stream for writing input
	StreamWrite(ctx context.Context, client *SSHClient, cmd string) (io.WriteCloser, error)

	// Close closes an SSH connection
	Close(client *SSHClient) error
}

// SSHTransport implements Transport using SSH
type SSHTransport struct{}

// New creates a new SSH transport
func New() Transport {
	return &SSHTransport{}
}

// Connect establishes an SSH connection
func (t *SSHTransport) Connect(ctx context.Context, config *SSHConfig) (*SSHClient, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Build SSH client config
	authMethods, err := config.Auth.SSHAuthMethods()
	if err != nil {
		return nil, fmt.Errorf("build auth methods: %w", err)
	}

	sshConfig := &ssh.ClientConfig{
		User: config.User,
		Auth: authMethods,
		HostKeyCallback: config.HostKeyCallback,
		Timeout:         config.Timeout,
	}

	// Connect with timeout
	address := fmt.Sprintf("%s:%d", config.Host, config.Port)

	// Use context deadline if provided
	var client *ssh.Client
	done := make(chan error, 1)

	go func() {
		var err error
		client, err = ssh.Dial("tcp", address, sshConfig)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			return nil, fmt.Errorf("connect to %s: %w", address, err)
		}
	case <-ctx.Done():
		return nil, fmt.Errorf("connection timeout: %w", ctx.Err())
	}

	sshClient := &SSHClient{
		config: config,
		client: client,
	}

	// Start keepalive if configured
	if config.Keepalive > 0 {
		sshClient.startKeepalive()
	}

	return sshClient, nil
}

// ExecuteCommand executes a command and returns the result
func (t *SSHTransport) ExecuteCommand(ctx context.Context, client *SSHClient, cmd string) (*CommandResult, error) {
	session, err := client.client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("new session: %w", err)
	}
	defer session.Close()

	result := &CommandResult{}

	// Capture stdout and stderr
	stdout, err := session.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	stderr, err := session.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	// Start command
	if err := session.Start(cmd); err != nil {
		return nil, fmt.Errorf("start command: %w", err)
	}

	// Read output in goroutines
	stdoutDone := make(chan error, 1)
	stderrDone := make(chan error, 1)

	go func() {
		data, err := io.ReadAll(stdout)
		if err != nil {
			stdoutDone <- err
			return
		}
		result.Stdout = data
		stdoutDone <- nil
	}()

	go func() {
		data, err := io.ReadAll(stderr)
		if err != nil {
			stderrDone <- err
			return
		}
		result.Stderr = data
		stderrDone <- nil
	}()

	// Wait for command to complete or context cancel
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- session.Wait()
	}()

	select {
	case err := <-waitDone:
		// Wait for output to be read
		<-stdoutDone
		<-stderrDone

		if err != nil {
			if exitErr, ok := err.(*ssh.ExitError); ok {
				result.ExitCode = exitErr.ExitStatus()
			} else {
				return nil, fmt.Errorf("wait for command: %w", err)
			}
		}
	case <-ctx.Done():
		session.Signal(ssh.SIGKILL)
		return nil, fmt.Errorf("command timeout: %w", ctx.Err())
	}

	return result, nil
}

// StreamCommand executes a command and returns a stream for reading output
func (t *SSHTransport) StreamCommand(ctx context.Context, client *SSHClient, cmd string) (io.ReadCloser, error) {
	session, err := client.client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("new session: %w", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	// Get stderr pipe to drain it
	stderr, err := session.StderrPipe()
	if err != nil {
		session.Close()
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := session.Start(cmd); err != nil {
		session.Close()
		return nil, fmt.Errorf("start command: %w", err)
	}

	// Drain stderr in background so session can complete
	go func() {
		io.Copy(io.Discard, stderr)
	}()

	// Wrap stdout with session cleanup
	return &streamReader{
		reader:  stdout,
		session: session,
	}, nil
}

// StreamWrite executes a command and returns a stream for writing input
func (t *SSHTransport) StreamWrite(ctx context.Context, client *SSHClient, cmd string) (io.WriteCloser, error) {
	session, err := client.client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("new session: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	// Get stdout and stderr pipes to drain them
	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	stderr, err := session.StderrPipe()
	if err != nil {
		session.Close()
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := session.Start(cmd); err != nil {
		session.Close()
		return nil, fmt.Errorf("start command: %w", err)
	}

	// Drain stdout and stderr in background so session can complete
	go func() {
		io.Copy(io.Discard, stdout)
	}()
	go func() {
		io.Copy(io.Discard, stderr)
	}()

	// Wrap stdin with session cleanup
	return &streamWriter{
		writer:  stdin,
		session: session,
	}, nil
}

// Close closes an SSH connection
func (t *SSHTransport) Close(client *SSHClient) error {
	if client == nil {
		return nil
	}

	client.stopKeepalive()

	if client.client != nil {
		return client.client.Close()
	}

	return nil
}

// streamReader wraps an io.Reader with session cleanup
type streamReader struct {
	reader  io.Reader
	session *ssh.Session
}

func (s *streamReader) Read(p []byte) (n int, err error) {
	return s.reader.Read(p)
}

func (s *streamReader) Close() error {
	// Wait for session to complete
	if s.session != nil {
		s.session.Wait()
		return s.session.Close()
	}
	return nil
}

// streamWriter wraps an io.Writer with session cleanup
type streamWriter struct {
	writer  io.WriteCloser
	session *ssh.Session
}

func (s *streamWriter) Write(p []byte) (n int, err error) {
	return s.writer.Write(p)
}

func (s *streamWriter) Close() error {
	// Close stdin first
	if err := s.writer.Close(); err != nil {
		return err
	}

	// Wait for session to complete
	if s.session != nil {
		s.session.Wait()
		return s.session.Close()
	}

	return nil
}

// CommandResult contains the result of a command execution
type CommandResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}
