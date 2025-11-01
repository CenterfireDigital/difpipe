# DifPipe API Specifications

## Overview

DifPipe provides multiple API interfaces for different use cases:
1. **CLI API** - Command-line interface for interactive and scripted usage
2. **Go Library API** - Native Go package for embedding in applications
3. **REST API** - HTTP-based API for web integrations
4. **gRPC API** - High-performance RPC for service-to-service communication
5. **WebSocket API** - Real-time streaming updates

## CLI API

### Basic Commands

#### Transfer Command

```bash
difpipe transfer [OPTIONS] SOURCE DESTINATION
```

**Options:**
```
--checkpoint-interval SIZE    Save checkpoint every SIZE (e.g., 100MB, 1GB)
--checkpoint-dir PATH         Directory for checkpoint files (default: ~/.difpipe/checkpoints)
--parallel N                  Number of parallel streams (default: 1)
--buffer-size SIZE            Transfer buffer size (default: 32MB)
--compression ALGO            Compression algorithm: none|auto|lz4|zstd|gzip (default: auto)
--compression-level N         Compression level (1-19 for zstd)
--bandwidth-limit RATE        Limit bandwidth (e.g., 10MB/s, 100Mbps)
--retry-max N                 Maximum retry attempts (default: 3)
--retry-backoff DURATION      Initial retry backoff (default: 1s)
--timeout DURATION            Operation timeout (default: 30m)
--verify ALGO                 Verify with checksum: none|sha256|sha512|md5
--progress FORMAT             Progress output: none|simple|json|bar (default: simple)
--log-level LEVEL            Log level: debug|info|warn|error (default: info)
--log-file PATH              Log to file
--metrics-endpoint URL        Prometheus metrics push gateway
--config FILE                Configuration file
--dry-run                    Simulate transfer without executing
```

**Examples:**
```bash
# Basic transfer
difpipe transfer user@host:/source/file.tar.gz /local/dest/

# With resume capability and progress
difpipe transfer \
  --checkpoint-interval 1GB \
  --progress bar \
  user@source:/data/large.dump \
  user@dest:/backup/

# Multi-destination with verification
difpipe transfer \
  --verify sha256 \
  --parallel 4 \
  /local/file.tar \
  s3://bucket/path/ \
  gs://bucket/path/ \
  user@host:/remote/path/
```

#### Stream Command

```bash
difpipe stream [OPTIONS]
```

**Options:**
```
--source TYPE:CONFIG         Source configuration
--destination TYPE:CONFIG    Destination configuration
--transform PIPELINE         Transform pipeline
--mode MODE                  Streaming mode: proxy|fan-out|fan-in
```

**Examples:**
```bash
# TCP proxy with monitoring
difpipe stream \
  --source tcp:localhost:3306 \
  --destination tcp:remote:3306 \
  --monitor

# Kafka to S3 streaming
difpipe stream \
  --source kafka:broker:9092/topic \
  --destination s3://bucket/path/ \
  --transform compress:zstd
```

#### Daemon Command

```bash
difpipe daemon [OPTIONS]
```

**Options:**
```
--config FILE               Daemon configuration file
--api-port PORT            API server port (default: 8080)
--metrics-port PORT        Metrics server port (default: 9090)
--pid-file PATH            PID file location
--workers N                Number of worker threads
```

### Management Commands

```bash
# List transfers
difpipe list [--status STATUS] [--format FORMAT]

# Show transfer details
difpipe show TRANSFER_ID

# Cancel transfer
difpipe cancel TRANSFER_ID

# Resume transfer
difpipe resume TRANSFER_ID

# Verify transfer
difpipe verify SOURCE DESTINATION

# Benchmark
difpipe benchmark [--duration DURATION] [--size SIZE]
```

## Go Library API

### Installation

```bash
go get github.com/larrydiffey/difpipe
```

### Basic Usage

```go
package main

import (
    "context"
    "log"

    "github.com/larrydiffey/difpipe"
    "github.com/larrydiffey/difpipe/pkg/config"
)

func main() {
    // Create client
    client := difpipe.NewClient()

    // Configure transfer
    cfg := &config.TransferConfig{
        Source: config.Source{
            Type: "ssh",
            Path: "user@host:/path/file.tar.gz",
            Auth: config.AuthConfig{
                Method: "key",
                KeyPath: "~/.ssh/id_rsa",
            },
        },
        Destination: config.Destination{
            Type: "s3",
            Path: "s3://bucket/path/",
            Auth: config.AuthConfig{
                Method: "aws-iam",
            },
        },
        Options: config.Options{
            Compression: config.CompressionAuto,
            Parallel: 4,
            CheckpointInterval: 100 * 1024 * 1024, // 100MB
        },
    }

    // Start transfer
    transfer, err := client.Transfer(context.Background(), cfg)
    if err != nil {
        log.Fatal(err)
    }

    // Monitor progress
    for progress := range transfer.Progress() {
        log.Printf("Progress: %.2f%% (%d/%d bytes)",
            progress.Percentage,
            progress.BytesTransferred,
            progress.TotalBytes)
    }

    // Wait for completion
    if err := transfer.Wait(); err != nil {
        log.Fatal(err)
    }
}
```

