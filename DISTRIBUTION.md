# Distribution & Release Process

This document outlines what's needed to distribute DifPipe through various channels.

## Current Status

**Available Now:**
- ✅ Build from source (works today)
- ✅ `go install` (works today)

**Not Yet Available:**
- ❌ Homebrew
- ❌ Pre-built binaries
- ❌ Docker images
- ❌ Package managers (apt/yum)

---

## Homebrew Distribution

### What's Needed:

1. **Create GitHub Releases**
   - Tag versions (e.g., `v0.5.0`)
   - Upload pre-built binaries
   - Include checksums

2. **Create Homebrew Formula**
   ```ruby
   # Formula/difpipe.rb
   class Syncerpipe < Formula
     desc "Intelligent data transfer orchestrator"
     homepage "https://github.com/larrydiffey/difpipe"
     url "https://github.com/larrydiffey/difpipe/archive/v0.5.0.tar.gz"
     sha256 "..."
     license "Apache-2.0"

     depends_on "go" => :build

     def install
       system "go", "build", "-o", bin/"difpipe", "./cmd/difpipe"
     end

     test do
       assert_match "difpipe version", shell_output("#{bin}/difpipe --version")
     end
   end
   ```

3. **Options for Distribution**

   **Option A: Official Homebrew (Recommended)**
   - Submit PR to `homebrew/homebrew-core`
   - Requires meeting quality standards
   - Will be reviewed by Homebrew maintainers
   - Takes longer but reaches more users
   - URL: https://github.com/Homebrew/homebrew-core

   **Option B: Personal Tap (Quick Start)**
   - Create `homebrew-difpipe` repo
   - Users install with: `brew tap larrydiffey/difpipe && brew install difpipe`
   - Faster to set up
   - Full control over releases
   - Smaller user reach

### Steps to Create Personal Tap:

```bash
# 1. Create tap repository
mkdir homebrew-difpipe
cd homebrew-difpipe

# 2. Create Formula directory
mkdir Formula

# 3. Add formula
cat > Formula/difpipe.rb <<'EOF'
class Syncerpipe < Formula
  desc "Intelligent data transfer orchestrator"
  homepage "https://github.com/larrydiffey/difpipe"
  url "https://github.com/larrydiffey/difpipe/archive/refs/tags/v0.5.0.tar.gz"
  sha256 "CALCULATE_THIS"
  license "Apache-2.0"

  depends_on "go" => :build

  def install
    system "go", "build", "-o", bin/"difpipe", "./cmd/difpipe"
  end

  test do
    assert_match "0.5.0", shell_output("#{bin}/difpipe --version")
  end
end
EOF

# 4. Push to GitHub
git init
git add .
git commit -m "Add difpipe formula"
git remote add origin https://github.com/larrydiffey/homebrew-difpipe.git
git push -u origin main
```

### Calculate SHA256:
```bash
curl -L https://github.com/larrydiffey/difpipe/archive/refs/tags/v0.5.0.tar.gz | shasum -a 256
```

---

## GitHub Releases with Binaries

### What's Needed:

1. **GoReleaser Configuration**

Create `.goreleaser.yaml`:
```yaml
project_name: difpipe

before:
  hooks:
    - go mod tidy
    - go test ./...

builds:
  - main: ./cmd/difpipe
    binary: difpipe
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.commit={{.Commit}}
      - -X main.date={{.Date}}

archives:
  - format: tar.gz
    format_overrides:
      - goos: windows
        format: zip
    files:
      - README.md
      - LICENSE

checksum:
  name_template: 'checksums.txt'

changelog:
  sort: asc
  filters:
    exclude:
      - '^docs:'
      - '^test:'
```

2. **GitHub Actions for Releases**

Create `.github/workflows/release.yml`:
```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

permissions:
  contents: write

jobs:
  goreleaser:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      - uses: goreleaser/goreleaser-action@v5
        with:
          version: latest
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

3. **Create Release**
```bash
# Tag version
git tag -a v0.5.0 -m "Release v0.5.0"
git push origin v0.5.0

# GoReleaser will automatically:
# - Build binaries for all platforms
# - Create GitHub release
# - Upload binaries
# - Generate checksums
# - Create changelog
```

---

## Docker Images

### What's Needed:

Create `Dockerfile`:
```dockerfile
FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o difpipe ./cmd/difpipe

FROM alpine:latest
RUN apk --no-cache add ca-certificates rsync rclone

COPY --from=builder /app/difpipe /usr/local/bin/

ENTRYPOINT ["difpipe"]
CMD ["--help"]
```

### Build and Push:
```bash
# Build
docker build -t difpipe:0.5.0 .

# Tag for Docker Hub
docker tag difpipe:0.5.0 larrydiffey/difpipe:0.5.0
docker tag difpipe:0.5.0 larrydiffey/difpipe:latest

# Push
docker push larrydiffey/difpipe:0.5.0
docker push larrydiffey/difpipe:latest
```

### Usage:
```bash
docker run --rm difpipe:0.5.0 --version
docker run --rm -v /data:/data difpipe:0.5.0 analyze /data
```

---

## Package Managers (apt/yum)

### Debian/Ubuntu (apt)

1. **Create .deb package**
   - Use `fpm` tool or native `dpkg-deb`
   - Include binary, man pages, systemd service

2. **Host on package repository**
   - Use packagecloud.io or own apt repo
   - Or submit to Debian/Ubuntu official repos (lengthy process)

### RedHat/CentOS (yum/dnf)

1. **Create .rpm package**
   - Use `rpmbuild` or `fpm`
   - Include spec file

2. **Host on repository**
   - Use packagecloud.io or COPR
   - Or submit to EPEL (lengthy process)

---

## Recommended First Steps

1. **✅ Set up GitHub Releases with GoReleaser** (easiest, most impact)
   - Provides binaries for all platforms
   - Automated with GitHub Actions
   - Users can download directly

2. **✅ Create Personal Homebrew Tap** (macOS/Linux users)
   - Quick to set up
   - Full control
   - Good user experience

3. **⏳ Docker Images** (for containerized deployments)
   - Good for CI/CD integration
   - Easy to maintain

4. **⏳ Submit to Official Homebrew** (after stability)
   - Wait for v1.0.0
   - Larger user reach
   - More credibility

5. **⏳ Package Managers** (later, for enterprise)
   - After v1.0.0
   - When demand is proven

---

## Installation After Setup

Once released, users can:

```bash
# Homebrew (after tap created)
brew tap larrydiffey/difpipe
brew install difpipe

# Or from official (if accepted)
brew install difpipe

# Direct download (after GitHub releases)
curl -L https://github.com/larrydiffey/difpipe/releases/download/v0.5.0/difpipe_Linux_x86_64.tar.gz | tar xz
sudo mv difpipe /usr/local/bin/

# Docker (after image published)
docker pull larrydiffey/difpipe:0.5.0

# Go install (works now!)
go install github.com/larrydiffey/difpipe/cmd/difpipe@latest
```

---

## Checklist for First Release

- [ ] Set up GoReleaser config
- [ ] Create GitHub Actions workflow
- [ ] Tag v0.5.0 release
- [ ] Verify binaries build correctly
- [ ] Create personal Homebrew tap
- [ ] Test installation on clean machine
- [ ] Update README with working instructions
- [ ] Announce release

**Estimated Time:** 2-3 hours for complete setup
