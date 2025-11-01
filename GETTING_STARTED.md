# Getting Started - DifPipe Development

## Quick Start

```bash
cd ~/Projects/DifPipe

# Initialize Go module
go mod init github.com/larrydiffey/difpipe

# Install dependencies
go get github.com/rclone/rclone
go get github.com/spf13/cobra
go get github.com/spf13/viper
go get github.com/sirupsen/logrus
go get gopkg.in/yaml.v3

# Create project structure
mkdir -p cmd/difpipe
mkdir -p pkg/{config,core,analyzer,orchestrator,output}
mkdir -p pkg/engines/{rclone,rsync,tarstream}

# Build
go build -o difpipe ./cmd/difpipe

# Test
./difpipe --version
```

## Development Order for v0.1.0

### Phase 1: Foundation (Day 1)
1. **Initialize project structure**
   - `go.mod` and dependencies
   - Create package directories

2. **Config system** (`pkg/config/`)
   - Load from JSON/YAML/stdin
   - Environment variables
   - Auto-detection

3. **Output formatters** (`pkg/output/`)
   - JSON, YAML, text formatters
   - Progress streaming

### Phase 2: Core Logic (Days 2-3)
4. **Core interfaces** (`pkg/core/`)
   - FileAnalysis struct
   - TransferEngine interface
   - Strategy types

5. **File analyzer** (`pkg/analyzer/`)
   - Analyze local files
   - Recommend strategy
   - Sample-based detection

6. **Rclone wrapper** (`pkg/engines/rclone/`)
   - Execute rclone commands
   - Parse rclone output
   - Progress monitoring

### Phase 3: CLI & Integration (Days 4-5)
7. **Orchestrator** (`pkg/orchestrator/`)
   - Strategy selection
   - Engine coordination
   - Progress tracking

8. **CLI** (`cmd/difpipe/`)
   - Transfer command
   - Analyze command
   - Status command
   - Schema command

### Phase 4: Testing & Polish (Days 6-7)
9. **Tests**
   - Unit tests for each package
   - Integration tests
   - End-to-end scenarios

10. **Documentation**
    - Code comments
    - Usage examples
    - Release notes

## File Structure

```
DifPipe/
├── cmd/
│   └── difpipe/
│       └── main.go              # CLI entry point
├── pkg/
│   ├── config/
│   │   ├── config.go            # Config structures
│   │   ├── loader.go            # Load from file/stdin/env
│   │   └── config_test.go
│   ├── core/
│   │   ├── interfaces.go        # Core interfaces
│   │   ├── types.go             # Common types
│   │   └── exit_codes.go        # Semantic exit codes
│   ├── output/
│   │   ├── formatter.go         # Output formatting
│   │   └── stream.go            # Progress streaming
│   ├── analyzer/
│   │   ├── analyzer.go          # File analysis
│   │   └── strategy.go          # Strategy recommendation
│   ├── orchestrator/
│   │   ├── orchestrator.go      # Main orchestration
│   │   └── selector.go          # Strategy selection
│   └── engines/
│       ├── rclone/
│       │   ├── engine.go        # Rclone wrapper
│       │   ├── command.go       # Command building
│       │   └── progress.go      # Progress parsing
│       ├── rsync/
│       │   └── engine.go
│       └── tarstream/
│           └── engine.go
├── test/
│   ├── integration/
│   └── fixtures/
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## Key Implementation Patterns

### 1. Config Loading Priority
```go
// Priority order (highest to lowest):
1. Command-line flags
2. Config file (--config)
3. Stdin pipe (--config -)
4. Environment variables
5. Defaults
```

### 2. Output Format Selection
```go
formatter := output.New(output.Format(outputFormat), os.Stdout)
formatter.Format(result) // Automatically outputs JSON/YAML/text
```

### 3. Strategy Selection
```go
analysis := analyzer.Analyze(source)
strategy := analysis.Recommendation // auto, rclone, rsync, tar
engine := engines[strategy]
engine.Transfer(opts)
```

### 4. Progress Streaming
```go
if progressStream {
    for event := range engine.Progress() {
        json.NewEncoder(os.Stdout).Encode(event)
    }
}
```

## Testing Strategy

```bash
# Unit tests
go test ./pkg/...

# Integration tests
go test ./test/integration/...

# Coverage
go test -cover ./...

# Race detection
go test -race ./...
```

## Example Usage (What We're Building)

```bash
# Analyze files
difpipe analyze /data s3://backup --output json

# Transfer with auto-detection
difpipe transfer /data s3://backup

# Transfer with config file
difpipe transfer --config transfer.yaml

# Transfer with stdin config
echo '{"source":"/data","dest":"s3://backup"}' | \
  difpipe transfer --config - --output json

# Get status
difpipe status transfer-abc123 --output json
```

## MCP Integration Target

```typescript
// This is what we're enabling
const difpipe = {
  analyze: (source, dest) =>
    exec(`difpipe analyze ${source} ${dest} --output json`),

  transfer: (source, dest) =>
    exec(`difpipe transfer ${source} ${dest} --output json`),

  status: (id) =>
    exec(`difpipe status ${id} --output json`)
};
```

## Dependencies

- **rclone**: Transfer engine (70+ backends)
- **cobra**: CLI framework
- **viper**: Configuration management
- **logrus**: Structured logging
- **yaml.v3**: YAML parsing

## Build Commands

```bash
# Development build
go build -o difpipe ./cmd/difpipe

# Production build
go build -ldflags="-s -w" -o difpipe ./cmd/difpipe

# Cross-compile
GOOS=linux GOARCH=amd64 go build -o difpipe-linux ./cmd/difpipe

# Run without building
go run ./cmd/difpipe transfer /source /dest
```

## Development Tips

1. **Start with config**: Get JSON/YAML loading working first
2. **Mock rclone initially**: Don't need actual transfers to test logic
3. **Test output formats early**: Ensure JSON/YAML work correctly
4. **Use table-driven tests**: Easier to add test cases
5. **Log liberally**: Use structured logging everywhere

## Ready to Code!

Now let's start implementing. Begin with:

```bash
cd ~/Projects/DifPipe
git init
git add .
git commit -m "Initial project structure"
```

Then start coding in this order:
1. `pkg/config/config.go` - Core config structures
2. `pkg/config/loader.go` - Config loading
3. `pkg/output/formatter.go` - Output formatting
4. `pkg/core/interfaces.go` - Core types
5. `cmd/difpipe/main.go` - CLI skeleton

Let's build this! 🚀