### Advanced Usage

#### Custom Transform Pipeline

```go
// Create custom transform pipeline
pipeline := difpipe.NewPipeline().
    Compress(difpipe.Zstd(3)).
    Encrypt(difpipe.AES256(key)).
    Filter(func(data []byte) bool {
        return !bytes.Contains(data, []byte("SENSITIVE"))
    })

transfer := difpipe.Transfer{
    Source:    source,
    Dest:      dest,
    Transform: pipeline,
}
```

#### Streaming with Fan-Out

```go
// Stream to multiple destinations
stream := difpipe.NewStream()

// Add source
stream.Source(difpipe.TCPSource{
    Address: "localhost:8080",
})

// Add multiple destinations
stream.AddDestination(difpipe.S3Destination{
    Bucket: "backup-bucket",
    Prefix: "streams/",
})

stream.AddDestination(difpipe.KafkaDestination{
    Brokers: []string{"kafka:9092"},
    Topic:   "events",
})

// Start streaming
ctx := context.Background()
if err := stream.Start(ctx); err != nil {
    log.Fatal(err)
}
```

#### Event Handling

```go
client := difpipe.NewClient()

// Register event handlers
client.OnProgress(func(p difpipe.Progress) {
    fmt.Printf("Transferred: %d MB\n", p.BytesTransferred/1024/1024)
})

client.OnError(func(err error) {
    if difpipe.IsRetryable(err) {
        log.Printf("Retryable error: %v", err)
    } else {
        log.Printf("Fatal error: %v", err)
    }
})

client.OnComplete(func(result difpipe.Result) {
    fmt.Printf("Transfer complete: %s\n", result.TransferID)
    fmt.Printf("Duration: %s\n", result.Duration)
    fmt.Printf("Average speed: %.2f MB/s\n", result.AverageSpeed)
})
```

### Core Interfaces

```go
// Transfer interface
type Transfer interface {
    ID() string
    Start(ctx context.Context) error
    Cancel() error
    Wait() error
    Progress() <-chan Progress
    Status() Status
}

// Client interface
type Client interface {
    Transfer(ctx context.Context, config *TransferConfig) (Transfer, error)
    Stream(ctx context.Context, config *StreamConfig) (Stream, error)
    List(ctx context.Context, filter ListFilter) ([]Transfer, error)
    Get(ctx context.Context, id string) (Transfer, error)
    Cancel(ctx context.Context, id string) error
}

// Stream interface
type Stream interface {
    Source(source Source) Stream
    AddDestination(dest Destination) Stream
    Transform(transform Transform) Stream
    Start(ctx context.Context) error
    Stop() error
    Stats() StreamStats
}

// Progress represents transfer progress
type Progress struct {
    TransferID       string
    BytesTransferred int64
    TotalBytes       int64
    Percentage       float64
    Rate             float64 // bytes per second
    ETA              time.Duration
    Timestamp        time.Time
}

// Source interface
type Source interface {
    Open(ctx context.Context) (io.ReadCloser, error)
    Size() (int64, error)
    Info() SourceInfo
}

// Destination interface
type Destination interface {
    Open(ctx context.Context) (io.WriteCloser, error)
    SupportsResume() bool
    Resume(offset int64) error
    Info() DestinationInfo
}

// Transform interface
type Transform interface {
    Name() string
    Transform(data []byte) ([]byte, error)
    Close() error
}
```

## REST API

### Base URL
```
http://localhost:8080/api/v1
```

### Authentication
```http
Authorization: Bearer <token>
X-API-Key: <api-key>
```

### Endpoints

#### Create Transfer

**POST** `/transfers`

```json
{
  "source": {
    "type": "ssh",
    "path": "user@host:/path/file.tar.gz",
    "auth": {
      "method": "key",
      "key_path": "/home/user/.ssh/id_rsa"
    }
  },
  "destination": {
    "type": "s3",
    "path": "s3://bucket/path/",
    "auth": {
      "method": "aws-iam"
    }
  },
  "options": {
    "compression": "auto",
    "parallel": 4,
    "checkpoint_interval": "100MB",
    "verify": "sha256"
  }
}
```

