package transport

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// AuthMethod represents an SSH authentication method
type AuthMethod interface {
	// SSHAuthMethods returns the SSH auth methods for this authentication
	SSHAuthMethods() ([]ssh.AuthMethod, error)

	// String returns a string representation (without sensitive data)
	String() string
}

// PasswordAuth implements password-based authentication
type PasswordAuth struct {
	Password string
}

// NewPasswordAuth creates a new password authentication method
func NewPasswordAuth(password string) *PasswordAuth {
	return &PasswordAuth{Password: password}
}

// SSHAuthMethods returns the SSH auth methods
func (a *PasswordAuth) SSHAuthMethods() ([]ssh.AuthMethod, error) {
	return []ssh.AuthMethod{
		ssh.Password(a.Password),
	}, nil
}

// String returns a string representation
func (a *PasswordAuth) String() string {
	return "password"
}

// KeyAuth implements public key authentication
type KeyAuth struct {
	KeyPath    string
	Passphrase string
}

// NewKeyAuth creates a new key authentication method
func NewKeyAuth(keyPath, passphrase string) *KeyAuth {
	return &KeyAuth{
		KeyPath:    keyPath,
		Passphrase: passphrase,
	}
}

// SSHAuthMethods returns the SSH auth methods
func (a *KeyAuth) SSHAuthMethods() ([]ssh.AuthMethod, error) {
	// Read private key file
	keyData, err := os.ReadFile(a.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("read key file: %w", err)
	}

	// Parse private key
	var signer ssh.Signer
	if a.Passphrase != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(keyData, []byte(a.Passphrase))
	} else {
		signer, err = ssh.ParsePrivateKey(keyData)
	}

	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	return []ssh.AuthMethod{
		ssh.PublicKeys(signer),
	}, nil
}

// String returns a string representation
func (a *KeyAuth) String() string {
	return fmt.Sprintf("key(%s)", a.KeyPath)
}

// AgentAuth implements SSH agent authentication
type AgentAuth struct {
	socketPath string
}

// NewAgentAuth creates a new agent authentication method
func NewAgentAuth() *AgentAuth {
	return &AgentAuth{
		socketPath: os.Getenv("SSH_AUTH_SOCK"),
	}
}

// SSHAuthMethods returns the SSH auth methods
func (a *AgentAuth) SSHAuthMethods() ([]ssh.AuthMethod, error) {
	if a.socketPath == "" {
		return nil, fmt.Errorf("SSH_AUTH_SOCK not set")
	}

	// Connect to SSH agent
	conn, err := net.Dial("unix", a.socketPath)
	if err != nil {
		return nil, fmt.Errorf("connect to SSH agent: %w", err)
	}

	agentClient := agent.NewClient(conn)

	return []ssh.AuthMethod{
		ssh.PublicKeysCallback(agentClient.Signers),
	}, nil
}

// String returns a string representation
func (a *AgentAuth) String() string {
	return "agent"
}

// MultiAuth combines multiple authentication methods
type MultiAuth struct {
	methods []AuthMethod
}

// NewMultiAuth creates a new multi-auth method
func NewMultiAuth(methods ...AuthMethod) *MultiAuth {
	return &MultiAuth{methods: methods}
}

// SSHAuthMethods returns all SSH auth methods
func (a *MultiAuth) SSHAuthMethods() ([]ssh.AuthMethod, error) {
	var allMethods []ssh.AuthMethod

	for _, method := range a.methods {
		methods, err := method.SSHAuthMethods()
		if err != nil {
			// Skip methods that fail, try others
			continue
		}
		allMethods = append(allMethods, methods...)
	}

	if len(allMethods) == 0 {
		return nil, fmt.Errorf("no valid authentication methods available")
	}

	return allMethods, nil
}

// String returns a string representation
func (a *MultiAuth) String() string {
	return fmt.Sprintf("multi(%d methods)", len(a.methods))
}

// AuthFromConfig creates an AuthMethod from configuration
func AuthFromConfig(authConfig map[string]interface{}) (AuthMethod, error) {
	if authConfig == nil {
		return nil, fmt.Errorf("auth config is nil")
	}

	// Check for password
	if password, ok := authConfig["password"].(string); ok && password != "" {
		return NewPasswordAuth(password), nil
	}

	// Check for key file
	if keyPath, ok := authConfig["key"].(string); ok && keyPath != "" {
		passphrase, _ := authConfig["passphrase"].(string)
		return NewKeyAuth(keyPath, passphrase), nil
	}

	// Check for agent
	if useAgent, ok := authConfig["agent"].(bool); ok && useAgent {
		return NewAgentAuth(), nil
	}

	// Try multiple methods
	var methods []AuthMethod

	// Try agent first
	if agent := NewAgentAuth(); agent.socketPath != "" {
		methods = append(methods, agent)
	}

	// Try default key locations
	homeDir, err := os.UserHomeDir()
	if err == nil {
		defaultKeys := []string{
			homeDir + "/.ssh/id_rsa",
			homeDir + "/.ssh/id_ed25519",
			homeDir + "/.ssh/id_ecdsa",
		}

		for _, keyPath := range defaultKeys {
			if _, err := os.Stat(keyPath); err == nil {
				methods = append(methods, NewKeyAuth(keyPath, ""))
			}
		}
	}

	if len(methods) > 0 {
		return NewMultiAuth(methods...), nil
	}

	return nil, fmt.Errorf("no authentication method configured")
}
