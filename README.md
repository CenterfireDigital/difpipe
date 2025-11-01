# DifPipe

[![Go](https://img.shields.io/badge/go-1.25+-00ADD8.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/license-Apache%202.0-green.svg)](LICENSE)
[![Status](https://img.shields.io/badge/status-alpha-yellow.svg)](https://github.com/CenterfireDigital/difpipe)

**Intelligent data transfer tool that automatically selects the optimal strategy for your workload.**

> **⚠️ Development Status**: DifPipe is in active development (alpha). Core engines work but not all features are fully tested. Use with caution in production environments.

## What is DifPipe?

DifPipe analyzes your data and automatically picks the best transfer method:
- **Rclone** for cloud storage (S3, GCS, Azure, etc.)
- **Rsync** for large files with delta sync
- **Tar streaming** for millions of small files
- **Proxy mode** for remote-to-remote transfers

No more guessing which tool to use—DifPipe does it for you.

## Current Features

✅ **Rclone Engine** - 70+ storage backends (S3, GCS, Azure, Dropbox, SFTP, etc.)
✅ **Rsync Engine** - Delta transfers for large files
✅ **Tar Streaming** - Efficient bundling of small files
✅ **Proxy Engine** - Remote-to-remote transfers without local storage
✅ **Batched Tar** - Parallel processing with buffering (tested: 307k files in 6m16s)
✅ **Smart Analysis** - Automatic strategy selection based on file patterns
✅ **Checkpointing** - Resume interrupted transfers
✅ **SSH Support** - Password and key-based authentication

## Installation

### From Source (Recommended)

```bash
git clone https://github.com/CenterfireDigital/difpipe.git
cd difpipe
go build -o difpipe ./cmd/difpipe
sudo mv difpipe /usr/local/bin/

# Verify installation
difpipe --version
```

### Using Go Install

```bash
go install github.com/CenterfireDigital/difpipe/cmd/difpipe@latest
```

## Quick Start

### Basic Usage

```bash
# Analyze files and get strategy recommendation
difpipe analyze /data/source

# Auto-detect and transfer
difpipe transfer /data/source /data/dest

# Force specific strategy
difpipe transfer /data/source /backup --strategy tar
difpipe transfer /data/source /backup --strategy rsync
difpipe transfer /data/source s3://bucket --strategy rclone
```

### Strategy Selection

DifPipe automatically selects the optimal engine:

| Files | Pattern | Strategy | Why |
|-------|---------|----------|-----|
| 1-10 files | Any | **Rsync** | Simple, efficient |
| 1000+ files | >80% small (<10KB) | **Tar** | Bundle into streams |
| Mixed | >50% large (>100MB) | **Rsync** | Optimized for large files |
| Any | Cloud destination | **Rclone** | Native cloud support |
| Remote→Remote | Via SSH | **Proxy** | Direct streaming |

## Use Cases

### 1. Remote-to-Remote Transfers (Proxy Mode)

Transfer files between remote servers without downloading locally:

```bash
# Using environment variables for authentication
export DIFPIPE_SOURCE_PASSWORD='source-password'
export DIFPIPE_DEST_PASSWORD='dest-password'

difpipe transfer \
  root@server1.example.com:~/backup.tar.gz \
  root@server2.example.com:~/backup.tar.gz \
  --strategy proxy

# Real-world example: 5GB file in 1m49s @ 46.9 MB/s
```

**Benefits:**
- No local disk space required
- Servers don't need direct connectivity
- Memory-efficient streaming
- Progress tracking

### 2. Many Small Files (Batched Tar)

Efficiently transfer millions of small files:

```bash
# DifPipe auto-detects and uses batched tar
difpipe transfer \
  root@source:/var/log/app \
  root@dest:/backup/logs \
  --strategy tar

# Real-world test: 307,928 files (5.7GB) in 6m16s
```

**Features:**
- Parallel workers (configurable)
- Disk buffering (FIFO queue)
- Checkpointing for resume
- Bin packing algorithm (50MB batches)

### 3. Large File Transfers

Use rsync for efficient large file sync:

```bash
difpipe transfer \
  /media/videos \
  user@backup-server:/storage/videos \
  --strategy rsync
```

### 4. Cloud Storage

Transfer to/from cloud storage:

```bash
# To S3
difpipe transfer /data/backup s3://my-bucket/backup/

# From GCS
difpipe transfer gs://source-bucket/data/ /local/restore/

# Between cloud providers
difpipe transfer s3://bucket-a/ gs://bucket-b/ --strategy rclone
```

## Configuration

### Environment Variables

Authentication for remote transfers:

```bash
export DIFPIPE_SOURCE_PASSWORD='your-source-password'
export DIFPIPE_DEST_PASSWORD='your-dest-password'
```

### Configuration File

```yaml
# transfer.yaml
transfer:
  source:
    path: "root@source-server:~/data/"
    auth:
      password: "source-password"
  destination:
    path: "root@dest-server:~/backup/"
    auth:
      password: "dest-password"
  options:
    strategy: auto
    checkpoint: true

# Use with:
difpipe transfer --config transfer.yaml
```

## Architecture

DifPipe is built in Go with a modular engine architecture:

```
┌─────────────────────────────────────────┐
│         CLI Interface                    │
├─────────────────────────────────────────┤
│         Orchestrator                     │
│    (Strategy Selection & Coordination)   │
├──────────┬──────────┬──────────┬────────┤
│  Rclone  │  Rsync   │   Tar    │  Proxy │
│  Engine  │  Engine  │  Engine  │ Engine │
└──────────┴──────────┴──────────┴────────┘
```

### Batched Tar Pipeline

For small files, DifPipe uses a sophisticated pipeline:

```
1. Enumerate files via SSH (find command)
2. Group into 50MB batches (bin packing)
3. Source workers create tar.gz archives in parallel
4. Buffer management (FIFO, configurable size)
5. Destination workers extract in parallel
6. Checkpointing for resume capability
```

## Performance

Real-world test results:

| Scenario | Files | Size | Time | Notes |
|----------|-------|------|------|-------|
| Batched Tar | 307,928 | 5.7 GB | 6m 16s | Remote→Remote via SSH |
| Proxy Transfer | 1 file | 5.0 GB | 1m 49s | 46.9 MB/s throughput |
| Rsync Large Files | Mixed | 10+ GB | Varies | Delta sync efficiency |

## Development Status

### What Works
- ✅ All four transfer engines (rclone, rsync, tar, proxy)
- ✅ Automatic strategy selection
- ✅ Batched tar with parallel workers
- ✅ Checkpointing and resume
- ✅ SSH authentication (password and key)
- ✅ Buffer management

### Known Limitations
- ⚠️ Not all features fully tested in production
- ⚠️ Limited error recovery in some edge cases
- ⚠️ No progress reporting UI (command-line only)
- ⚠️ Configuration options still being refined

### Planned Features
- 📋 Enhanced progress reporting
- 📋 Metrics and monitoring
- 📋 Retry logic with exponential backoff
- 📋 Multi-destination fan-out
- 📋 WebUI for monitoring
- 📋 Plugin architecture

See [PROJECT_PLAN.md](PROJECT_PLAN.md) for the complete roadmap.

## Building from Source

```bash
# Clone repository
git clone https://github.com/CenterfireDigital/difpipe.git
cd difpipe

# Build
go build -o difpipe ./cmd/difpipe

# Run tests
go test ./...

# Install
sudo mv difpipe /usr/local/bin/
```

## Requirements

- **Go**: 1.25+ for building from source
- **rclone**: Required for rclone engine
- **rsync**: Required for rsync engine
- **tar**: Required for tar streaming (usually pre-installed)
- **ssh/sshpass**: Required for remote operations

## Contributing

DifPipe is open source and contributions are welcome! Since the project is in early development, please open an issue first to discuss major changes.

## License

Apache 2.0 - See [LICENSE](LICENSE) for details.

## Author

Created by Larry Diffey ([Centerfire Digital](https://centerfire.digital))

## Acknowledgments

DifPipe builds upon ideas from:
- **rsync** - The legendary file synchronization tool
- **rclone** - Cloud storage synchronization
- **tar** - The classic archival utility

---

**DifPipe** - Smart data transfer, automatically. 🚀