**Response:**
```json
{
  "id": "transfer-123",
  "status": "in_progress",
  "source": "user@host:/path/file.tar.gz",
  "destination": "s3://bucket/path/",
  "bytes_transferred": 0,
  "total_bytes": 1073741824,
  "created_at": "2024-01-01T00:00:00Z",
  "links": {
    "self": "/api/v1/transfers/transfer-123",
    "progress": "/api/v1/transfers/transfer-123/progress",
    "cancel": "/api/v1/transfers/transfer-123/cancel"
  }
}
```

#### Get Transfer

**GET** `/transfers/{id}`

**Response:**
```json
{
  "id": "transfer-123",
  "status": "completed",
  "source": "user@host:/path/file.tar.gz",
  "destination": "s3://bucket/path/",
  "bytes_transferred": 1073741824,
  "total_bytes": 1073741824,
  "percentage": 100,
  "rate": 52428800,
  "duration": 20.5,
  "checksum": "sha256:abcdef...",
  "created_at": "2024-01-01T00:00:00Z",
  "completed_at": "2024-01-01T00:00:20.5Z"
}
```

#### List Transfers

**GET** `/transfers`

**Query Parameters:**
- `status` - Filter by status (pending, in_progress, completed, failed)
- `limit` - Maximum results (default: 20, max: 100)
- `offset` - Pagination offset
- `sort` - Sort field (created_at, completed_at, size)
- `order` - Sort order (asc, desc)

**Response:**
```json
{
  "transfers": [
    {
      "id": "transfer-123",
      "status": "completed",
      "source": "user@host:/path/file.tar.gz",
      "destination": "s3://bucket/path/",
      "percentage": 100,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ],
  "pagination": {
    "total": 150,
    "limit": 20,
    "offset": 0,
    "next": "/api/v1/transfers?offset=20&limit=20"
  }
}
```

#### Stream Progress (SSE)

**GET** `/transfers/{id}/progress`

```http
GET /api/v1/transfers/transfer-123/progress
Accept: text/event-stream

data: {"bytes_transferred": 10485760, "percentage": 10, "rate": 10485760}
data: {"bytes_transferred": 20971520, "percentage": 20, "rate": 10485760}
...
data: {"bytes_transferred": 1073741824, "percentage": 100, "status": "completed"}
```

#### Cancel Transfer

**POST** `/transfers/{id}/cancel`

**Response:**
```json
{
  "id": "transfer-123",
  "status": "cancelled",
  "message": "Transfer cancelled by user"
}
```

### WebSocket API

**URL:** `ws://localhost:8080/ws`

#### Subscribe to Events

```json
{
  "type": "subscribe",
  "channels": ["transfers", "metrics", "logs"]
}
```

#### Event Messages

```json
{
  "type": "transfer.progress",
  "data": {
    "id": "transfer-123",
    "bytes_transferred": 52428800,
    "percentage": 50,
    "rate": 10485760
  }
}

{
  "type": "transfer.completed",
  "data": {
    "id": "transfer-123",
    "duration": 10.5,
    "checksum": "sha256:abcdef..."
  }
}

{
  "type": "metric",
  "data": {
    "name": "bytes_transferred_total",
    "value": 1073741824,
    "labels": {
      "source": "ssh",
      "destination": "s3"
    }
  }
}
```

## gRPC API

### Proto Definition

```protobuf
syntax = "proto3";
package difpipe.v1;
option go_package = "github.com/larrydiffey/difpipe/api/v1";

service DifPipeService {
  // Transfer operations
  rpc CreateTransfer(CreateTransferRequest) returns (Transfer);
  rpc GetTransfer(GetTransferRequest) returns (Transfer);
  rpc ListTransfers(ListTransfersRequest) returns (ListTransfersResponse);
  rpc CancelTransfer(CancelTransferRequest) returns (Transfer);

  // Streaming operations
  rpc StreamProgress(StreamProgressRequest) returns (stream Progress);
  rpc StreamLogs(StreamLogsRequest) returns (stream LogEntry);

  // Bidirectional streaming
  rpc TransferStream(stream TransferChunk) returns (stream TransferStatus);
}

message Transfer {
  string id = 1;
  string source = 2;
  string destination = 3;
  int64 bytes_transferred = 4;
  int64 total_bytes = 5;
  TransferStatus status = 6;
  google.protobuf.Timestamp created_at = 7;
  google.protobuf.Timestamp completed_at = 8;
  map<string, string> metadata = 9;
}

message Progress {
  string transfer_id = 1;
  int64 bytes_transferred = 2;
  int64 total_bytes = 3;
  double percentage = 4;
  double rate = 5;
  google.protobuf.Duration eta = 6;
}

message TransferChunk {
  string transfer_id = 1;
  bytes data = 2;
  int64 offset = 3;
  bool is_last = 4;
}
```

### Client Example

