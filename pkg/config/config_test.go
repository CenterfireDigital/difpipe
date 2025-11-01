package config

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestLoadConfig_JSON(t *testing.T) {
	jsonConfig := `{
		"transfer": {
			"source": {"path": "/source"},
			"destination": {"path": "/dest"},
			"options": {
				"strategy": "rclone",
				"parallel": 8
			}
		},
		"output": {
			"format": "json"
		}
	}`

	cfg, err := ParseAuto([]byte(jsonConfig))
	if err != nil {
		t.Fatalf("Failed to parse JSON config: %v", err)
	}

	if cfg.Transfer.Source.Path != "/source" {
		t.Errorf("Expected source path /source, got %s", cfg.Transfer.Source.Path)
	}

	if cfg.Transfer.Destination.Path != "/dest" {
		t.Errorf("Expected dest path /dest, got %s", cfg.Transfer.Destination.Path)
	}

	if cfg.Transfer.Options.Strategy != "rclone" {
		t.Errorf("Expected strategy rclone, got %s", cfg.Transfer.Options.Strategy)
	}

	if cfg.Transfer.Options.Parallel != 8 {
		t.Errorf("Expected parallel 8, got %d", cfg.Transfer.Options.Parallel)
	}
}

func TestLoadConfig_YAML(t *testing.T) {
	yamlConfig := `
transfer:
  source:
    path: /source
  destination:
    path: /dest
  options:
    strategy: rsync
    parallel: 4
output:
  format: text
`

	cfg, err := ParseAuto([]byte(yamlConfig))
	if err != nil {
		t.Fatalf("Failed to parse YAML config: %v", err)
	}

	if cfg.Transfer.Source.Path != "/source" {
		t.Errorf("Expected source path /source, got %s", cfg.Transfer.Source.Path)
	}

	if cfg.Transfer.Options.Strategy != "rsync" {
		t.Errorf("Expected strategy rsync, got %s", cfg.Transfer.Options.Strategy)
	}
}

func TestFromEnv(t *testing.T) {
	// Set environment variables
	os.Setenv("DIFPIPE_SOURCE", "/env/source")
	os.Setenv("DIFPIPE_DEST", "/env/dest")
	os.Setenv("DIFPIPE_STRATEGY", "tar")
	os.Setenv("DIFPIPE_PARALLEL", "16")
	defer func() {
		os.Unsetenv("DIFPIPE_SOURCE")
		os.Unsetenv("DIFPIPE_DEST")
		os.Unsetenv("DIFPIPE_STRATEGY")
		os.Unsetenv("DIFPIPE_PARALLEL")
	}()

	cfg := FromEnv()

	if cfg.Transfer.Source.Path != "/env/source" {
		t.Errorf("Expected source from env, got %s", cfg.Transfer.Source.Path)
	}

	if cfg.Transfer.Destination.Path != "/env/dest" {
		t.Errorf("Expected dest from env, got %s", cfg.Transfer.Destination.Path)
	}

	if cfg.Transfer.Options.Strategy != "tar" {
		t.Errorf("Expected strategy tar, got %s", cfg.Transfer.Options.Strategy)
	}

	if cfg.Transfer.Options.Parallel != 16 {
		t.Errorf("Expected parallel 16, got %d", cfg.Transfer.Options.Parallel)
	}
}

func TestMerge(t *testing.T) {
	cfg1 := &Config{
		Transfer: TransferConfig{
			Source: SourceConfig{Path: "/source1"},
			Options: TransferOptions{
				Strategy: "rclone",
				Parallel: 4,
			},
		},
	}

	cfg2 := &Config{
		Transfer: TransferConfig{
			Destination: DestinationConfig{Path: "/dest2"},
			Options: TransferOptions{
				Parallel: 8,
			},
		},
	}

	// cfg1 has higher priority
	merged := Merge(cfg1, cfg2)

	if merged.Transfer.Source.Path != "/source1" {
		t.Errorf("Expected source from cfg1, got %s", merged.Transfer.Source.Path)
	}

	if merged.Transfer.Destination.Path != "/dest2" {
		t.Errorf("Expected dest from cfg2, got %s", merged.Transfer.Destination.Path)
	}

	if merged.Transfer.Options.Strategy != "rclone" {
		t.Errorf("Expected strategy from cfg1, got %s", merged.Transfer.Options.Strategy)
	}

	if merged.Transfer.Options.Parallel != 4 {
		t.Errorf("Expected parallel from cfg1, got %d", merged.Transfer.Options.Parallel)
	}
}