```go
import (
    pb "github.com/larrydiffey/difpipe/api/v1"
    "google.golang.org/grpc"
)

func main() {
    conn, err := grpc.Dial("localhost:9090", grpc.WithInsecure())
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    client := pb.NewDifPipeServiceClient(conn)

    // Create transfer
    transfer, err := client.CreateTransfer(context.Background(), &pb.CreateTransferRequest{
        Source:      "user@host:/path/file",
        Destination: "s3://bucket/path/",
        Options: &pb.TransferOptions{
            Compression: pb.Compression_AUTO,
            Parallel:    4,
        },
    })

    // Stream progress
    stream, err := client.StreamProgress(context.Background(), &pb.StreamProgressRequest{
        TransferId: transfer.Id,
    })

    for {
        progress, err := stream.Recv()
        if err == io.EOF {
            break
        }
        log.Printf("Progress: %.2f%%", progress.Percentage)
    }
}
```

## Error Codes

### Standard Error Response

```json
{
  "error": {
    "code": "TRANSFER_FAILED",
    "message": "Transfer failed due to network error",
    "details": {
      "transfer_id": "transfer-123",
      "retry_count": 3,
      "last_error": "connection timeout"
    },
    "timestamp": "2024-01-01T00:00:00Z"
  }
}
```

### Error Code Reference

| Code | HTTP Status | Description |
|------|------------|-------------|
| `INVALID_REQUEST` | 400 | Request validation failed |
| `UNAUTHORIZED` | 401 | Authentication required |
| `FORBIDDEN` | 403 | Insufficient permissions |
| `NOT_FOUND` | 404 | Resource not found |
| `CONFLICT` | 409 | Resource conflict |
| `RATE_LIMITED` | 429 | Too many requests |
| `TRANSFER_FAILED` | 500 | Transfer operation failed |
| `CONNECTION_ERROR` | 502 | Connection to source/destination failed |
| `TIMEOUT` | 504 | Operation timeout |
| `INSUFFICIENT_SPACE` | 507 | Insufficient storage |

## Rate Limiting

All API endpoints are rate-limited:

- **Default:** 100 requests per minute
- **Authenticated:** 1000 requests per minute
- **Transfer operations:** 10 concurrent transfers

Rate limit headers:
```http
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 999
X-RateLimit-Reset: 1640995200
```

## Webhooks

### Configuration

```json
{
  "url": "https://example.com/webhook",
  "events": ["transfer.completed", "transfer.failed"],
  "secret": "webhook-secret",
  "retry": {
    "enabled": true,
    "max_attempts": 3,
    "backoff": "exponential"
  }
}
```

### Event Payload

```json
{
  "event": "transfer.completed",
  "timestamp": "2024-01-01T00:00:00Z",
  "data": {
    "transfer_id": "transfer-123",
    "source": "user@host:/path/file",
    "destination": "s3://bucket/path/",
    "bytes_transferred": 1073741824,
    "duration": 20.5,
    "checksum": "sha256:abcdef..."
  }
}
```

### Signature Verification

```go
func verifyWebhookSignature(payload []byte, signature string, secret string) bool {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write(payload)
    expected := hex.EncodeToString(mac.Sum(nil))
    return hmac.Equal([]byte(signature), []byte(expected))
}
```

## SDK Support

### Official SDKs

- **Go**: Native implementation
- **Python**: `pip install difpipe`
- **JavaScript/Node**: `npm install @difpipe/client`
- **Rust**: `cargo add difpipe`
- **Java**: Maven/Gradle support

### Python Example

```python
from difpipe import Client

client = Client(api_key="your-api-key")

# Create transfer
transfer = client.transfer(
    source="user@host:/path/file",
    destination="s3://bucket/path/",
    compression="auto",
    parallel=4
)

# Monitor progress
for progress in transfer.progress():
    print(f"Progress: {progress.percentage:.1f}%")

# Wait for completion
result = transfer.wait()
print(f"Transfer completed in {result.duration} seconds")
```

### JavaScript Example

```javascript
const { DifPipe } = require('@difpipe/client');

const client = new DifPipe({ apiKey: 'your-api-key' });

// Create transfer
const transfer = await client.transfer({
  source: 'user@host:/path/file',
  destination: 's3://bucket/path/',
  options: {
    compression: 'auto',
    parallel: 4
  }
});

// Monitor progress
transfer.on('progress', (progress) => {
  console.log(`Progress: ${progress.percentage}%`);
});

// Wait for completion
await transfer.wait();
console.log('Transfer completed');
```

## API Versioning

The API follows semantic versioning:

- **Current stable:** v1
- **Beta:** v2-beta
- **Deprecation policy:** 6 months notice

Version selection:
```http
# Via URL path
GET /api/v1/transfers

# Via header
GET /api/transfers
API-Version: v1

# Via query parameter
GET /api/transfers?version=v1
```

This comprehensive API specification provides multiple interfaces for different use cases, ensuring DifPipe can be integrated into any environment or workflow.