func TestLoadConfig_Stdin(t *testing.T) {
	jsonConfig := `{"transfer":{"source":{"path":"/src"},"destination":{"path":"/dst"}}}`

	// This tests the detection logic, not actual stdin reading
	if strings.HasPrefix(jsonConfig, "{") {
		var cfg Config
		err := json.Unmarshal([]byte(jsonConfig), &cfg)
		if err != nil {
			t.Fatalf("Failed to unmarshal JSON: %v", err)
		}

		if cfg.Transfer.Source.Path != "/src" {
			t.Errorf("Expected /src, got %s", cfg.Transfer.Source.Path)
		}
	}
}

func TestParseAuto_Empty(t *testing.T) {
	_, err := ParseAuto([]byte(""))
	if err == nil {
		t.Error("Expected error for empty config, got nil")
	}
}

func TestParseAuto_Invalid(t *testing.T) {
	_, err := ParseAuto([]byte("not valid json or yaml"))
	if err == nil {
		t.Error("Expected error for invalid config, got nil")
	}
}

func TestLoadConfig_WithAuth(t *testing.T) {
	yamlConfig := `
transfer:
  source:
    path: "root@server1:~/file"
    auth:
      password: "test-password-123!@#"
  destination:
    path: "root@server2:~/file"
    auth:
      password: "#special!chars)}"
  options:
    strategy: proxy
`

	cfg, err := ParseAuto([]byte(yamlConfig))
	if err != nil {
		t.Fatalf("Failed to parse YAML config with auth: %v", err)
	}

	if cfg.Transfer.Source.Auth == nil {
		t.Error("Expected source auth to be populated, got nil")
	}

	if pwd, ok := cfg.Transfer.Source.Auth["password"].(string); !ok || pwd != "test-password-123!@#" {
		t.Errorf("Expected source password 'test-password-123!@#', got %v", cfg.Transfer.Source.Auth["password"])
	}

	if cfg.Transfer.Destination.Auth == nil {
		t.Error("Expected destination auth to be populated, got nil")
	}

	if pwd, ok := cfg.Transfer.Destination.Auth["password"].(string); !ok || pwd != "#special!chars)}" {
		t.Errorf("Expected dest password '#special!chars)}', got %v", cfg.Transfer.Destination.Auth["password"])
	}
}

func TestMerge_WithAuth(t *testing.T) {
	cfg1 := &Config{
		Transfer: TransferConfig{
			Source: SourceConfig{
				Path: "user@server:~/file",
				Auth: map[string]interface{}{
					"password": "secret123",
				},
			},
			Destination: DestinationConfig{
				Path: "user@dest:~/file",
				Auth: map[string]interface{}{
					"password": "dest-secret",
				},
			},
		},
	}

	cfg2 := &Config{
		Transfer: TransferConfig{
			Options: TransferOptions{
				Strategy: "proxy",
			},
		},
	}

	// cfg1 has higher priority and should preserve auth
	merged := Merge(cfg1, cfg2)

	if merged.Transfer.Source.Auth == nil {
		t.Fatal("Source auth was lost during merge")
	}

	if pwd, ok := merged.Transfer.Source.Auth["password"].(string); !ok || pwd != "secret123" {
		t.Errorf("Expected source password 'secret123', got %v", merged.Transfer.Source.Auth["password"])
	}

	if merged.Transfer.Destination.Auth == nil {
		t.Fatal("Destination auth was lost during merge")
	}

	if pwd, ok := merged.Transfer.Destination.Auth["password"].(string); !ok || pwd != "dest-secret" {
		t.Errorf("Expected dest password 'dest-secret', got %v", merged.Transfer.Destination.Auth["password"])
	}
}
